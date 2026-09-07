package main

import (
	"strings"
)

// ============ 控制流信号 ============
// return / break / continue 通过信号向上传递，而不是共享解释器字段，
// 因此递归调用不会互相覆盖返回值。

type sigKind int

const (
	sigNone sigKind = iota
	sigReturn
	sigBreak
	sigContinue
)

type signal struct {
	kind sigKind
	val  Object
}

type Interpreter struct {
	globals    *Environment
	env        *Environment
	callDepth  int
	reprDepth  int
	currentGen *Generator
	// frames 记录当前调用栈（供零参 super() 获取当前类与 self/cls）
	frames []*callFrame
}

type callFrame struct {
	fn    *Function
	local *Environment
}

// setDefClass 递归设置包装对象内部函数的定义类
func setDefClass(v Object, cls *Class) {
	switch w := v.(type) {
	case *Function:
		w.DefClass = cls
	case *StaticMethod:
		w.Fn.DefClass = cls
	case *ClassMethod:
		w.Fn.DefClass = cls
	case *Property:
		w.Getter.DefClass = cls
	}
}

// activeInterp 供内建函数（map / filter / sorted 的 key）回调用户函数
var activeInterp *Interpreter

func NewInterpreter(argv []Object) *Interpreter {
	g := initGlobalEnv(argv)
	interp := &Interpreter{globals: g, env: g}
	activeInterp = interp
	return interp
}

func (i *Interpreter) Run(prog *Program) error {
	_, err := i.execBlock(prog.Body)
	return err
}

// ============ 语句执行 ============

func (i *Interpreter) execBlock(stmts []Stmt) (*signal, error) {
	for _, s := range stmts {
		sig, err := i.exec(s)
		if err != nil || sig != nil {
			return sig, err
		}
	}
	return nil, nil
}

func (i *Interpreter) exec(s Stmt) (*signal, error) {
	switch st := s.(type) {
	case *ExprStmt:
		_, err := i.eval(st.Value)
		return nil, err

	case *Assign:
		return nil, i.execAssign(st)

	case *AugAssign:
		return nil, i.execAugAssign(st)

	case *IfStmt:
		cond, err := i.eval(st.Cond)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return i.execBlock(st.Body)
		}
		for _, el := range st.Elifs {
			c, err := i.eval(el.Cond)
			if err != nil {
				return nil, err
			}
			if truthy(c) {
				return i.execBlock(el.Body)
			}
		}
		if st.Else != nil {
			return i.execBlock(st.Else)
		}
		return nil, nil

	case *WhileStmt:
		broken := false
		for {
			cond, err := i.eval(st.Cond)
			if err != nil {
				return nil, err
			}
			if !truthy(cond) {
				break
			}
			sig, err := i.execBlock(st.Body)
			if err != nil {
				return nil, err
			}
			if sig == nil {
				continue
			}
			switch sig.kind {
			case sigBreak:
				broken = true
			case sigContinue:
				continue
			case sigReturn:
				return sig, nil
			}
			if broken {
				break
			}
		}
		if !broken && st.Else != nil {
			return i.execBlock(st.Else)
		}
		return nil, nil

	case *ForStmt:
		iterVal, err := i.eval(st.Iter)
		if err != nil {
			return nil, err
		}
		items, err := iterate(iterVal)
		if err != nil {
			return nil, err
		}
		broken := false
		for _, item := range items {
			if err := i.bindTargets(st.Targets, item); err != nil {
				return nil, err
			}
			sig, err := i.execBlock(st.Body)
			if err != nil {
				return nil, err
			}
			if sig == nil {
				continue
			}
			switch sig.kind {
			case sigBreak:
				broken = true
			case sigContinue:
				continue
			case sigReturn:
				return sig, nil
			}
			if broken {
				break
			}
		}
		if !broken && st.Else != nil {
			return i.execBlock(st.Else)
		}
		return nil, nil

	case *FuncDef:
		defaults, err := i.evalDefaults(st.Params)
		if err != nil {
			return nil, err
		}
		i.env.Set(st.Name, &Function{
			Name:     st.Name,
			Params:   st.Params,
			Body:     st.Body,
			Env:      i.env,
			Defaults: defaults,
			IsGen:    containsYield(st.Body),
		})
		return nil, nil

	case *ReturnStmt:
		if st.Value == nil {
			return &signal{kind: sigReturn, val: None}, nil
		}
		v, err := i.eval(st.Value)
		if err != nil {
			return nil, err
		}
		return &signal{kind: sigReturn, val: v}, nil

	case *YieldStmt:
		// yield 在执行现场同步让步：发送值后阻塞等待下一次 next()，
		// 恢复后继续执行同一语句块，因此 yield 信号不会向上传播。
		g := i.currentGen
		if g == nil {
			return nil, newExc("SyntaxError", "'yield' outside function")
		}
		var val Object = None
		if st.Value != nil {
			v, err := i.eval(st.Value)
			if err != nil {
				return nil, err
			}
			val = v
		}
		g.ch <- genEvent{val: val}
		<-g.resume
		i.env = g.localEnv
		i.callDepth = g.depth + 1
		return nil, nil

	case *BreakStmt:
		return &signal{kind: sigBreak}, nil

	case *ContinueStmt:
		return &signal{kind: sigContinue}, nil

	case *PassStmt:
		return nil, nil

	case *ClassDef:
		return nil, i.execClassDef(st)

	case *TryStmt:
		return i.execTry(st)

	case *WithStmt:
		return i.execWith(st)

	case *ImportStmt:
		for _, a := range st.Names {
			mod, err := i.importModule(a.Path)
			if err != nil {
				return nil, err
			}
			name := a.Alias
			if name == "" {
				name = a.Path[0]
			}
			i.env.Set(name, mod)
		}
		return nil, nil

	case *FromImportStmt:
		mod, err := i.importModule(st.Module)
		if err != nil {
			return nil, err
		}
		m, ok := mod.(*Module)
		if !ok {
			return nil, newExc("ImportError", "'%s' 不是模块", strings.Join(st.Module, "."))
		}
		for _, a := range st.Names {
			if a.Path[0] == "*" {
				for k, v := range m.Attrs {
					i.env.Set(k, v)
				}
				continue
			}
			v, ok := m.Attrs[a.Path[0]]
			if !ok {
				return nil, newExc("ImportError", "无法从 '%s' 导入名称 '%s'", m.Name, a.Path[0])
			}
			name := a.Alias
			if name == "" {
				name = a.Path[0]
			}
			i.env.Set(name, v)
		}
		return nil, nil

	case *RaiseStmt:
		return nil, i.execRaise(st)

	case *AssertStmt:
		v, err := i.eval(st.Cond)
		if err != nil {
			return nil, err
		}
		if !truthy(v) {
			msg := ""
			if st.Msg != nil {
				mv, err := i.eval(st.Msg)
				if err != nil {
					return nil, err
				}
				msg = Str(mv)
			}
			return nil, &PyException{ExcType: "AssertionError", Msg: msg}
		}
		return nil, nil

	case *DeleteStmt:
		return nil, i.execDelete(st)

	case *GlobalStmt:
		if i.env.declaredGlobal == nil {
			i.env.declaredGlobal = map[string]bool{}
		}
		for _, n := range st.Names {
			i.env.declaredGlobal[n] = true
			if _, ok := i.env.global.vars[n]; !ok {
				i.env.global.vars[n] = None
			}
		}
		return nil, nil

	case *NonlocalStmt:
		if i.env.declaredNonlocal == nil {
			i.env.declaredNonlocal = map[string]bool{}
		}
		for _, n := range st.Names {
			// 与 Python 一致：nonlocal 的名字必须已在某个外层函数作用域中绑定
			found := false
			for cur := i.env.parent; cur != nil; cur = cur.parent {
				if _, ok := cur.vars[n]; ok {
					found = true
					break
				}
			}
			if !found {
				return nil, newExc("SyntaxError", "no binding for nonlocal '%s' found", n)
			}
			i.env.declaredNonlocal[n] = true
		}
		return nil, nil

	case *DecoratedStmt:
		return nil, i.execDecorated(st)
	}
	return nil, newExc("RuntimeError", "无法执行的语句")
}

// execWith 执行 with 语句：按序进入各上下文，执行体后按相反顺序退出。
// 若体中发生异常，异常信息传给 __exit__；__exit__ 返回 True 表示抑制异常。
func (i *Interpreter) execWith(st *WithStmt) (*signal, error) {
	exits := make([]Object, 0, len(st.Items))
	for _, it := range st.Items {
		v, err := i.eval(it.CtxExpr)
		if err != nil {
			if ferr := i.runWithExits(exits, nil); ferr != nil {
				return nil, ferr
			}
			return nil, err
		}
		enterFn, err := getAttr(v, "__enter__")
		if err != nil {
			if ferr := i.runWithExits(exits, nil); ferr != nil {
				return nil, ferr
			}
			return nil, err
		}
		res, err := i.callObject(enterFn, nil, nil)
		if err != nil {
			if ferr := i.runWithExits(exits, nil); ferr != nil {
				return nil, ferr
			}
			return nil, err
		}
		if it.Alias != "" {
			i.setVar(it.Alias, res)
		}
		exitFn, err := getAttr(v, "__exit__")
		if err != nil {
			if ferr := i.runWithExits(exits, nil); ferr != nil {
				return nil, ferr
			}
			return nil, err
		}
		exits = append(exits, exitFn)
	}
	sig, berr := i.execBlock(st.Body)
	remaining := i.runWithExits(exits, berr)
	if remaining != nil {
		return nil, remaining
	}
	return sig, nil
}

// runWithExits 按相反顺序调用 __exit__；返回值为 True 时抑制传入的异常
func (i *Interpreter) runWithExits(exits []Object, bodyErr error) error {
	err := bodyErr
	for k := len(exits) - 1; k >= 0; k-- {
		var args []Object
		if err != nil {
			if pe, ok := err.(*PyException); ok {
				args = []Object{&PyType{Name: pe.ExcType}, pe, None}
			} else {
				args = []Object{&PyType{Name: "RuntimeError"}, None, None}
			}
		} else {
			args = []Object{None, None, None}
		}
		res, eerr := i.callObject(exits[k], args, nil)
		if eerr != nil {
			return eerr
		}
		if err != nil {
			if b, ok := res.(bool); ok && b {
				err = nil
			}
		}
	}
	return err
}

// ---------- 装饰器 ----------

// execDecorated 先执行被装饰的 def/class，再自内向外应用装饰器
func (i *Interpreter) execDecorated(st *DecoratedStmt) error {
	var name string
	switch t := st.Target.(type) {
	case *FuncDef:
		name = t.Name
	case *ClassDef:
		name = t.Name
	default:
		return newExc("SyntaxError", "装饰器只能修饰 def 或 class")
	}
	if _, err := i.exec(st.Target); err != nil {
		return err
	}
	v, ok := i.env.Get(name)
	if !ok {
		return newExc("RuntimeError", "装饰目标 '%s' 定义失败", name)
	}
	for k := len(st.Decorators) - 1; k >= 0; k-- {
		d, err := i.eval(st.Decorators[k])
		if err != nil {
			return err
		}
		v, err = i.callObject(d, []Object{v}, nil)
		if err != nil {
			return err
		}
	}
	i.env.Set(name, v)
	return nil
}

// ---------- 赋值 ----------

func (i *Interpreter) execAssign(st *Assign) error {
	v, err := i.eval(st.Value)
	if err != nil {
		return err
	}
	if len(st.Targets) == 1 {
		return i.assignTarget(st.Targets[0], v)
	}
	// 元组解包：a, b = 1, 2
	if seq, ok := v.(*Tuple); ok {
		if len(seq.Items) != len(st.Targets) {
			return newExc("ValueError", "解包数量不匹配：期望 %d 个，实际 %d 个", len(st.Targets), len(seq.Items))
		}
		for k, t := range st.Targets {
			if err := i.assignTarget(t, seq.Items[k]); err != nil {
				return err
			}
		}
		return nil
	}
	if seq, ok := v.(*List); ok {
		if len(seq.Items) != len(st.Targets) {
			return newExc("ValueError", "解包数量不匹配：期望 %d 个，实际 %d 个", len(st.Targets), len(seq.Items))
		}
		for k, t := range st.Targets {
			if err := i.assignTarget(t, seq.Items[k]); err != nil {
				return err
			}
		}
		return nil
	}
	// 链式赋值：a = b = 1
	for _, t := range st.Targets {
		if err := i.assignTarget(t, v); err != nil {
			return err
		}
	}
	return nil
}

func (i *Interpreter) assignTarget(t Expr, v Object) error {
	switch tt := t.(type) {
	case *Name:
		i.setVar(tt.Id, v)
		return nil
	case *Attribute:
		obj, err := i.eval(tt.Value)
		if err != nil {
			return err
		}
		switch t := obj.(type) {
		case *Instance:
			t.Fields[tt.Attr] = v
			return nil
		case *Class:
			t.Attrs[tt.Attr] = v
			return nil
		}
		return newExc("AttributeError", "'%s' 对象不支持属性赋值", typeName(obj))
	case *Subscript:
		obj, err := i.eval(tt.Value)
		if err != nil {
			return err
		}
		key, err := i.eval(tt.Index)
		if err != nil {
			return err
		}
		return i.setItem(obj, key, v)
	case *TupleLit:
		items, err := iterate(v)
		if err != nil {
			return newExc("TypeError", "无法解包 '%s' 对象", typeName(v))
		}
		if len(items) != len(tt.Items) {
			return newExc("ValueError", "解包数量不匹配：期望 %d 个，实际 %d 个", len(tt.Items), len(items))
		}
		for k, sub := range tt.Items {
			if err := i.assignTarget(sub, items[k]); err != nil {
				return err
			}
		}
		return nil
	}
	return newExc("SyntaxError", "无法赋值给该目标")
}

// setVar 采用 Python 语义：赋值在当前作用域建立绑定，
// 除非该名字在当前帧被 global / nonlocal 声明过。
func (i *Interpreter) setVar(name string, v Object) {
	if i.env.declaredGlobal[name] {
		i.env.global.vars[name] = v
		return
	}
	if i.env.declaredNonlocal[name] {
		// 绑定到最近一个拥有该名字的外层作用域（exec NonlocalStmt 已校验存在性）
		for cur := i.env.parent; cur != nil; cur = cur.parent {
			if _, ok := cur.vars[name]; ok {
				cur.vars[name] = v
				return
			}
		}
	}
	i.env.Set(name, v)
}

func (i *Interpreter) bindTargets(targets []Expr, v Object) error {
	if len(targets) == 1 {
		return i.assignTarget(targets[0], v)
	}
	items, err := iterate(v)
	if err != nil {
		return newExc("TypeError", "无法解包 '%s' 对象", typeName(v))
	}
	if len(items) != len(targets) {
		return newExc("ValueError", "解包数量不匹配：期望 %d 个，实际 %d 个", len(targets), len(items))
	}
	for k, t := range targets {
		if err := i.assignTarget(t, items[k]); err != nil {
			return err
		}
	}
	return nil
}

func (i *Interpreter) setItem(obj, key, v Object) error {
	// 实例的 __setitem__ 优先
	if inst, ok := obj.(*Instance); ok {
		if fn, has := inst.Class.LookupMethod("__setitem__"); has {
			_, err := callObjectRef(&Method{Recv: inst, Fn: fn}, []Object{key, v}, nil)
			return err
		}
	}
	switch x := obj.(type) {
	case *Dict:
		x.Set(key, v)
		return nil
	case *List:
		n, ok := intVal(key)
		if !ok {
			return newExc("TypeError", "列表下标必须是整数，实际为 '%s'", typeName(key))
		}
		if n < 0 {
			n += len(x.Items)
		}
		if n < 0 || n >= len(x.Items) {
			return newExc("IndexError", "list assignment index out of range")
		}
		x.Items[n] = v
		return nil
	case *PyCounter:
		x.D.Set(key, v)
		return nil
	case *PyDefaultDict:
		x.D.Set(key, v)
		return nil
	case *PyOrderedDict:
		x.D.Set(key, v)
		return nil
	case *PyDeque:
		n, ok := intVal(key)
		if !ok {
			return newExc("TypeError", "deque 下标必须是整数")
		}
		if n < 0 {
			n += len(x.Items)
		}
		if n < 0 || n >= len(x.Items) {
			return newExc("IndexError", "deque assignment index out of range")
		}
		x.Items[n] = v
		return nil
	}
	return newExc("TypeError", "'%s' 对象不支持下标赋值", typeName(obj))
}

func (i *Interpreter) execAugAssign(st *AugAssign) error {
	cur, err := i.eval(st.Target)
	if err != nil {
		return err
	}
	rhs, err := i.eval(st.Value)
	if err != nil {
		return err
	}
	res, err := binaryOp(strings.TrimSuffix(st.Op, "="), cur, rhs)
	if err != nil {
		return err
	}
	return i.assignTarget(st.Target, res)
}

func (i *Interpreter) execDelete(st *DeleteStmt) error {
	for _, t := range st.Targets {
		switch tt := t.(type) {
		case *Name:
			if !i.env.Delete(tt.Id) {
				return newExc("NameError", "name '%s' is not defined", tt.Id)
			}
		case *Attribute:
			obj, err := i.eval(tt.Value)
			if err != nil {
				return err
			}
			inst, ok := obj.(*Instance)
			if !ok {
				return newExc("AttributeError", "'%s' 对象不支持属性删除", typeName(obj))
			}
			delete(inst.Fields, tt.Attr)
		case *Subscript:
			obj, err := i.eval(tt.Value)
			if err != nil {
				return err
			}
			key, err := i.eval(tt.Index)
			if err != nil {
				return err
			}
			switch x := obj.(type) {
			case *Dict:
				x.Delete(key)
			case *List:
				n, ok := intVal(key)
				if !ok {
					return newExc("TypeError", "列表下标必须是整数")
				}
				if n < 0 {
					n += len(x.Items)
				}
				if n < 0 || n >= len(x.Items) {
					return newExc("IndexError", "list assignment index out of range")
				}
				x.Items = append(x.Items[:n], x.Items[n+1:]...)
			default:
				return newExc("TypeError", "'%s' 对象不支持元素删除", typeName(obj))
			}
		default:
			return newExc("SyntaxError", "无法删除该目标")
		}
	}
	return nil
}

// ---------- 类 ----------

func (i *Interpreter) execClassDef(st *ClassDef) error {
	cls := &Class{Name: st.Name, Methods: map[string]*Function{}, Attrs: map[string]Object{}}
	var bases []*Class
	for _, be := range st.Bases {
		bv, err := i.eval(be)
		if err != nil {
			return err
		}
		bcls, ok := bv.(*Class)
		if !ok {
			return newExc("TypeError", "基类必须是类，实际为 '%s'", typeName(bv))
		}
		bases = append(bases, bcls)
	}
	cls.Bases = bases
	mro, err := computeMRO(cls, bases)
	if err != nil {
		return err
	}
	cls.MRO = mro
	classEnv := NewEnvironment(i.env)
	saved := i.env
	i.env = classEnv
	for _, bs := range st.Body {
		if fd, ok := bs.(*FuncDef); ok {
			var defaults []Object
			defaults, err = i.evalDefaults(fd.Params)
			if err != nil {
				break
			}
			fn := &Function{
				Name:     fd.Name,
				Params:   fd.Params,
				Body:     fd.Body,
				Env:      classEnv,
				Defaults: defaults,
				IsGen:    containsYield(fd.Body),
			}
			fn.DefClass = cls
			cls.Methods[fd.Name] = fn
			continue
		}
		if ds, ok := bs.(*DecoratedStmt); ok {
			err = i.execDecorated(ds)
			if err != nil {
				break
			}
			name := ds.decoratedName()
			if v, ok := classEnv.vars[name]; ok {
				if fn, isFn := v.(*Function); isFn {
					setDefClass(v, cls)
					cls.Methods[name] = fn
					// 函数从 classEnv 移除；其余（property/static/classmethod 等
					// 包装对象）保留，随后统一成为类属性
					delete(classEnv.vars, name)
				} else {
					setDefClass(v, cls)
				}
			}
			continue
		}
		_, err = i.exec(bs)
		if err != nil {
			break
		}
	}
	i.env = saved
	if err != nil {
		return err
	}
	// 类体中的非函数绑定成为类属性
	for name, v := range classEnv.vars {
		if _, isFn := v.(*Function); !isFn {
			cls.Attrs[name] = v
		}
	}
	i.env.Set(st.Name, cls)
	return nil
}

func (i *Interpreter) evalDefaults(params []Param) ([]Object, error) {
	var defaults []Object
	for _, p := range params {
		if p.Default == nil {
			defaults = append(defaults, nil)
			continue
		}
		v, err := i.eval(p.Default)
		if err != nil {
			return nil, err
		}
		defaults = append(defaults, v)
	}
	return defaults, nil
}

func (i *Interpreter) instantiate(cls *Class, args []Object, kwargs map[string]Object) (Object, error) {
	inst := &Instance{Class: cls, Fields: map[string]Object{}}
	if fn, ok := cls.LookupMethod("__init__"); ok {
		all := make([]Object, 0, len(args)+1)
		all = append(all, inst)
		all = append(all, args...)
		if _, err := i.callFunction(fn, all, kwargs); err != nil {
			return nil, err
		}
	}
	return inst, nil
}

// init 注入实例字符串化钩子：Repr/Str 通过它调用 __repr__/__str__
func init() {
	instanceStrHook = func(inst *Instance, dunder string) (string, bool) {
		if activeInterp == nil || activeInterp.reprDepth >= 4 {
			return "", false
		}
		if _, ok := inst.Class.LookupMethod(dunder); !ok {
			return "", false
		}
		activeInterp.reprDepth++
		defer func() { activeInterp.reprDepth-- }()
		m, err := getAttr(inst, dunder)
		if err != nil {
			return "", false
		}
		v, err := activeInterp.callObject(m, nil, nil)
		if err != nil {
			return "", false
		}
		s, ok := v.(string)
		return s, ok
	}
}

// ---------- 异常处理 ----------

func (i *Interpreter) execTry(st *TryStmt) (*signal, error) {
	sig, err := i.execBlock(st.Body)
	if err == nil && sig == nil && st.Else != nil {
		sig, err = i.execBlock(st.Else)
	}
	if err != nil {
		exc, ok := err.(*PyException)
		if !ok {
			if st.Finally != nil {
				i.execBlock(st.Finally)
			}
			return nil, err
		}
		matched := false
		for _, h := range st.Handlers {
			if h.ExcType == nil {
				matched = true
			} else {
				tv, terr := i.eval(h.ExcType)
				if terr != nil {
					continue
				}
				if isInstanceOf(Object(exc), tv) {
					matched = true
				}
			}
			if !matched {
				continue
			}
			if h.Name != "" {
				i.env.Set(h.Name, Object(exc))
			}
			sig, err = i.execBlock(h.Body)
			break
		}
	}
	if st.Finally != nil {
		fsig, ferr := i.execBlock(st.Finally)
		if ferr != nil {
			return nil, ferr
		}
		if fsig != nil {
			return fsig, nil
		}
	}
	return sig, err
}

func (i *Interpreter) execRaise(st *RaiseStmt) error {
	if st.Value == nil {
		return &PyException{ExcType: "RuntimeError", Msg: "No active exception to re-raise"}
	}
	v, err := i.eval(st.Value)
	if err != nil {
		return err
	}
	switch x := v.(type) {
	case *PyException:
		return x
	case *PyType:
		return &PyException{ExcType: x.Name}
	case string:
		return &PyException{ExcType: "Exception", Msg: x}
	}
	return newExc("TypeError", "异常必须继承自 BaseException，实际为 '%s'", typeName(v))
}

// ---------- import ----------

func (i *Interpreter) importModule(path []string) (Object, error) {
	if len(path) == 0 {
		return nil, newExc("ImportError", "空的模块名")
	}
	full := strings.Join(path, ".")
	cur, ok := i.globals.Get(path[0])
	if !ok {
		return nil, newExc("ImportError", "没有名为 '%s' 的模块", full)
	}
	for _, p := range path[1:] {
		m, ok := cur.(*Module)
		if !ok {
			return nil, newExc("ImportError", "没有名为 '%s' 的模块", full)
		}
		cur, ok = m.Attrs[p]
		if !ok {
			return nil, newExc("ImportError", "没有名为 '%s' 的模块", full)
		}
	}
	return cur, nil
}

// ============ 表达式求值 ============

func (i *Interpreter) eval(e Expr) (Object, error) {
	switch x := e.(type) {
	case *IntLit:
		return x.Value, nil
	case *FloatLit:
		return x.Value, nil
	case *StrLit:
		return x.Value, nil
	case *BoolLit:
		return x.Value, nil
	case *NoneLit:
		return None, nil

	case *Name:
		if v, ok := i.env.Get(x.Id); ok {
			return v, nil
		}
		if v, ok := i.globals.Get(x.Id); ok {
			return v, nil
		}
		return nil, newExc("NameError", "name '%s' is not defined", x.Id)

	case *ListLit:
		items := make([]Object, len(x.Items))
		for k, it := range x.Items {
			v, err := i.eval(it)
			if err != nil {
				return nil, err
			}
			items[k] = v
		}
		return &List{Items: items}, nil

	case *TupleLit:
		items := make([]Object, len(x.Items))
		for k, it := range x.Items {
			v, err := i.eval(it)
			if err != nil {
				return nil, err
			}
			items[k] = v
		}
		return &Tuple{Items: items}, nil

	case *DictLit:
		d := NewDict()
		for k := range x.Keys {
			kv, err := i.eval(x.Keys[k])
			if err != nil {
				return nil, err
			}
			vv, err := i.eval(x.Vals[k])
			if err != nil {
				return nil, err
			}
			d.Set(kv, vv)
		}
		return d, nil

	case *BinOp:
		l, err := i.eval(x.Left)
		if err != nil {
			return nil, err
		}
		r, err := i.eval(x.Right)
		if err != nil {
			return nil, err
		}
		return binaryOp(x.Op, l, r)

	case *UnaryOp:
		v, err := i.eval(x.Operand)
		if err != nil {
			return nil, err
		}
		return unaryOp(x.Op, v)

	case *BoolOp:
		var last Object
		for _, operand := range x.Values {
			v, err := i.eval(operand)
			if err != nil {
				return nil, err
			}
			last = v
			if x.Op == "and" && !truthy(v) {
				return v, nil
			}
			if x.Op == "or" && truthy(v) {
				return v, nil
			}
		}
		return last, nil

	case *Compare:
		return i.evalCompare(x)

	case *Call:
		return i.evalCall(x)

	case *Subscript:
		obj, err := i.eval(x.Value)
		if err != nil {
			return nil, err
		}
		if sl, ok := x.Index.(*Slice); ok {
			var start, stop, step Object
			if sl.Start != nil {
				if start, err = i.eval(sl.Start); err != nil {
					return nil, err
				}
			}
			if sl.Stop != nil {
				if stop, err = i.eval(sl.Stop); err != nil {
					return nil, err
				}
			}
			if sl.Step != nil {
				if step, err = i.eval(sl.Step); err != nil {
					return nil, err
				}
			}
			return i.sliceObject(obj, start, stop, step)
		}
		key, err := i.eval(x.Index)
		if err != nil {
			return nil, err
		}
		return i.getItem(obj, key)

	case *Attribute:
		obj, err := i.eval(x.Value)
		if err != nil {
			return nil, err
		}
		return getAttr(obj, x.Attr)

	case *FStrLit:
		return i.evalFString(x)

	case *ListComp:
		return i.evalListComp(x)

	case *GenExpr:
		// 生成器表达式编译为一个无参生成器函数，惰性求值
		return &Generator{Fn: &Function{
			Name:  "<genexpr>",
			Body:  genExprBody(x.Elem, x.Clauses),
			Env:   i.env,
			IsGen: true,
		}}, nil

	case *DictComp:
		return i.evalDictComp(x)

	case *CondExpr:
		cond, err := i.eval(x.Cond)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return i.eval(x.Body)
		}
		return i.eval(x.Else)

	case *Lambda:
		defaults, err := i.evalDefaults(x.Params)
		if err != nil {
			return nil, err
		}
		return &Function{
			Name:     "<lambda>",
			Params:   x.Params,
			Body:     []Stmt{&ReturnStmt{Value: x.Body}},
			Env:      i.env,
			Defaults: defaults,
		}, nil
	}
	return nil, newExc("RuntimeError", "无法求值的表达式")
}

func (i *Interpreter) evalCompare(c *Compare) (Object, error) {
	left, err := i.eval(c.Left)
	if err != nil {
		return nil, err
	}
	prev := left
	for k, op := range c.Ops {
		right, err := i.eval(c.Comps[k])
		if err != nil {
			return nil, err
		}
		res, err := applyCompare(op, prev, right)
		if err != nil {
			return nil, err
		}
		b, ok := res.(bool)
		if !ok || !b {
			return res, nil
		}
		prev = right
	}
	return true, nil
}

// instanceCompareOp 尝试用实例的重载比较方法比较；命中返回结果
func instanceCompareOp(op string, l, r Object) (Object, bool, error) {
	li, lok := l.(*Instance)
	if !lok {
		return nil, false, nil
	}
	var name string
	switch op {
	case "<":
		name = "__lt__"
	case "<=":
		name = "__le__"
	case ">":
		name = "__gt__"
	case ">=":
		name = "__ge__"
	default:
		return nil, false, nil
	}
	fn, ok := li.Class.LookupMethod(name)
	if !ok {
		return nil, false, nil
	}
	v, err := callObjectRef(&Method{Recv: li, Fn: fn}, []Object{r}, nil)
	return v, true, err
}

// instanceEqOp 尝试 __eq__ / __ne__；hit 为 false 表示没有定义
func instanceEqOp(op string, l, r Object) (Object, bool, error) {
	name := "__ne__"
	if op == "==" {
		name = "__eq__"
	}
	for _, cand := range []Object{l, r} {
		inst, ok := cand.(*Instance)
		if !ok {
			continue
		}
		fn, has := inst.Class.LookupMethod(name)
		if !has {
			continue
		}
		v, err := callObjectRef(&Method{Recv: inst, Fn: fn}, []Object{otherOf(l, r, cand)}, nil)
		return v, true, err
	}
	return nil, false, nil
}

func otherOf(l, r, cand Object) Object {
	if cand == l {
		return r
	}
	return l
}

func applyCompare(op string, l, r Object) (Object, error) {
	switch op {
	case "==", "!=":
		if v, hit, err := instanceEqOp(op, l, r); hit {
			if err != nil {
				return nil, err
			}
			if op == "!=" {
				return !truthy(v), nil
			}
			return v, nil
		}
		if op == "==" {
			return objectsEqual(l, r), nil
		}
		return !objectsEqual(l, r), nil
	case "in":
		return contains(l, r)
	case "not in":
		v, err := contains(l, r)
		if err != nil {
			return nil, err
		}
		return !v, nil
	case "is":
		return sameObject(l, r), nil
	case "is not":
		return !sameObject(l, r), nil
	case "<", "<=", ">", ">=":
		if v, hit, err := instanceCompareOp(op, l, r); hit {
			if err != nil {
				return nil, err
			}
			return v, nil
		}
		c, ok := compareValues(l, r)
		if !ok {
			return nil, newExc("TypeError", "'%s' 与 '%s' 之间不支持 '%s'", typeName(l), typeName(r), op)
		}
		switch op {
		case "<":
			return c < 0, nil
		case "<=":
			return c <= 0, nil
		case ">":
			return c > 0, nil
		case ">=":
			return c >= 0, nil
		}
	}
	return nil, newExc("SyntaxError", "未知的比较运算符 %s", op)
}

func sameObject(a, b Object) bool {
	switch a.(type) {
	case *List, *Tuple, *Dict, *Set, *Instance, *Function, *Class, *Module, *PyNone:
		return a == b
	}
	return objectsEqual(a, b)
}

func (i *Interpreter) evalFString(f *FStrLit) (Object, error) {
	var sb strings.Builder
	for _, p := range f.Parts {
		if p.Value == nil {
			sb.WriteString(p.Text)
			continue
		}
		v, err := i.eval(p.Value)
		if err != nil {
			return nil, err
		}
		if p.Spec != "" {
			target := v
			if p.Conv == "r" {
				target = Repr(v)
			}
			s, ferr := applyFormatSpec(target, p.Spec)
			if ferr != nil {
				return nil, ferr
			}
			sb.WriteString(s)
			continue
		}
		if p.Conv == "r" {
			sb.WriteString(Repr(v))
		} else {
			sb.WriteString(Str(v))
		}
	}
	return sb.String(), nil
}

// ---------- 推导式 ----------

func (i *Interpreter) evalListComp(lc *ListComp) (Object, error) {
	out := &List{}
	err := i.runComp(lc.Clauses, 0, func() error {
		v, err := i.eval(lc.Elem)
		if err != nil {
			return err
		}
		out.Items = append(out.Items, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (i *Interpreter) evalDictComp(dc *DictComp) (Object, error) {
	out := NewDict()
	err := i.runComp(dc.Clauses, 0, func() error {
		k, err := i.eval(dc.Key)
		if err != nil {
			return err
		}
		v, err := i.eval(dc.Value)
		if err != nil {
			return err
		}
		out.Set(k, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// runComp 递归展开推导式的多层 for 与 if，并在独立作用域中绑定循环变量
func (i *Interpreter) runComp(clauses []CompClause, depth int, emit func() error) error {
	if depth >= len(clauses) {
		return emit()
	}
	c := clauses[depth]
	iterVal, err := i.eval(c.Iter)
	if err != nil {
		return err
	}
	items, err := iterate(iterVal)
	if err != nil {
		return err
	}
	saved := i.env
	i.env = NewEnvironment(i.env)
	defer func() { i.env = saved }()
	for _, item := range items {
		if err := i.bindTargets(c.Targets, item); err != nil {
			return err
		}
		ok := true
		for _, cond := range c.Ifs {
			cv, err := i.eval(cond)
			if err != nil {
				return err
			}
			if !truthy(cv) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if err := i.runComp(clauses, depth+1, emit); err != nil {
			return err
		}
	}
	return nil
}

// genExprBody 把生成器表达式的 for/if 子句展开为嵌套的 for/if 语句，
// 最内层对元素表达式执行 yield
func genExprBody(el Expr, clauses []CompClause) []Stmt {
	c := clauses[0]
	var inner []Stmt
	if len(clauses) > 1 {
		inner = genExprBody(el, clauses[1:])
	} else {
		inner = []Stmt{&YieldStmt{Value: el}}
	}
	for k := len(c.Ifs) - 1; k >= 0; k-- {
		inner = []Stmt{&IfStmt{Cond: c.Ifs[k], Body: inner}}
	}
	return []Stmt{&ForStmt{Targets: c.Targets, Iter: c.Iter, Body: inner}}
}

// ---------- 下标与切片 ----------

func (i *Interpreter) getItem(obj, key Object) (Object, error) {
	// 实例的 __getitem__ 优先
	if inst, ok := obj.(*Instance); ok {
		if fn, has := inst.Class.LookupMethod("__getitem__"); has {
			return callObjectRef(&Method{Recv: inst, Fn: fn}, []Object{key}, nil)
		}
	}
	switch x := obj.(type) {
	case *List:
		n, ok := intVal(key)
		if !ok {
			return nil, newExc("TypeError", "列表下标必须是整数，实际为 '%s'", typeName(key))
		}
		if n < 0 {
			n += len(x.Items)
		}
		if n < 0 || n >= len(x.Items) {
			return nil, newExc("IndexError", "list index out of range")
		}
		return x.Items[n], nil
	case *Tuple:
		n, ok := intVal(key)
		if !ok {
			return nil, newExc("TypeError", "元组下标必须是整数，实际为 '%s'", typeName(key))
		}
		if n < 0 {
			n += len(x.Items)
		}
		if n < 0 || n >= len(x.Items) {
			return nil, newExc("IndexError", "tuple index out of range")
		}
		return x.Items[n], nil
	case string:
		n, ok := intVal(key)
		if !ok {
			return nil, newExc("TypeError", "字符串下标必须是整数，实际为 '%s'", typeName(key))
		}
		runes := []rune(x)
		if n < 0 {
			n += len(runes)
		}
		if n < 0 || n >= len(runes) {
			return nil, newExc("IndexError", "string index out of range")
		}
		return string(runes[n]), nil
	case *Dict:
		if v, ok := x.Get(key); ok {
			return v, nil
		}
		return nil, newExc("KeyError", "%s", Repr(key))
	case *Range:
		n, ok := intVal(key)
		if !ok {
			return nil, newExc("TypeError", "range 下标必须是整数")
		}
		length := x.Len()
		if n < 0 {
			n += length
		}
		if n < 0 || n >= length {
			return nil, newExc("IndexError", "range object index out of range")
		}
		return x.At(n), nil
	case *PyCounter:
		if v, ok := x.D.Get(key); ok {
			return v, nil
		}
		return 0, nil
	case *PyDefaultDict:
		if v, ok := x.D.Get(key); ok {
			return v, nil
		}
		v, err := callObjectRef(x.Factory, nil, nil)
		if err != nil {
			return nil, err
		}
		x.D.Set(key, v)
		return v, nil
	case *PyOrderedDict:
		if v, ok := x.D.Get(key); ok {
			return v, nil
		}
		return nil, newExc("KeyError", "%s", Repr(key))
	case *PyDeque:
		n, ok := intVal(key)
		if !ok {
			return nil, newExc("TypeError", "deque 下标必须是整数")
		}
		if n < 0 {
			n += len(x.Items)
		}
		if n < 0 || n >= len(x.Items) {
			return nil, newExc("IndexError", "deque index out of range")
		}
		return x.Items[n], nil
	}
	return nil, newExc("TypeError", "'%s' 对象不支持下标访问", typeName(obj))
}

func (i *Interpreter) sliceObject(obj Object, start, stop, step Object) (Object, error) {
	switch x := obj.(type) {
	case *List:
		idx, err := sliceIndices(len(x.Items), start, stop, step)
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(idx))
		for k, n := range idx {
			out[k] = x.Items[n]
		}
		return &List{Items: out}, nil
	case *Tuple:
		idx, err := sliceIndices(len(x.Items), start, stop, step)
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(idx))
		for k, n := range idx {
			out[k] = x.Items[n]
		}
		return &Tuple{Items: out}, nil
	case string:
		runes := []rune(x)
		idx, err := sliceIndices(len(runes), start, stop, step)
		if err != nil {
			return nil, err
		}
		out := make([]rune, len(idx))
		for k, n := range idx {
			out[k] = runes[n]
		}
		return string(out), nil
	case *Range:
		items, err := iterate(x)
		if err != nil {
			return nil, err
		}
		idx, err := sliceIndices(len(items), start, stop, step)
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(idx))
		for k, n := range idx {
			out[k] = items[n]
		}
		return &List{Items: out}, nil
	}
	return nil, newExc("TypeError", "'%s' 对象不支持切片", typeName(obj))
}

// ---------- 属性访问 ----------

func builtinMethodExists(typeStr, name string) bool {
	switch typeStr {
	case "str":
		_, ok := strMethods[name]
		return ok
	case "list":
		_, ok := listMethods[name]
		return ok
	case "dict":
		_, ok := dictMethods[name]
		return ok
	case "tuple":
		_, ok := tupleMethods[name]
		return ok
	case "set":
		_, ok := setMethods[name]
		return ok
	case "Counter":
		_, ok := counterMethods[name]
		if !ok {
			_, ok = dictMethods[name]
		}
		return ok
	case "defaultdict":
		_, ok := dictMethods[name]
		return ok
	case "OrderedDict":
		_, ok := orderedDictMethods[name]
		if !ok {
			_, ok = dictMethods[name]
		}
		return ok
	case "deque":
		_, ok := dequeMethods[name]
		return ok
	}
	return false
}

// unwrapClassAttr 解包类属性中的描述符包装（property/staticmethod/classmethod）
// recv 为实例时绑定 property getter；recv 为 nil 表示通过类访问。
func unwrapClassAttr(owner *Class, recv Object, v Object) (Object, bool) {
	switch w := v.(type) {
	case *Property:
		if recv == nil {
			return w, true
		}
		r, err := callObjectRef(w.Getter, []Object{recv}, nil)
		if err != nil {
			return nil, false
		}
		return r, true
	case *StaticMethod:
		return w.Fn, true
	case *ClassMethod:
		// 绑定动态类：实例访问绑定其实际类型；类访问绑定访问处的类
		switch r := recv.(type) {
		case *Instance:
			return &Method{Recv: r.Class, Fn: w.Fn}, true
		case *Class:
			return &Method{Recv: r, Fn: w.Fn}, true
		}
		return &Method{Recv: owner, Fn: w.Fn}, true
	}
	return v, true
}

func getAttr(obj Object, name string) (Object, error) {
	switch x := obj.(type) {
	case *Instance:
		if v, ok := x.Fields[name]; ok {
			return v, nil
		}
		for _, cur := range x.Class.MRO {
			if fn, ok := cur.Methods[name]; ok {
				return &Method{Recv: obj, Fn: fn}, nil
			}
			if v, ok := cur.Attrs[name]; ok {
				if r, ok2 := unwrapClassAttr(cur, obj, v); ok2 {
					return r, nil
				}
				return nil, newExc("AttributeError", "属性 '%s' 访问失败", name)
			}
		}
		return nil, newExc("AttributeError", "'%s' 对象没有属性 '%s'", x.Class.Name, name)
	case *Module:
		if v, ok := x.Attrs[name]; ok {
			return v, nil
		}
		return nil, newExc("AttributeError", "module '%s' 没有属性 '%s'", x.Name, name)
	case *Class:
		if name == "__name__" || name == "name" {
			return x.Name, nil
		}
		for _, cur := range x.MRO {
			if fn, ok := cur.Methods[name]; ok {
				return fn, nil
			}
			if v, ok := cur.Attrs[name]; ok {
				if r, ok2 := unwrapClassAttr(cur, x, v); ok2 {
					return r, nil
				}
				return nil, newExc("AttributeError", "属性 '%s' 访问失败", name)
			}
		}
		return nil, newExc("AttributeError", "type object '%s' 没有属性 '%s'", x.Name, name)
	case *Function:
		if name == "__name__" || name == "name" {
			return x.Name, nil
		}
		return nil, newExc("AttributeError", "'function' 对象没有属性 '%s'", name)
	case *PyType:
		if name == "__name__" || name == "name" {
			return x.Name, nil
		}
		return nil, newExc("AttributeError", "type 对象没有属性 '%s'", name)
	case *Super:
		if v, ok := x.lookup(name); ok {
			return v, nil
		}
		return nil, newExc("AttributeError", "'super' 对象没有属性 '%s'", name)
	case *PyException:
		switch name {
		case "args":
			return &Tuple{Items: []Object{x.Msg}}, nil
		case "__class__":
			return &PyType{Name: x.ExcType}, nil
		}
		return nil, newExc("AttributeError", "'%s' 对象没有属性 '%s'", x.ExcType, name)
	}
	if builtinMethodExists(typeName(obj), name) {
		return &BuiltinMethod{Recv: obj, Name: name}, nil
	}
	return nil, newExc("AttributeError", "'%s' 对象没有属性 '%s'", typeName(obj), name)
}

// ---------- 调用 ----------

func (i *Interpreter) evalCall(c *Call) (Object, error) {
	var args []Object
	kwargs := map[string]Object{}
	for _, a := range c.Args {
		v, err := i.eval(a.Value)
		if err != nil {
			return nil, err
		}
		switch {
		case a.Star:
			items, err := iterate(v)
			if err != nil {
				return nil, err
			}
			args = append(args, items...)
		case a.Star2:
			d, ok := v.(*Dict)
			if !ok {
				return nil, newExc("TypeError", "** 展开的参数必须是字典，实际为 '%s'", typeName(v))
			}
			for _, k := range d.Keys {
				ks, ok := k.(string)
				if !ok {
					return nil, newExc("TypeError", "** 展开的键必须是字符串")
				}
				kwargs[ks] = d.Vals[keyOf(k)]
			}
		case a.Name != "":
			kwargs[a.Name] = v
		default:
			args = append(args, v)
		}
	}
	fn, err := i.eval(c.Func)
	if err != nil {
		return nil, err
	}
	return i.callObject(fn, args, kwargs)
}

// callObject 统一分发所有可调用对象
func callObject(fn Object, args []Object, kwargs map[string]Object) (Object, error) {
	if activeInterp == nil {
		return nil, newExc("RuntimeError", "解释器尚未初始化")
	}
	return activeInterp.callObject(fn, args, kwargs)
}

// callObjectRef 是 callObject 的间接引用。builtins.go 中的包级变量
// （方法表、内置函数表）通过它调用用户函数，从而避免包级初始化循环。
var callObjectRef func(fn Object, args []Object, kwargs map[string]Object) (Object, error)

func init() {
	callObjectRef = callObject
}

func (i *Interpreter) callObject(fn Object, args []Object, kwargs map[string]Object) (Object, error) {
	switch f := fn.(type) {
	case *Function:
		return i.callFunction(f, args, kwargs)
	case *Builtin:
		return f.Fn(args, kwargs)
	case *Method:
		all := make([]Object, 0, len(args)+1)
		all = append(all, f.Recv)
		all = append(all, args...)
		return i.callFunction(f.Fn, all, kwargs)
	case *BuiltinMethod:
		return callBuiltinMethod(f.Recv, f.Name, args, kwargs)
	case *Class:
		return i.instantiate(f, args, kwargs)
	case *Instance:
		// 实例可调用：走 __call__ 方法
		if fn, ok := f.Class.LookupMethod("__call__"); ok {
			all := make([]Object, 0, len(args)+1)
			all = append(all, f)
			all = append(all, args...)
			return i.callFunction(fn, all, kwargs)
		}
		return nil, newExc("TypeError", "'%s' 对象不可调用", f.Class.Name)
	case *PyType:
		if isExceptionTypeName(f.Name) {
			msg := ""
			if len(args) > 0 {
				msg = Str(args[0])
			}
			return nil, &PyException{ExcType: f.Name, Msg: msg}
		}
		if b, ok := builtinFuncs[strings.ToLower(f.Name)]; ok {
			return b(args, kwargs)
		}
		return nil, newExc("TypeError", "'%s' 对象不可调用", f.Name)
	}
	return nil, newExc("TypeError", "'%s' 对象不可调用", typeName(fn))
}

func (i *Interpreter) callFunction(fn *Function, args []Object, kwargs map[string]Object) (Object, error) {
	if fn.IsGen {
		// 生成器调用只创建生成器对象，函数体在首次 next() 时才执行
		return &Generator{Fn: fn, Args: args, Kwargs: kwargs}, nil
	}
	if i.callDepth >= 200 {
		return nil, newExc("RecursionError", "超过最大递归深度")
	}
	local := NewEnvironment(fn.Env)
	if err := i.bindParams(fn, local, args, kwargs); err != nil {
		return nil, err
	}
	saved := i.env
	i.env = local
	i.callDepth++
	i.frames = append(i.frames, &callFrame{fn: fn, local: local})
	sig, err := i.execBlock(fn.Body)
	i.frames = i.frames[:len(i.frames)-1]
	i.callDepth--
	i.env = saved
	if err != nil {
		return nil, err
	}
	if sig != nil && sig.kind == sigReturn {
		return sig.val, nil
	}
	return None, nil
}

// bindParams 把实参绑定到函数的局部作用域（位置/关键字/默认值/*args/**kwargs）
func (i *Interpreter) bindParams(fn *Function, local *Environment, args []Object, kwargs map[string]Object) error {
	pi := 0
	for idx, p := range fn.Params {
		if p.Star || p.Star2 {
			continue
		}
		if pi < len(args) {
			if _, ok := kwargs[p.Name]; ok {
				return newExc("TypeError", "%s() 的参数 '%s' 同时收到了位置值与关键字值", fn.Name, p.Name)
			}
			local.Set(p.Name, args[pi])
			pi++
			continue
		}
		// 关键字参数优先于默认值
		if v, ok := kwargs[p.Name]; ok {
			local.Set(p.Name, v)
			continue
		}
		if idx < len(fn.Defaults) && fn.Defaults[idx] != nil {
			local.Set(p.Name, fn.Defaults[idx])
			continue
		}
		return newExc("TypeError", "%s() 缺少必要的位置参数: '%s'", fn.Name, p.Name)
	}
	hasStar := false
	for _, p := range fn.Params {
		if p.Star {
			hasStar = true
			rest := make([]Object, 0)
			for ; pi < len(args); pi++ {
				rest = append(rest, args[pi])
			}
			local.Set(p.Name, &Tuple{Items: rest})
		} else if p.Star2 {
			d := NewDict()
			for k, v := range kwargs {
				d.Set(k, v)
			}
			local.Set(p.Name, d)
		}
	}
	if pi < len(args) && !hasStar {
		return newExc("TypeError", "%s() 只接受 %d 个位置参数，但传入了 %d 个", fn.Name, pi, len(args))
	}
	return nil
}

// ---------- 生成器协程驱动 ----------

// generatorIterateHook 供 objects.go 的 iterate 物化生成器，打破包级初始化循环
var generatorIterateHook func(g *Generator) ([]Object, error)

func init() {
	generatorIterateHook = func(g *Generator) ([]Object, error) {
		var out []Object
		for {
			v, ok, err := activeInterp.genNext(g)
			if err != nil {
				return nil, err
			}
			if !ok {
				return out, nil
			}
			out = append(out, v)
		}
	}
}

// genNext 拉取生成器的下一个值；耗尽时 ok 为 false。
// 调用方与生成器 goroutine 通过无缓冲信道交替执行，解释器状态
// （i.env / i.callDepth）在每次交接时恢复为调用方的现场。
// genNextRef 供内置 next() 间接调用 genNext，避免 builtinFuncs 的初始化循环
var genNextRef func(i *Interpreter, g *Generator) (Object, bool, error)

func init() {
	genNextRef = (*Interpreter).genNext
}

func (i *Interpreter) genNext(g *Generator) (Object, bool, error) {
	if g.finished {
		// 耗尽：ok 为 false 且不报错（list()/for 遍历得到空；
		// 内置 next() 负责把 !ok 转换为 StopIteration）
		return nil, false, nil
	}
	// 列表迭代器模式（iter() 内建函数创建）
	if g.Fn == nil {
		if g.pos < len(g.Items) {
			v := g.Items[g.pos]
			g.pos++
			return v, true, nil
		}
		g.finished = true
		return nil, false, nil
	}
	callerEnv := i.env
	callerDepth := i.callDepth
	if !g.started {
		g.started = true
		g.ch = make(chan genEvent)
		g.resume = make(chan bool)
		go func() {
			ev := i.runGenBody(g)
			g.ch <- ev
		}()
	} else {
		g.resume <- true
	}
	ev := <-g.ch
	i.env = callerEnv
	i.callDepth = callerDepth
	if ev.err != nil {
		g.finished = true
		return nil, false, ev.err
	}
	if ev.done {
		g.finished = true
		return nil, false, nil
	}
	return ev.val, true, nil
}

// runGenBody 在生成器 goroutine 中执行函数体（在首次 next() 时开始）。
// yield 事件由 YieldStmt 在执行现场直接发送；本函数返回最终事件（结束或异常）。
func (i *Interpreter) runGenBody(g *Generator) genEvent {
	savedGen := i.currentGen
	i.currentGen = g
	local := NewEnvironment(g.Fn.Env)
	if err := i.bindParams(g.Fn, local, g.Args, g.Kwargs); err != nil {
		i.currentGen = savedGen
		return genEvent{err: err}
	}
	g.localEnv = local
	g.depth = i.callDepth
	i.env = local
	i.callDepth++
	i.frames = append(i.frames, &callFrame{fn: g.Fn, local: local})
	_, err := i.execBlock(g.Fn.Body)
	i.frames = i.frames[:len(i.frames)-1]
	i.callDepth--
	i.currentGen = savedGen
	if err != nil {
		if pe, ok := err.(*PyException); ok {
			return genEvent{err: pe}
		}
		return genEvent{err: err}
	}
	return genEvent{done: true}
}

// containsYield 判断语句块中是否直接包含 yield（不进入嵌套的 def/class/装饰器）
func containsYield(stmts []Stmt) bool {
	for _, s := range stmts {
		switch t := s.(type) {
		case *YieldStmt:
			return true
		case *IfStmt:
			if containsYield(t.Body) || containsYield(t.Else) {
				return true
			}
			for _, el := range t.Elifs {
				if containsYield(el.Body) {
					return true
				}
			}
		case *WhileStmt:
			if containsYield(t.Body) || containsYield(t.Else) {
				return true
			}
		case *ForStmt:
			if containsYield(t.Body) || containsYield(t.Else) {
				return true
			}
		case *TryStmt:
			if containsYield(t.Body) || containsYield(t.Else) || containsYield(t.Finally) {
				return true
			}
			for _, h := range t.Handlers {
				if containsYield(h.Body) {
					return true
				}
			}
		case *WithStmt:
			if containsYield(t.Body) {
				return true
			}
		}
	}
	return false
}
