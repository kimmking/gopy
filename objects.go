package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ============ 运行时对象 ============
// Object 使用 Go 原生类型表示基础值（int / float64 / bool / string），
// 容器与可调用对象使用指针类型，从而天然具备引用语义。

type Object interface{}

type PyNone struct{}

// None 是 Python 的 None 单例
var None Object = &PyNone{}

type List struct{ Items []Object }

type Tuple struct{ Items []Object }

// Dict 保持插入顺序（Python 3.7+ 字典有序）
type Dict struct {
	Keys []Object
	Vals map[string]Object
}

func NewDict() *Dict {
	return &Dict{Vals: map[string]Object{}}
}

func (d *Dict) Set(k, v Object) {
	key := keyOf(k)
	if _, ok := d.Vals[key]; !ok {
		d.Keys = append(d.Keys, k)
	}
	d.Vals[key] = v
}

func (d *Dict) Get(k Object) (Object, bool) {
	v, ok := d.Vals[keyOf(k)]
	return v, ok
}

func (d *Dict) Delete(k Object) {
	key := keyOf(k)
	if _, ok := d.Vals[key]; !ok {
		return
	}
	delete(d.Vals, key)
	for i, kk := range d.Keys {
		if keyOf(kk) == key {
			d.Keys = append(d.Keys[:i], d.Keys[i+1:]...)
			break
		}
	}
}

func (d *Dict) Len() int { return len(d.Keys) }

// Set 是保持插入顺序的集合
type Set struct {
	Vals  map[string]Object
	Order []Object
}

func NewSet() *Set {
	return &Set{Vals: map[string]Object{}}
}

func (s *Set) Add(v Object) {
	k := keyOf(v)
	if _, ok := s.Vals[k]; ok {
		return
	}
	s.Vals[k] = v
	s.Order = append(s.Order, v)
}

func (s *Set) Has(v Object) bool {
	_, ok := s.Vals[keyOf(v)]
	return ok
}

func (s *Set) Remove(v Object) {
	k := keyOf(v)
	if _, ok := s.Vals[k]; !ok {
		return
	}
	delete(s.Vals, k)
	for i, o := range s.Order {
		if keyOf(o) == k {
			s.Order = append(s.Order[:i], s.Order[i+1:]...)
			break
		}
	}
}

func (s *Set) Len() int { return len(s.Order) }

// Range 惰性整数区间
type Range struct{ Start, Stop, Step int }

func (r *Range) Len() int {
	if r.Step > 0 && r.Start >= r.Stop {
		return 0
	}
	if r.Step < 0 && r.Start <= r.Stop {
		return 0
	}
	n := (r.Stop - r.Start) / r.Step
	if (r.Stop-r.Start)%r.Step != 0 {
		n++
	}
	if n < 0 {
		return 0
	}
	return n
}

func (r *Range) At(i int) int { return r.Start + i*r.Step }

// Function 用户自定义函数，Env 即闭包环境
type Function struct {
	Name     string
	Params   []Param
	Body     []Stmt
	Env      *Environment
	Defaults []Object
	// IsGen 标记函数体包含 yield，调用时返回 Generator 而不立即执行
	IsGen bool
	// DefClass 记录方法定义所在的类（供零参 super() 使用）
	DefClass *Class
}

// genEvent 是生成器在信道上传递的事件：让步值 / 结束 / 异常
type genEvent struct {
	val  Object
	err  error
	done bool
}

// Generator 惰性生成器：函数体在独立 goroutine 中执行，
// yield 通过无缓冲信道与 next() 调用方同步（协程式交替执行）。
// Fn 为 nil 时退化为对固定 Items 的迭代器（由内置 iter() 创建）。
type Generator struct {
	Fn      *Function
	Args    []Object
	Kwargs  map[string]Object
	Items   []Object
	pos     int
	ch      chan genEvent
	resume  chan bool
	started bool
	finished bool
	localEnv *Environment
	depth    int
}

type BuiltinFn func(args []Object, kwargs map[string]Object) (Object, error)

type Builtin struct {
	Name string
	Fn   BuiltinFn
}

// Method 绑定到实例的用户方法
type Method struct {
	Recv Object
	Fn   *Function
}

// Property @property 包装的 getter
type Property struct{ Getter *Function }

// StaticMethod @staticmethod 包装的函数（访问时不绑定 self）
type StaticMethod struct{ Fn *Function }

// ClassMethod @classmethod 包装的函数（访问时绑定类对象）
type ClassMethod struct{ Fn *Function }

// BuiltinMethod 绑定到内建类型实例的方法
type BuiltinMethod struct {
	Recv Object
	Name string
}

// Class 类对象，Bases 支持多继承，MRO 为 C3 线性化结果
type Class struct {
	Name    string
	Bases   []*Class
	MRO     []*Class
	Methods map[string]*Function
	Attrs   map[string]Object
}

// LookupMethod 沿 MRO 查找方法
func (c *Class) LookupMethod(name string) (*Function, bool) {
	for _, cur := range c.MRO {
		if fn, ok := cur.Methods[name]; ok {
			return fn, true
		}
	}
	return nil, false
}

// LookupAttr 沿 MRO 查找类属性
func (c *Class) LookupAttr(name string) (Object, bool) {
	for _, cur := range c.MRO {
		if v, ok := cur.Attrs[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// computeMRO 计算 C3 线性化：L(C) = C + merge(L(B1), ..., [B1, B2, ...])
func computeMRO(cls *Class, bases []*Class) ([]*Class, error) {
	if len(bases) == 1 {
		return append([]*Class{cls}, bases[0].MRO...), nil
	}
	if len(bases) == 0 {
		return []*Class{cls}, nil
	}
	seqs := make([][]*Class, 0, len(bases)+2)
	for _, b := range bases {
		seqs = append(seqs, b.MRO)
	}
	headSeq := make([]*Class, len(bases))
	copy(headSeq, bases)
	seqs = append(seqs, headSeq, []*Class{cls})
	out := []*Class{cls}
	for {
		anyLeft := false
		for _, s := range seqs {
			if len(s) > 0 {
				anyLeft = true
				break
			}
		}
		if !anyLeft {
			return out, nil
		}
		// 找一个不出现在任何序列尾部的头部候选
		var cand *Class
		for _, s := range seqs {
			if len(s) == 0 {
				continue
			}
			c := s[0]
			inTail := false
			for _, s2 := range seqs {
				for k := 1; k < len(s2); k++ {
					if s2[k] == c {
						inTail = true
						break
					}
				}
				if inTail {
					break
				}
			}
			if !inTail {
				cand = c
				break
			}
		}
		if cand == nil {
			return nil, newExc("TypeError", "Cannot create a consistent method resolution order (MRO) for bases %s", cls.Name)
		}
		out = append(out, cand)
		for k, s := range seqs {
			if len(s) > 0 && s[0] == cand {
				rest := make([]*Class, len(s)-1)
				copy(rest, s[1:])
				seqs[k] = rest
			}
		}
	}
}

// Super 零参 super() 的返回值：沿 Obj 实际类型的 MRO 中 Cls 之后的基类查找
type Super struct {
	Cls *Class
	Obj Object
}

// superLookup 在 Obj 类型的 MRO 中从 Cls 之后开始查找
func (s *Super) lookup(name string) (Object, bool) {
	var mro []*Class
	switch o := s.Obj.(type) {
	case *Instance:
		mro = o.Class.MRO
	case *Class:
		mro = o.MRO
	default:
		return nil, false
	}
	start := 1
	for k, c := range mro {
		if c == s.Cls {
			start = k + 1
			break
		}
	}
	for k := start; k < len(mro); k++ {
		cur := mro[k]
		if fn, ok := cur.Methods[name]; ok {
			return &Method{Recv: s.Obj, Fn: fn}, true
		}
		if v, ok := cur.Attrs[name]; ok {
			if r, ok2 := unwrapClassAttr(cur, s.Obj, v); ok2 {
				return r, true
			}
		}
	}
	return nil, false
}

// Instance 类的实例
type Instance struct {
	Class  *Class
	Fields map[string]Object
}

// Module 内置模块
type Module struct {
	Name  string
	Attrs map[string]Object
}

// PyCounter collections.Counter：底层为 *Dict
type PyCounter struct{ D *Dict }

// PyDefaultDict collections.defaultdict
type PyDefaultDict struct {
	D       *Dict
	Factory Object
}

// PyOrderedDict collections.OrderedDict
type PyOrderedDict struct{ D *Dict }

// PyDeque collections.deque
type PyDeque struct{ Items []Object }

// PyType 表示类型对象（内建类型与异常类）
type PyType struct {
	Name string
}

// ============ 环境（作用域链） ============

type Environment struct {
	vars   map[string]Object
	parent *Environment
	global *Environment
	// declaredGlobal 记录本帧中被 global 声明的名字
	declaredGlobal map[string]bool
	// declaredNonlocal 记录本帧中被 nonlocal 声明的名字
	declaredNonlocal map[string]bool
}

func NewEnvironment(parent *Environment) *Environment {
	e := &Environment{vars: map[string]Object{}, parent: parent}
	if parent != nil {
		e.global = parent.global
	} else {
		e.global = e
	}
	return e
}

// Get 沿作用域链查找变量
func (e *Environment) Get(name string) (Object, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if v, ok := cur.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// Set 采用 Python 语义：赋值在当前作用域创建/更新绑定
func (e *Environment) Set(name string, v Object) {
	e.vars[name] = v
}

// SetGlobal 在全局作用域赋值（global 声明）
func (e *Environment) SetGlobal(name string, v Object) {
	e.global.vars[name] = v
}

// Delete 沿作用域链删除一个绑定
func (e *Environment) Delete(name string) bool {
	for cur := e; cur != nil; cur = cur.parent {
		if _, ok := cur.vars[name]; ok {
			delete(cur.vars, name)
			return true
		}
	}
	return false
}

// ============ 异常 ============

type PyException struct {
	ExcType string
	Msg     string
	Trace   []string
}

func (e *PyException) Error() string {
	if e.Msg == "" {
		return e.ExcType
	}
	return e.ExcType + ": " + e.Msg
}

func newExc(t, format string, args ...interface{}) error {
	return &PyException{ExcType: t, Msg: fmt.Sprintf(format, args...)}
}

// ============ 类型转换与判定 ============

func typeName(v Object) string {
	switch x := v.(type) {
	case *PyNone:
		return "NoneType"
	case bool:
		return "bool"
	case int:
		return "int"
	case float64:
		return "float"
	case string:
		return "str"
	case *List:
		return "list"
	case *Tuple:
		return "tuple"
	case *Dict:
		return "dict"
	case *Set:
		return "set"
	case *Range:
		return "range"
	case *Function:
		return "function"
	case *Generator:
		return "generator"
	case *Builtin:
		return "builtin_function_or_method"
	case *Method:
		return "method"
	case *BuiltinMethod:
		return "method"
	case *Property:
		return "property"
	case *StaticMethod:
		return "function"
	case *ClassMethod:
		return "bound method"
	case *Super:
		return "super"
	case *Class:
		return "type"
	case *PyType:
		return "type"
	case *PyException:
		return x.ExcType
	case *Instance:
		return x.Class.Name
	case *Module:
		return "module"
	case *PyCounter:
		return "Counter"
	case *PyDefaultDict:
		return "defaultdict"
	case *PyOrderedDict:
		return "OrderedDict"
	case *PyDeque:
		return "deque"
	}
	return "object"
}

// numVal 返回数值视图，非数值返回 false
func numVal(v Object) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case float64:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// intVal 返回整数视图，bool 也按 Python 规则参与整数运算
func intVal(v Object) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func toFloat(v Object) (float64, bool) {
	if f, ok := numVal(v); ok {
		return f, true
	}
	if s, ok := v.(string); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func truthy(v Object) bool {
	switch x := v.(type) {
	case *PyNone:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case float64:
		return x != 0
	case string:
		return len(x) > 0
	case *List:
		return len(x.Items) > 0
	case *Tuple:
		return len(x.Items) > 0
	case *Dict:
		return x.Len() > 0
	case *Set:
		return x.Len() > 0
	case *Range:
		return x.Len() > 0
	case *Instance:
		// 实例的真值由 __bool__ 决定（缺省恒真）
		if m, has := instanceBoolFn(x); has {
			v, err := callObjectRef(m, nil, nil)
			if err != nil {
				return true
			}
			return truthy(v)
		}
	}
	return true
}

// instanceBoolFn 查找实例的 __bool__ 方法
func instanceBoolFn(inst *Instance) (*Method, bool) {
	fn, ok := inst.Class.LookupMethod("__bool__")
	if !ok {
		return nil, false
	}
	return &Method{Recv: inst, Fn: fn}, true
}

// isBasicValue 判断是否为非容器的基础值
func isBasicValue(v Object) bool {
	switch v.(type) {
	case int, float64, bool, string:
		return true
	}
	return false
}

// keyOf 生成 dict / set 的键；数值按其值归一化，符合 Python 的 1 == 1.0
func keyOf(v Object) string {
	if f, ok := numVal(v); ok {
		return "n|" + formatFloat(f)
	}
	return typeName(v) + "|" + Repr(v)
}

// ============ 输出格式化 ============

func formatFloat(f float64) string {
	if math.IsNaN(f) {
		return "nan"
	}
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e16 {
		return fmt.Sprintf("%.1f", f)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func escapeRepr(s string, q byte) string {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			sb.WriteString("\\n")
		case '\t':
			sb.WriteString("\\t")
		case '\r':
			sb.WriteString("\\r")
		case '\\':
			sb.WriteString("\\\\")
		default:
			if byte(r) == q && r < 128 {
				sb.WriteByte('\\')
			}
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// quoteString 生成 Python 风格的字符串字面量（优先单引号）
func quoteString(s string) string {
	hasSingle := strings.ContainsRune(s, '\'')
	hasDouble := strings.ContainsRune(s, '"')
	q := byte('\'')
	if hasSingle && !hasDouble {
		q = '"'
	}
	return string(q) + escapeRepr(s, q) + string(q)
}

var addrCounter int

func fakeAddr(prefix string) string {
	addrCounter += 0x10
	return fmt.Sprintf("0x%08x", 0x1000+addrCounter)
}

// instanceStrHook 由 interpreter.go 在 init 中注入，用于调用实例的 __str__/__repr__。
// 通过函数变量而非直接调用，避免 Repr 与求值器之间形成包级初始化循环。
var instanceStrHook func(inst *Instance, dunder string) (string, bool)

// instanceRepr 实现 repr(obj)：优先 __repr__，否则默认对象表示
func instanceRepr(inst *Instance) string {
	if instanceStrHook != nil {
		if s, ok := instanceStrHook(inst, "__repr__"); ok {
			return s
		}
	}
	return "<" + inst.Class.Name + " object at " + fakeAddr("obj") + ">"
}

// instanceToString 实现 str(obj)：优先 __str__，否则回退到 repr
func instanceToString(inst *Instance) string {
	if instanceStrHook != nil {
		if s, ok := instanceStrHook(inst, "__str__"); ok {
			return s
		}
	}
	return instanceRepr(inst)
}

// factoryRepr 输出 defaultdict 工厂的 CPython 风格表示：内建类型显示为 <class 'x'>
func factoryRepr(f Object) string {
	if b, ok := f.(*Builtin); ok {
		switch b.Name {
		case "int", "float", "str", "bool", "list", "tuple", "dict", "set":
			return "<class '" + b.Name + "'>"
		}
	}
	return Repr(f)
}

// mostCommon 返回按计数降序（稳定）排列的 (key, count) 元组列表
func (c *PyCounter) mostCommon() []Object {
	type pair struct {
		idx int
		t   *Tuple
	}
	pairs := make([]pair, 0, c.D.Len())
	for i, k := range c.D.Keys {
		v := c.D.Vals[keyOf(k)]
		pairs = append(pairs, pair{i, &Tuple{Items: []Object{k, v}}})
	}
	sort.SliceStable(pairs, func(a, b int) bool {
		ca, _ := numVal(pairs[a].t.Items[1])
		cb, _ := numVal(pairs[b].t.Items[1])
		return ca > cb
	})
	out := make([]Object, len(pairs))
	for i, p := range pairs {
		out[i] = p.t
	}
	return out
}

// Repr 生成 Python repr()
func Repr(v Object) string {
	switch x := v.(type) {
	case *PyNone:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(x)
	case float64:
		return formatFloat(x)
	case string:
		return quoteString(x)
	case *List:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			parts[i] = Repr(it)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *Tuple:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			parts[i] = Repr(it)
		}
		if len(parts) == 1 {
			return "(" + parts[0] + ",)"
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *Dict:
		parts := make([]string, 0, x.Len())
		for _, k := range x.Keys {
			parts = append(parts, Repr(k)+": "+Repr(x.Vals[keyOf(k)]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *Set:
		if x.Len() == 0 {
			return "set()"
		}
		parts := make([]string, 0, x.Len())
		for _, it := range x.Order {
			parts = append(parts, Repr(it))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *Range:
		if x.Step == 1 {
			return fmt.Sprintf("range(%d, %d)", x.Start, x.Stop)
		}
		return fmt.Sprintf("range(%d, %d, %d)", x.Start, x.Stop, x.Step)
	case *Function:
		return "<function " + x.Name + " at " + fakeAddr("fn") + ">"
	case *Generator:
		return "<generator object " + x.Fn.Name + " at " + fakeAddr("gen") + ">"
	case *Builtin:
		return "<built-in function " + x.Name + ">"
	case *Method:
		return "<bound method " + x.Fn.Name + " of " + Repr(x.Recv) + ">"
	case *BuiltinMethod:
		return "<built-in method " + x.Name + " of " + typeName(x.Recv) + " object>"
	case *Class:
		return "<class '" + x.Name + "'>"
	case *PyType:
		return "<class '" + x.Name + "'>"
	case *Instance:
		return instanceRepr(x)
	case *Module:
		return "<module '" + x.Name + "'>"
	case *PyCounter:
		parts := make([]string, 0)
		for _, kv := range x.mostCommon() {
			pair := kv.(*Tuple).Items
			parts = append(parts, Repr(pair[0])+": "+Repr(pair[1]))
		}
		return "Counter({" + strings.Join(parts, ", ") + "})"
	case *PyDefaultDict:
		return "defaultdict(" + factoryRepr(x.Factory) + ", " + Repr(x.D) + ")"
	case *PyOrderedDict:
		parts := make([]string, 0, x.D.Len())
		for _, k := range x.D.Keys {
			parts = append(parts, "("+Repr(k)+", "+Repr(x.D.Vals[keyOf(k)])+")")
		}
		return "OrderedDict([" + strings.Join(parts, ", ") + "])"
	case *PyDeque:
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			parts[i] = Repr(it)
		}
		return "deque([" + strings.Join(parts, ", ") + "])"
	case *PyException:
		if x.Msg == "" {
			return x.ExcType + "()"
		}
		return x.ExcType + "(" + quoteString(x.Msg) + ")"
	}
	return fmt.Sprintf("%v", v)
}

// Str 生成 Python str()：字符串本身不加引号，异常只输出消息，实例优先 __str__，容器沿用 repr
func Str(v Object) string {
	switch x := v.(type) {
	case string:
		return x
	case *PyException:
		return x.Msg
	case *Instance:
		return instanceToString(x)
	}
	return Repr(v)
}

// ============ 比较与相等 ============

func objectsEqual(a, b Object) bool {
	af, aok := numVal(a)
	bf, bok := numVal(b)
	if aok && bok {
		return af == bf
	}
	switch x := a.(type) {
	case *PyNone:
		_, ok := b.(*PyNone)
		return ok
	case string:
		y, ok := b.(string)
		return ok && x == y
	case *List:
		y, ok := b.(*List)
		if !ok || len(x.Items) != len(y.Items) {
			return false
		}
		for i := range x.Items {
			if !objectsEqual(x.Items[i], y.Items[i]) {
				return false
			}
		}
		return true
	case *Tuple:
		y, ok := b.(*Tuple)
		if !ok || len(x.Items) != len(y.Items) {
			return false
		}
		for i := range x.Items {
			if !objectsEqual(x.Items[i], y.Items[i]) {
				return false
			}
		}
		return true
	case *Set:
		y, ok := b.(*Set)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, it := range x.Order {
			if !y.Has(it) {
				return false
			}
		}
		return true
	case *Dict:
		y, ok := b.(*Dict)
		if !ok || x.Len() != y.Len() {
			return false
		}
		for _, k := range x.Keys {
			yv, ok := y.Get(k)
			if !ok || !objectsEqual(x.Vals[keyOf(k)], yv) {
				return false
			}
		}
		return true
	}
	return false
}

// compareValues 返回 -1 / 0 / 1，无法比较时 ok 为 false
func compareValues(a, b Object) (int, bool) {
	af, aok := numVal(a)
	bf, bok := numVal(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1, true
		case af > bf:
			return 1, true
		}
		return 0, true
	}
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), true
		}
	}
	if x, ok := a.(*List); ok {
		if y, ok := b.(*List); ok {
			n := len(x.Items)
			if len(y.Items) < n {
				n = len(y.Items)
			}
			for i := 0; i < n; i++ {
				c, ok := compareValues(x.Items[i], y.Items[i])
				if !ok || c != 0 {
					return c, ok
				}
			}
			switch {
			case len(x.Items) < len(y.Items):
				return -1, true
			case len(x.Items) > len(y.Items):
				return 1, true
			}
			return 0, true
		}
	}
	return 0, false
}

// ============ 运算符 ============

// dunderOpName 把算术运算符映射到双下划线方法名
func dunderOpName(op string) string {
	switch op {
	case "+":
		return "add"
	case "-":
		return "sub"
	case "*":
		return "mul"
	case "/":
		return "truediv"
	case "//":
		return "floordiv"
	case "%":
		return "mod"
	case "**":
		return "pow"
	}
	return ""
}

// instanceBinOp 尝试用实例的运算符重载方法执行二元运算；
// 先试左操作数的 __op__，再试右操作数的 __rop__。命中返回结果。
func instanceBinOp(op string, l, r Object) (Object, bool, error) {
	name := dunderOpName(op)
	if name == "" {
		return nil, false, nil
	}
	if li, ok := l.(*Instance); ok {
		if fn, ok2 := li.Class.LookupMethod("__" + name + "__"); ok2 {
			v, err := callObjectRef(&Method{Recv: li, Fn: fn}, []Object{r}, nil)
			return v, true, err
		}
	}
	if ri, ok := r.(*Instance); ok {
		if fn, ok2 := ri.Class.LookupMethod("__r" + name + "__"); ok2 {
			v, err := callObjectRef(&Method{Recv: ri, Fn: fn}, []Object{l}, nil)
			return v, true, err
		}
	}
	return nil, false, nil
}

func binaryOp(op string, l, r Object) (Object, error) {
	// 实例的运算符重载优先于内建路径
	if _, isL := l.(*Instance); isL {
		if v, hit, err := instanceBinOp(op, l, r); hit {
			return v, err
		}
	} else if _, isR := r.(*Instance); isR {
		if v, hit, err := instanceBinOp(op, l, r); hit {
			return v, err
		}
	}
	switch op {
	case "+":
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if lok && rok {
			return li + ri, nil
		}
		lf, lok2 := numVal(l)
		rf, rok2 := numVal(r)
		if lok2 && rok2 {
			return lf + rf, nil
		}
		if ls, ok := l.(string); ok {
			if rs, ok := r.(string); ok {
				return ls + rs, nil
			}
		}
		if ll, ok := l.(*List); ok {
			if rl, ok := r.(*List); ok {
				out := make([]Object, 0, len(ll.Items)+len(rl.Items))
				out = append(out, ll.Items...)
				out = append(out, rl.Items...)
				return &List{Items: out}, nil
			}
		}
		if lt, ok := l.(*Tuple); ok {
			if rt, ok := r.(*Tuple); ok {
				out := make([]Object, 0, len(lt.Items)+len(rt.Items))
				out = append(out, lt.Items...)
				out = append(out, rt.Items...)
				return &Tuple{Items: out}, nil
			}
		}
		return nil, newExc("TypeError", "不支持的运算 +: '%s' 和 '%s'", typeName(l), typeName(r))

	case "-":
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if lok && rok {
			return li - ri, nil
		}
		lf, lok2 := numVal(l)
		rf, rok2 := numVal(r)
		if lok2 && rok2 {
			return lf - rf, nil
		}
		return nil, newExc("TypeError", "不支持的运算 -: '%s' 和 '%s'", typeName(l), typeName(r))

	case "*":
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if lok && rok {
			return li * ri, nil
		}
		lf, lok2 := numVal(l)
		rf, rok2 := numVal(r)
		if lok2 && rok2 {
			return lf * rf, nil
		}
		// 序列重复
		if s, ok := l.(string); ok {
			if n, ok := intVal(r); ok {
				if n <= 0 {
					return "", nil
				}
				return strings.Repeat(s, n), nil
			}
		}
		if n, ok := intVal(l); ok {
			if s, ok := r.(string); ok {
				if n <= 0 {
					return "", nil
				}
				return strings.Repeat(s, n), nil
			}
		}
		if seq, ok := l.(*List); ok {
			if n, ok := intVal(r); ok {
				out := make([]Object, 0, len(seq.Items)*maxInt(n, 0))
				for i := 0; i < n; i++ {
					out = append(out, seq.Items...)
				}
				return &List{Items: out}, nil
			}
		}
		return nil, newExc("TypeError", "不支持的运算 *: '%s' 和 '%s'", typeName(l), typeName(r))

	case "/":
		lf, lok := numVal(l)
		rf, rok := numVal(r)
		if !lok || !rok {
			return nil, newExc("TypeError", "不支持的运算 /: '%s' 和 '%s'", typeName(l), typeName(r))
		}
		if rf == 0 {
			return nil, newExc("ZeroDivisionError", "division by zero")
		}
		return lf / rf, nil

	case "//":
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if lok && rok {
			if ri == 0 {
				return nil, newExc("ZeroDivisionError", "integer division or modulo by zero")
			}
			q := li / ri
			if (li%ri != 0) && ((li < 0) != (ri < 0)) {
				q--
			}
			return q, nil
		}
		lf, lok2 := numVal(l)
		rf, rok2 := numVal(r)
		if !lok2 || !rok2 {
			return nil, newExc("TypeError", "不支持的运算 //: '%s' 和 '%s'", typeName(l), typeName(r))
		}
		if rf == 0 {
			return nil, newExc("ZeroDivisionError", "float division by zero")
		}
		return math.Floor(lf / rf), nil

	case "%":
		if s, ok := l.(string); ok {
			return percentFormat(s, r)
		}
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if lok && rok {
			if ri == 0 {
				return nil, newExc("ZeroDivisionError", "integer division or modulo by zero")
			}
			m := li % ri
			if m != 0 && (m < 0) != (ri < 0) {
				m += ri
			}
			return m, nil
		}
		lf, lok2 := numVal(l)
		rf, rok2 := numVal(r)
		if !lok2 || !rok2 {
			return nil, newExc("TypeError", "不支持的运算 %%: '%s' 和 '%s'", typeName(l), typeName(r))
		}
		if rf == 0 {
			return nil, newExc("ZeroDivisionError", "float modulo by zero")
		}
		m := math.Mod(lf, rf)
		if m != 0 && (m < 0) != (rf < 0) {
			m += rf
		}
		return m, nil

	case "**":
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if lok && rok && ri >= 0 {
			return intPow(li, ri), nil
		}
		lf, lok2 := numVal(l)
		rf, rok2 := numVal(r)
		if !lok2 || !rok2 {
			return nil, newExc("TypeError", "不支持的运算 **: '%s' 和 '%s'", typeName(l), typeName(r))
		}
		if lf == 0 && rf < 0 {
			return nil, newExc("ZeroDivisionError", "0.0 cannot be raised to a negative power")
		}
		return math.Pow(lf, rf), nil

	case "&", "|", "^", "<<", ">>":
		li, lok := intVal(l)
		ri, rok := intVal(r)
		if !lok || !rok {
			return nil, newExc("TypeError", "位运算只支持整数: '%s' 和 '%s'", typeName(l), typeName(r))
		}
		switch op {
		case "&":
			return li & ri, nil
		case "|":
			return li | ri, nil
		case "^":
			return li ^ ri, nil
		case "<<":
			if ri < 0 {
				return nil, newExc("ValueError", "负数位移量")
			}
			return li << uint(ri), nil
		case ">>":
			if ri < 0 {
				return nil, newExc("ValueError", "负数位移量")
			}
			return li >> uint(ri), nil
		}
	}
	return nil, newExc("SyntaxError", "未知运算符 %s", op)
}

// ============ printf 风格字符串格式化（% 运算符） ============

// padFieldA 按宽度与对齐方式填充字段。
// align: '<' 左对齐, '>' 右对齐, '^' 居中, '=' 数值符号感知(符号在左、其余补零)。
// zero 为 true 时以 '0' 填充并做符号感知；否则使用 fill 字符。
func padFieldA(s string, width int, align byte, fill byte, zero bool) string {
	if width <= len(s) {
		return s
	}
	pad := width - len(s)
	switch align {
	case '<':
		return s + strings.Repeat(string(fill), pad)
	case '^':
		left := pad / 2
		right := pad - left
		return strings.Repeat(string(fill), left) + s + strings.Repeat(string(fill), right)
	case '=':
		if len(s) > 0 && (s[0] == '-' || s[0] == '+' || s[0] == ' ') {
			if zero {
				return s[:1] + strings.Repeat("0", pad) + s[1:]
			}
			return s[:1] + strings.Repeat(string(fill), pad) + s[1:]
		}
		if zero {
			return strings.Repeat("0", pad) + s
		}
		return strings.Repeat(string(fill), pad) + s
	default: // '>'
		if len(s) > 0 && (s[0] == '-' || s[0] == '+' || s[0] == ' ') {
			if zero {
				return s[:1] + strings.Repeat("0", pad) + s[1:]
			}
			return s[:1] + strings.Repeat(string(fill), pad) + s[1:]
		}
		if zero {
			return strings.Repeat("0", pad) + s
		}
		return strings.Repeat(string(fill), pad) + s
	}
}

// fmtInt 按给定进制/类型格式化整数
func fmtInt(iv int, typ byte, sign string, alt bool, align byte, fill byte, zero bool, width int, hasPrec bool, prec int) string {
	neg := iv < 0
	mag := iv
	if neg {
		mag = -mag
	}
	sg := ""
	if neg {
		sg = "-"
	} else if sign == "+" {
		sg = "+"
	} else if sign == " " {
		sg = " "
	}
	digits := ""
	switch typ {
	case 'd', 'i', 'u':
		digits = strconv.Itoa(mag)
	case 'b':
		digits = strconv.FormatInt(int64(mag), 2)
	case 'B':
		digits = strings.ToUpper(strconv.FormatInt(int64(mag), 2))
	case 'o':
		digits = strconv.FormatInt(int64(mag), 8)
	case 'x':
		digits = strconv.FormatInt(int64(mag), 16)
	case 'X':
		digits = strings.ToUpper(strconv.FormatInt(int64(mag), 16))
	}
	if hasPrec {
		for len(digits) < prec {
			digits = "0" + digits
		}
	}
	if alt && mag != 0 {
		switch typ {
		case 'o':
			digits = "0o" + digits
		case 'x':
			digits = "0x" + digits
		case 'X':
			digits = "0X" + digits
		case 'b':
			digits = "0b" + digits
		case 'B':
			digits = "0B" + digits
		}
	}
	return padFieldA(sg+digits, width, align, fill, zero)
}

// fmtFloat 按给定类型格式化浮点数
func fmtFloat(fv float64, typ byte, sign string, alt bool, align byte, fill byte, zero bool, width int, hasPrec bool, prec int) string {
	p := 6
	if hasPrec {
		p = prec
	}
	var b byte
	switch typ {
	case 'f', 'F':
		b = 'f'
	case 'e', 'E':
		b = 'e'
	case 'g', 'G':
		b = 'g'
	}
	s := strconv.FormatFloat(fv, b, p, 64)
	if typ == 'F' || typ == 'E' || typ == 'G' {
		s = strings.ToUpper(s)
	}
	if fv >= 0 {
		if sign == "+" {
			s = "+" + s
		} else if sign == " " {
			s = " " + s
		}
	}
	return padFieldA(s, width, align, fill, zero)
}

func isAlign(b byte) bool {
	return b == '<' || b == '>' || b == '^' || b == '='
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

// percentFormat 实现 str % args 的 printf 风格格式化
func percentFormat(format string, arg Object) (Object, error) {
	var args []Object
	var mapping *Dict
	switch a := arg.(type) {
	case *Tuple:
		args = a.Items
	case *List:
		args = a.Items
	case *Dict:
		mapping = a
	default:
		args = []Object{arg}
	}
	var sb strings.Builder
	i, n := 0, len(format)
	idx := 0
	for i < n {
		c := format[i]
		if c != '%' {
			sb.WriteByte(c)
			i++
			continue
		}
		i++
		if i >= n {
			return nil, newExc("ValueError", "格式字符串中 %% 后缺少转换字符")
		}
		key := ""
		if format[i] == '(' {
			j := i + 1
			for j < n && format[j] != ')' {
				j++
			}
			if j >= n {
				return nil, newExc("ValueError", "格式字符串中未闭合的 %%(")
			}
			key = format[i+1 : j]
			i = j + 1
		}
		flags := ""
		for i < n && strings.IndexByte("#0- +", format[i]) >= 0 {
			flags += string(format[i])
			i++
		}
		wstr := ""
		for i < n && isDigitByte(format[i]) {
			wstr += string(format[i])
			i++
		}
		width := 0
		if wstr != "" {
			width, _ = strconv.Atoi(wstr)
		}
		hasPrec := false
		prec := 0
		if i < n && format[i] == '.' {
			i++
			pstr := ""
			for i < n && isDigitByte(format[i]) {
				pstr += string(format[i])
				i++
			}
			if pstr != "" {
				prec, _ = strconv.Atoi(pstr)
				hasPrec = true
			}
		}
		if i < n && (format[i] == 'l' || format[i] == 'L' || format[i] == 'h') {
			i++
		}
		conv := format[i]
		i++
		if conv == '%' {
			sb.WriteByte('%')
			continue
		}
		var val Object
		if key != "" {
			if mapping == nil {
				return nil, newExc("TypeError", "%%(name) 格式需要一个字典参数")
			}
			v, ok := mapping.Get(key)
			if !ok {
				return nil, newExc("KeyError", key)
			}
			val = v
		} else {
			if idx >= len(args) {
				return nil, newExc("TypeError", "格式字符串的参数不足")
			}
			val = args[idx]
			idx++
		}
		s, err := percentConv(val, conv, flags, width, hasPrec, prec)
		if err != nil {
			return nil, err
		}
		sb.WriteString(s)
	}
	return sb.String(), nil
}

func percentConv(val Object, conv byte, flags string, width int, hasPrec bool, prec int) (string, error) {
	align := byte('>')
	zero := false
	if strings.Contains(flags, "-") {
		align = '<'
	} else if strings.Contains(flags, "0") {
		zero = true
	}
	sign := ""
	if strings.Contains(flags, "+") {
		sign = "+"
	} else if strings.Contains(flags, " ") {
		sign = " "
	}
	alt := strings.Contains(flags, "#")
	switch conv {
	case 's', 'r':
		var s string
		if conv == 's' {
			s = Str(val)
		} else {
			s = Repr(val)
		}
		if hasPrec {
			runes := []rune(s)
			if prec < len(runes) {
				s = string(runes[:prec])
			}
		}
		return padFieldA(s, width, align, ' ', false), nil
	case 'c':
		if iv, ok := intVal(val); ok {
			return padFieldA(string(rune(iv)), width, align, ' ', false), nil
		}
		if s, ok := val.(string); ok && len([]rune(s)) == 1 {
			return padFieldA(s, width, align, ' ', false), nil
		}
		return "", newExc("TypeError", "%%c 需要整数或单字符字符串")
	case 'd', 'i', 'u', 'o', 'x', 'X', 'b', 'B':
		iv, ok := intVal(val)
		if !ok {
			if f, ok2 := numVal(val); ok2 {
				iv = int(f)
			} else {
				return "", newExc("TypeError", "%%%c 需要整数", conv)
			}
		}
		return fmtInt(iv, conv, sign, alt, align, ' ', zero, width, hasPrec, prec), nil
	case 'e', 'E', 'f', 'F', 'g', 'G':
		fv, ok := numVal(val)
		if !ok {
			return "", newExc("TypeError", "%%%c 需要数值", conv)
		}
		return fmtFloat(fv, conv, sign, alt, align, ' ', zero, width, hasPrec, prec), nil
	default:
		return "", newExc("ValueError", "不支持的格式字符 '%%%c'", conv)
	}
}

func intPow(a, b int) int {
	r := 1
	for i := 0; i < b; i++ {
		r *= a
	}
	return r
}

// ============ str.format 风格格式化 ============

// formatString 实现 str.format(*args, **kwargs)
func formatString(format string, args []Object, kwargs map[string]Object) (string, error) {
	var sb strings.Builder
	i, n := 0, len(format)
	auto := 0
	for i < n {
		c := format[i]
		if c == '{' {
			if i+1 < n && format[i+1] == '{' {
				sb.WriteByte('{')
				i += 2
				continue
			}
			j := i + 1
			for j < n && format[j] != '}' {
				j++
			}
			if j >= n {
				return "", newExc("ValueError", "未闭合的 {} 占位符")
			}
			spec := format[i+1 : j]
			i = j + 1
			text, err := renderField(spec, args, kwargs, &auto)
			if err != nil {
				return "", err
			}
			sb.WriteString(text)
		} else if c == '}' {
			if i+1 < n && format[i+1] == '}' {
				sb.WriteByte('}')
				i += 2
				continue
			}
			return "", newExc("ValueError", "单独的 } 占位符")
		} else {
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String(), nil
}

func renderField(spec string, args []Object, kwargs map[string]Object, auto *int) (string, error) {
	field := spec
	conv := byte(0)
	fspec := ""
	if idx := strings.IndexByte(spec, '!'); idx >= 0 {
		field = spec[:idx]
		rest := spec[idx+1:]
		if len(rest) > 0 {
			conv = rest[0]
		}
		if c2 := strings.IndexByte(rest, ':'); c2 >= 0 {
			fspec = rest[c2+1:]
		}
	} else if idx := strings.IndexByte(spec, ':'); idx >= 0 {
		field = spec[:idx]
		fspec = spec[idx+1:]
	}
	var val Object
	if field == "" {
		if *auto >= len(args) {
			return "", newExc("IndexError", "format 自动编号参数越界")
		}
		val = args[*auto]
		*auto++
	} else if isAllDigits(field) {
		idx, _ := strconv.Atoi(field)
		if idx >= len(args) {
			return "", newExc("IndexError", "format 位置参数 %d 越界", idx)
		}
		val = args[idx]
	} else {
		if v, ok := kwargs[field]; ok {
			val = v
		} else {
			return "", newExc("KeyError", field)
		}
	}
	if conv == 'r' {
		return formatValue(Repr(val), fspec)
	}
	if conv == 's' {
		return formatValue(Str(val), fspec)
	}
	return formatValue(val, fspec)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigitByte(s[i]) {
			return false
		}
	}
	return true
}

func formatValue(val Object, fspec string) (string, error) {
	fill := byte(' ')
	align := byte(0)
	pos := 0
	if pos < len(fspec) && isAlign(fspec[pos]) {
		align = fspec[pos]
		pos++
	} else if pos+1 < len(fspec) && isAlign(fspec[pos+1]) {
		fill = fspec[pos]
		align = fspec[pos+1]
		pos += 2
	}
	sign := ""
	if pos < len(fspec) && (fspec[pos] == '+' || fspec[pos] == '-' || fspec[pos] == ' ') {
		sign = string(fspec[pos])
		pos++
	}
	alt := false
	if pos < len(fspec) && fspec[pos] == '#' {
		alt = true
		pos++
	}
	zero := false
	if pos < len(fspec) && fspec[pos] == '0' {
		zero = true
		pos++
	}
	wstr := ""
	for pos < len(fspec) && isDigitByte(fspec[pos]) {
		wstr += string(fspec[pos])
		pos++
	}
	width := 0
	if wstr != "" {
		width, _ = strconv.Atoi(wstr)
	}
	hasPrec := false
	prec := 0
	if pos < len(fspec) && fspec[pos] == '.' {
		pos++
		pstr := ""
		for pos < len(fspec) && isDigitByte(fspec[pos]) {
			pstr += string(fspec[pos])
			pos++
		}
		if pstr != "" {
			prec, _ = strconv.Atoi(pstr)
			hasPrec = true
		}
	}
	typ := byte(0)
	if pos < len(fspec) {
		typ = fspec[pos]
		pos++
	}
	a := align
	if a == 0 {
		if zero {
			a = '='
		} else {
			a = '>'
		}
	}
	switch typ {
	case 0, 's', 'r':
		s := Str(val)
		if typ == 'r' {
			s = Repr(val)
		}
		if hasPrec {
			runes := []rune(s)
			if prec < len(runes) {
				s = string(runes[:prec])
			}
		}
		return padFieldA(s, width, a, fill, false), nil
	case 'd', 'i', 'u', 'o', 'x', 'X', 'b', 'B':
		iv, ok := intVal(val)
		if !ok {
			if f, ok2 := numVal(val); ok2 {
				iv = int(f)
			} else {
				return "", newExc("TypeError", "format 需要整数用于 %%%c", typ)
			}
		}
		return fmtInt(iv, typ, sign, alt, a, fill, zero, width, hasPrec, prec), nil
	case 'e', 'E', 'f', 'F', 'g', 'G':
		fv, ok := numVal(val)
		if !ok {
			return "", newExc("TypeError", "format 需要数值用于 %%%c", typ)
		}
		return fmtFloat(fv, typ, sign, alt, a, fill, zero, width, hasPrec, prec), nil
	case 'c':
		if iv, ok := intVal(val); ok {
			return padFieldA(string(rune(iv)), width, a, fill, false), nil
		}
		if s, ok := val.(string); ok && len([]rune(s)) == 1 {
			return padFieldA(s, width, a, fill, false), nil
		}
		return "", newExc("TypeError", "format %%c 需要整数或单字符")
	case '%':
		return padPercent(fvOf(val), sign, a, fill, width), nil
	}
	return "", newExc("ValueError", "未知的格式类型 '%c'", typ)
}

func fvOf(val Object) float64 {
	if f, ok := numVal(val); ok {
		return f
	}
	return 0
}

func padPercent(fv float64, sign string, a byte, fill byte, width int) string {
	s := strconv.FormatFloat(fv*100, 'f', 6, 64)
	if fv >= 0 {
		if sign == "+" {
			s = "+" + s
		} else if sign == " " {
			s = " " + s
		}
	}
	return padFieldA(s+"%", width, a, fill, false)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func unaryOp(op string, v Object) (Object, error) {
	switch op {
	case "-":
		if i, ok := intVal(v); ok {
			return -i, nil
		}
		if f, ok := numVal(v); ok {
			return -f, nil
		}
		return nil, newExc("TypeError", "一元 - 不支持 '%s'", typeName(v))
	case "+":
		if i, ok := intVal(v); ok {
			return i, nil
		}
		if f, ok := numVal(v); ok {
			return f, nil
		}
		return nil, newExc("TypeError", "一元 + 不支持 '%s'", typeName(v))
	case "~":
		i, ok := intVal(v)
		if !ok {
			return nil, newExc("TypeError", "一元 ~ 不支持 '%s'", typeName(v))
		}
		return ^i, nil
	case "not":
		return !truthy(v), nil
	}
	return nil, newExc("SyntaxError", "未知一元运算符 %s", op)
}

// ============ 迭代与切片 ============

// iterate 把一个可迭代对象展开为值列表
func iterate(v Object) ([]Object, error) {
	switch x := v.(type) {
	case *List:
		return x.Items, nil
	case *Tuple:
		return x.Items, nil
	case *Set:
		return x.Order, nil
	case *Dict:
		out := make([]Object, 0, x.Len())
		out = append(out, x.Keys...)
		return out, nil
	case *Range:
		n := x.Len()
		out := make([]Object, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, x.At(i))
		}
		return out, nil
	case string:
		out := make([]Object, 0, len(x))
		for _, r := range x {
			out = append(out, string(r))
		}
		return out, nil
	case *Generator:
		// 生成器惰性拉取至耗尽。通过钩子变量调用解释器，避免包级初始化循环
		if generatorIterateHook == nil {
			return nil, newExc("RuntimeError", "解释器尚未初始化")
		}
		return generatorIterateHook(x)
	case *PyCounter:
		out := make([]Object, 0, x.D.Len())
		out = append(out, x.D.Keys...)
		return out, nil
	case *PyDefaultDict:
		out := make([]Object, 0, x.D.Len())
		out = append(out, x.D.Keys...)
		return out, nil
	case *PyOrderedDict:
		out := make([]Object, 0, x.D.Len())
		out = append(out, x.D.Keys...)
		return out, nil
	case *PyDeque:
		return x.Items, nil
	}
	return nil, newExc("TypeError", "'%s' 对象不可迭代", typeName(v))
}

// contains 实现 in / not in
func contains(needle, hay Object) (bool, error) {
	// 实例的 __contains__ 优先
	if inst, ok := hay.(*Instance); ok {
		if fn, has := inst.Class.LookupMethod("__contains__"); has {
			v, err := callObjectRef(&Method{Recv: inst, Fn: fn}, []Object{needle}, nil)
			if err != nil {
				return false, err
			}
			return truthy(v), nil
		}
	}
	switch x := hay.(type) {
	case *List, *Tuple, *Set, *Range:
		items, err := iterate(hay)
		if err != nil {
			return false, err
		}
		for _, it := range items {
			if objectsEqual(it, needle) {
				return true, nil
			}
		}
		return false, nil
	case *Dict:
		_, ok := x.Get(needle)
		return ok, nil
	case *PyCounter:
		_, ok := x.D.Get(needle)
		return ok, nil
	case *PyDefaultDict:
		_, ok := x.D.Get(needle)
		return ok, nil
	case *PyOrderedDict:
		_, ok := x.D.Get(needle)
		return ok, nil
	case *PyDeque:
		for _, it := range x.Items {
			if objectsEqual(it, needle) {
				return true, nil
			}
		}
		return false, nil
	case string:
		s, ok := needle.(string)
		if !ok {
			return false, newExc("TypeError", "in 需要字符串左操作数，实际为 '%s'", typeName(needle))
		}
		return strings.Contains(x, s), nil
	}
	return false, newExc("TypeError", "'%s' 不支持成员检测", typeName(hay))
}

// sliceIndices 按 Python 语义计算切片下标序列
func sliceIndices(n int, start, stop, step Object) ([]int, error) {
	st := 1
	hasStep := step != nil
	if hasStep {
		i, ok := intVal(step)
		if !ok {
			return nil, newExc("TypeError", "切片步长必须是整数")
		}
		st = i
	}
	if st == 0 {
		return nil, newExc("ValueError", "slice step cannot be zero")
	}
	var idx []int
	if st > 0 {
		s := 0
		if start != nil {
			v, ok := intVal(start)
			if !ok {
				return nil, newExc("TypeError", "切片下标必须是整数")
			}
			s = v
			if s < 0 {
				s += n
			}
			if s < 0 {
				s = 0
			}
			if s > n {
				s = n
			}
		}
		e := n
		if stop != nil {
			v, ok := intVal(stop)
			if !ok {
				return nil, newExc("TypeError", "切片下标必须是整数")
			}
			e = v
			if e < 0 {
				e += n
			}
			if e < 0 {
				e = 0
			}
			if e > n {
				e = n
			}
		}
		for k := s; k < e; k += st {
			idx = append(idx, k)
		}
		return idx, nil
	}
	s := n - 1
	if start != nil {
		v, ok := intVal(start)
		if !ok {
			return nil, newExc("TypeError", "切片下标必须是整数")
		}
		s = v
		if s < 0 {
			s += n
		}
		if s < -1 {
			s = -1
		}
		if s > n-1 {
			s = n - 1
		}
	}
	e := -1
	if stop != nil {
		v, ok := intVal(stop)
		if !ok {
			return nil, newExc("TypeError", "切片下标必须是整数")
		}
		e = v
		if e < 0 {
			e += n
		}
		if e < -1 {
			e = -1
		}
		if e > n-1 {
			e = n - 1
		}
	}
	for k := s; k > e; k += st {
		idx = append(idx, k)
	}
	return idx, nil
}

// ============ f-string 格式说明符 ============

func isAlignChar(c byte) bool {
	return c == '<' || c == '>' || c == '^' || c == '='
}

// applyFormatSpec 按 Python 的 format spec 迷你语言格式化值：
// [[fill]align][sign][#][0][width][.precision][type]
// 支持类型 d b o x X e E f F g G % c s，其余返回 NotImplementedError。
func applyFormatSpec(v Object, spec string) (string, error) {
	if spec == "" {
		return Str(v), nil
	}
	i := 0
	fill := byte(' ')
	align := byte(0)
	if i < len(spec) && isAlignChar(spec[i]) {
		align = spec[i]
		i++
	} else if i+1 < len(spec) && isAlignChar(spec[i+1]) {
		fill = spec[i]
		align = spec[i+1]
		i += 2
	}
	plus := false
	sharp := false
	zeroPad := false
	for i < len(spec) {
		c := spec[i]
		if c == '+' {
			plus = true
		} else if c == '#' {
			sharp = true
		} else if c == '0' && align == 0 {
			zeroPad = true
		} else {
			break
		}
		i++
	}
	width := -1
	for i < len(spec) && spec[i] >= '0' && spec[i] <= '9' {
		if width < 0 {
			width = 0
		}
		width = width*10 + int(spec[i]-'0')
		i++
	}
	prec := -1
	if i < len(spec) && spec[i] == '.' {
		i++
		prec = 0
		for i < len(spec) && spec[i] >= '0' && spec[i] <= '9' {
			prec = prec*10 + int(spec[i]-'0')
			i++
		}
	}
	if i < len(spec) && spec[i] == ',' {
		return "", newExc("NotImplementedError", "格式说明符中的千分位分隔符尚未支持")
	}
	typ := byte(0)
	if i < len(spec) {
		typ = spec[i]
		i++
	}
	if i != len(spec) {
		return "", newExc("NotImplementedError", "无法识别的格式说明符: %s", spec)
	}

	body := ""
	switch typ {
	case 0, 's':
		_, isStr := v.(string)
		if !isStr {
			if f, ok := numVal(v); ok {
				_, isInt := v.(int)
				if typ == 0 && prec >= 0 && !isInt {
					body = strconv.FormatFloat(f, 'g', prec, 64)
					break
				}
			}
		}
		body = Str(v)
	case 'd':
		n, ok := intVal(v)
		if !ok {
			return "", newExc("ValueError", "格式类型 'd' 需要整数")
		}
		body = strconv.Itoa(n)
	case 'b', 'o', 'x', 'X':
		n, ok := intVal(v)
		if !ok {
			return "", newExc("ValueError", "进制格式化需要整数")
		}
		base := 16
		switch typ {
		case 'b':
			base = 2
		case 'o':
			base = 8
		}
		body = strconv.FormatInt(int64(n), base)
		if typ == 'X' {
			body = strings.ToUpper(body)
		}
		if sharp {
			switch typ {
			case 'b':
				body = "0b" + body
			case 'o':
				body = "0o" + body
			case 'x':
				body = "0x" + body
			case 'X':
				body = "0X" + body
			}
		}
	case 'c':
		n, ok := intVal(v)
		if !ok {
			return "", newExc("ValueError", "格式类型 'c' 需要整数")
		}
		body = string(rune(n))
	case 'e', 'E', 'f', 'F', 'g', 'G', '%':
		f, ok := toFloat(v)
		if !ok {
			return "", newExc("ValueError", "格式类型 '%c' 需要数值", typ)
		}
		p := prec
		switch typ {
		case 'e', 'E':
			if p < 0 {
				p = 6
			}
			body = strconv.FormatFloat(f, 'e', p, 64)
			if typ == 'E' {
				body = strings.ToUpper(body)
			}
		case 'f', 'F':
			if p < 0 {
				p = 6
			}
			body = strconv.FormatFloat(f, 'f', p, 64)
		case 'g', 'G':
			if p < 0 {
				p = 6
			}
			body = strconv.FormatFloat(f, 'g', p, 64)
			if typ == 'G' {
				body = strings.ToUpper(body)
			}
		case '%':
			if p < 0 {
				p = 6
			}
			body = strconv.FormatFloat(f*100, 'f', p, 64) + "%"
		}
	default:
		return "", newExc("NotImplementedError", "不支持的格式类型 '%c'", typ)
	}
	if prec >= 0 && (typ == 's' || typ == 0) {
		if _, isStr := v.(string); isStr {
			rs := []rune(body)
			if prec < len(rs) {
				body = string(rs[:prec])
			}
		}
	}

	// 符号与 0 填充
	if plus || zeroPad {
		neg := strings.HasPrefix(body, "-")
		digits := body
		if neg {
			digits = body[1:]
		}
		sign := ""
		if neg {
			sign = "-"
		} else if plus {
			sign = "+"
		}
		if zeroPad && width > 0 {
			if pad := width - len(sign) - len(digits); pad > 0 {
				return sign + strings.Repeat("0", pad) + digits, nil
			}
		}
		if neg || plus {
			body = sign + digits
		}
	}
	n := len([]rune(body))
	if width <= n {
		return body, nil
	}
	fillStr := string([]byte{fill})
	switch align {
	case '<':
		return body + strings.Repeat(fillStr, width-n), nil
	case '^':
		left := (width - n) / 2
		return strings.Repeat(fillStr, left) + body + strings.Repeat(fillStr, width-n-left), nil
	default:
		return strings.Repeat(fillStr, width-n) + body, nil
	}
}

// ============ 排序 ============

// sortObjects 升序排序：全数值按数值比，全字符串按字符串比，否则按 repr
func sortObjects(items []Object, reverse bool) []Object {
	out := make([]Object, len(items))
	copy(out, items)
	less := func(a, b Object) bool {
		if c, ok := compareValues(a, b); ok {
			if reverse {
				return c > 0
			}
			return c < 0
		}
		if reverse {
			return Repr(a) > Repr(b)
		}
		return Repr(a) < Repr(b)
	}
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}
