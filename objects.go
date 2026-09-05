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

// BuiltinMethod 绑定到内建类型实例的方法
type BuiltinMethod struct {
	Recv Object
	Name string
}

// Class 类对象，Parent 支持单继承
type Class struct {
	Name    string
	Parent  *Class
	Methods map[string]*Function
	Attrs   map[string]Object
}

func (c *Class) LookupMethod(name string) (*Function, bool) {
	for cur := c; cur != nil; cur = cur.Parent {
		if fn, ok := cur.Methods[name]; ok {
			return fn, true
		}
	}
	return nil, false
}

func (c *Class) LookupAttr(name string) (Object, bool) {
	for cur := c; cur != nil; cur = cur.Parent {
		if v, ok := cur.Attrs[name]; ok {
			return v, true
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
	case *Builtin:
		return "builtin_function_or_method"
	case *Method:
		return "method"
	case *BuiltinMethod:
		return "method"
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
	}
	return true
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

// instanceStrHook 由 interpreter.go 在 init 中注入，用于调用实例的 __str__。
// 通过函数变量而非直接调用，避免 Repr 与求值器之间形成包级初始化循环。
var instanceStrHook func(inst *Instance) (string, bool)

// instanceRepr 供 Repr 使用：优先尝试 __str__，否则输出默认的对象表示
func instanceRepr(inst *Instance) string {
	if instanceStrHook != nil {
		if s, ok := instanceStrHook(inst); ok {
			return s
		}
	}
	return "<" + inst.Class.Name + " object at " + fakeAddr("obj") + ">"
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
	case *PyException:
		if x.Msg == "" {
			return x.ExcType + "()"
		}
		return x.ExcType + "(" + quoteString(x.Msg) + ")"
	}
	return fmt.Sprintf("%v", v)
}

// Str 生成 Python str()：字符串本身不加引号，异常只输出消息，容器沿用 repr
func Str(v Object) string {
	switch x := v.(type) {
	case string:
		return x
	case *PyException:
		return x.Msg
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

func binaryOp(op string, l, r Object) (Object, error) {
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

func intPow(a, b int) int {
	r := 1
	for i := 0; i < b; i++ {
		r *= a
	}
	return r
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
	}
	return nil, newExc("TypeError", "'%s' 对象不可迭代", typeName(v))
}

// contains 实现 in / not in
func contains(needle, hay Object) (bool, error) {
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
