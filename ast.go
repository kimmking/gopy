package main

// ============ AST 节点定义 ============
// Stmt 与 Expr 分别代表语句与表达式节点，由 parser.go 产生、interpreter.go 求值。

type Stmt interface{ stmt() }

type Expr interface{ expr() }

// ---------- 语句 ----------

// Program 是整个脚本的根节点
type Program struct{ Body []Stmt }

// ExprStmt 单独的表达式语句，如 nums.append(1)
type ExprStmt struct{ Value Expr }

// Assign 支持链式赋值 a = b = 1 与元组解包 a, b = 1, 2
type Assign struct {
	Targets []Expr
	Value   Expr
}

// AugAssign 增量赋值，如 x += 1
type AugAssign struct {
	Target Expr
	Op     string // += -= *= /= //= %= **=
	Value  Expr
}

// ElifClause 是 if 语句中的一个 elif 分支
type ElifClause struct {
	Cond Expr
	Body []Stmt
}

// IfStmt 条件语句，Else 为 nil 表示没有 else 分支
type IfStmt struct {
	Cond  Expr
	Body  []Stmt
	Elifs []*ElifClause
	Else  []Stmt
}

// WhileStmt 循环语句，支持 while ... else
type WhileStmt struct {
	Cond Expr
	Body []Stmt
	Else []Stmt
}

// ForStmt 迭代语句，Targets 支持元组解包，支持 for ... else
type ForStmt struct {
	Targets []Expr
	Iter    Expr
	Body    []Stmt
	Else    []Stmt
}

// Param 是函数形参，Default 为 nil 表示无默认值；Star / Star2 标记 *args 与 **kwargs
type Param struct {
	Name    string
	Default Expr
	Star    bool
	Star2   bool
}

// FuncDef 函数定义
type FuncDef struct {
	Name   string
	Params []Param
	Body   []Stmt
}

// ReturnStmt 返回语句，Value 为 nil 表示裸 return
type ReturnStmt struct{ Value Expr }

type BreakStmt struct{}

type ContinueStmt struct{}

type PassStmt struct{}

// ClassDef 类定义，Bases 为父类表达式
type ClassDef struct {
	Name  string
	Bases []Expr
	Body  []Stmt
}

// ExceptClause 是 try 的一个 except 分支，ExcType 为 nil 表示裸 except
type ExceptClause struct {
	ExcType Expr
	Name    string
	Body    []Stmt
}

// TryStmt 异常处理，支持 except / else / finally
type TryStmt struct {
	Body     []Stmt
	Handlers []*ExceptClause
	Else     []Stmt
	Finally  []Stmt
}

// ImportAlias 表示 import a.b as c 中的一个名字
type ImportAlias struct {
	Path  []string
	Alias string
}

// ImportStmt 表示 import x, y as z
type ImportStmt struct{ Names []ImportAlias }

// FromImportStmt 表示 from x.y import a, b as c
type FromImportStmt struct {
	Module []string
	Names  []ImportAlias
}

// RaiseStmt 主动抛出异常
type RaiseStmt struct{ Value Expr }

// AssertStmt 断言语句
type AssertStmt struct {
	Cond Expr
	Msg  Expr
}

// DeleteStmt 删除变量或容器元素
type DeleteStmt struct{ Targets []Expr }

// GlobalStmt 声明全局变量
type GlobalStmt struct{ Names []string }

// NonlocalStmt 声明变量绑定到最近的外层函数作用域
type NonlocalStmt struct{ Names []string }

// DecoratedStmt 带装饰器的 def/class 定义
type DecoratedStmt struct {
	Decorators []Expr
	Target     Stmt
}

// decoratedName 返回被装饰目标的名字
func (d *DecoratedStmt) decoratedName() string {
	switch t := d.Target.(type) {
	case *FuncDef:
		return t.Name
	case *ClassDef:
		return t.Name
	}
	return ""
}

func (*Program) stmt()        {}
func (*ExprStmt) stmt()       {}
func (*Assign) stmt()         {}
func (*AugAssign) stmt()      {}
func (*IfStmt) stmt()         {}
func (*WhileStmt) stmt()      {}
func (*ForStmt) stmt()        {}
func (*FuncDef) stmt()        {}
func (*ReturnStmt) stmt()     {}
func (*BreakStmt) stmt()      {}
func (*ContinueStmt) stmt()   {}
func (*PassStmt) stmt()       {}
func (*ClassDef) stmt()       {}
func (*TryStmt) stmt()        {}
func (*ImportStmt) stmt()     {}
func (*FromImportStmt) stmt() {}
func (*RaiseStmt) stmt()      {}
func (*AssertStmt) stmt()     {}
func (*DeleteStmt) stmt()     {}
func (*GlobalStmt) stmt()     {}
func (*NonlocalStmt) stmt()   {}
func (*DecoratedStmt) stmt()  {}

// ---------- 表达式 ----------

// Name 变量引用
type Name struct{ Id string }

type IntLit struct{ Value int }

type FloatLit struct{ Value float64 }

type StrLit struct{ Value string }

// FStrPart 是 f-string 的一个片段：Value 为 nil 时 Text 是字面文本；
// Conv 保存转换符（"r" 表示 !r，"s" 表示 !s）
type FStrPart struct {
	Text  string
	Value Expr
	Conv  string
	Spec  string
}

// FStrLit f-string 字面量
type FStrLit struct{ Parts []FStrPart }

type BoolLit struct{ Value bool }

type NoneLit struct{}

type ListLit struct{ Items []Expr }

type TupleLit struct{ Items []Expr }

type DictLit struct {
	Keys []Expr
	Vals []Expr
}

// CompClause 是推导式中的一个 for 子句及其 if 过滤条件
type CompClause struct {
	Targets []Expr
	Iter    Expr
	Ifs     []Expr
}

type ListComp struct {
	Elem    Expr
	Clauses []CompClause
}

type DictComp struct {
	Key     Expr
	Value   Expr
	Clauses []CompClause
}

// BinOp 二元运算
type BinOp struct {
	Op    string
	Left  Expr
	Right Expr
}

// UnaryOp 一元运算，Op 取 "-" / "+" / "not" / "~"
type UnaryOp struct {
	Op      string
	Operand Expr
}

// BoolOp 短路逻辑运算，Values 长度 >= 2
type BoolOp struct {
	Op     string // and / or
	Values []Expr
}

// Compare 支持链式比较，如 1 < x < 10
type Compare struct {
	Left  Expr
	Ops   []string
	Comps []Expr
}

// CallArg 调用实参，Name 非空表示关键字参数；
// Star / Star2 表示调用处的 *iterable 与 **mapping 展开（Value 为名字表达式）
type CallArg struct {
	Name  string
	Value Expr
	Star  bool
	Star2 bool
}

type Call struct {
	Func Expr
	Args []CallArg
}

// Subscript 下标访问，Index 为 *Slice 时表示切片
type Subscript struct {
	Value Expr
	Index Expr
}

// Slice 切片下标，未给出的分量保持 nil
type Slice struct {
	Start Expr
	Stop  Expr
	Step  Expr
}

type Attribute struct {
	Value Expr
	Attr  string
}

// CondExpr 三元条件表达式：Body if Cond else Else
type CondExpr struct {
	Body Expr
	Cond Expr
	Else Expr
}

// Lambda 匿名函数
type Lambda struct {
	Params []Param
	Body   Expr
}

func (*Name) expr()      {}
func (*IntLit) expr()    {}
func (*FloatLit) expr()  {}
func (*StrLit) expr()    {}
func (*FStrLit) expr()   {}
func (*BoolLit) expr()   {}
func (*NoneLit) expr()   {}
func (*ListLit) expr()   {}
func (*TupleLit) expr()  {}
func (*DictLit) expr()   {}
func (*ListComp) expr()  {}
func (*DictComp) expr()  {}
func (*BinOp) expr()     {}
func (*UnaryOp) expr()   {}
func (*BoolOp) expr()    {}
func (*Compare) expr()   {}
func (*Call) expr()      {}
func (*Subscript) expr() {}
func (*Slice) expr()     {}
func (*Attribute) expr() {}
func (*CondExpr) expr()  {}
func (*Lambda) expr()    {}
