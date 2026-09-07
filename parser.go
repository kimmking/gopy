package main

import (
	"strconv"
	"strings"
)

// ============ 递归下降语法分析器 ============
// 表达式优先级由低到高：
//   or < and < not < 比较 < | < ^ < & < 移位 < +- < */ // % < 一元 < ** < 后缀 < 原子
// 该层次结构保证了 not 低于比较运算，与 Python 语义一致。

type Parser struct {
	toks []Token
	pos  int
}

// Parse 把源码解析为 AST，语法错误以 *SyntaxError 返回
func Parse(src string) (prog *Program, err error) {
	toks, terr := Tokenize(src)
	if terr != nil {
		return nil, terr
	}
	p := &Parser{toks: toks}
	defer func() {
		if r := recover(); r != nil {
			if se, ok := r.(*SyntaxError); ok {
				prog, err = nil, se
				return
			}
			panic(r)
		}
	}()
	return p.parseProgram()
}

// ParseExprString 解析一段独立的表达式源码（供 f-string 插值使用）
func ParseExprString(src string, line int) (e Expr, err error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, &SyntaxError{Line: line, Msg: "空的表达式"}
	}
	toks, terr := Tokenize(src)
	if terr != nil {
		return nil, terr
	}
	p := &Parser{toks: toks}
	defer func() {
		if r := recover(); r != nil {
			if se, ok := r.(*SyntaxError); ok {
				se.Line = line
				e, err = nil, se
				return
			}
			panic(r)
		}
	}()
	e = p.parseOr()
	if !p.atEnd() {
		return nil, &SyntaxError{Line: line, Msg: "无法解析的表达式: " + src}
	}
	return e, nil
}

// ---------- 基础工具 ----------

func (p *Parser) peek() Token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return Token{Type: TkEOF, Text: ""}
}

func (p *Parser) peekAt(n int) Token {
	if p.pos+n < len(p.toks) {
		return p.toks[p.pos+n]
	}
	return Token{Type: TkEOF, Text: ""}
}

func (p *Parser) advance() Token {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *Parser) at(t TokenType) bool {
	return p.peek().Type == t
}

func (p *Parser) atEnd() bool {
	return p.at(TkEOF) || p.at(TkNewline)
}

func (p *Parser) atOp(s string) bool {
	t := p.peek()
	return t.Type == TkOp && t.Text == s
}

func (p *Parser) atKw(s string) bool {
	t := p.peek()
	return t.Type == TkName && t.Text == s
}

func (p *Parser) acceptOp(s string) bool {
	if p.atOp(s) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) acceptKw(s string) bool {
	if p.atKw(s) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expectOp(s string) {
	if !p.atOp(s) {
		p.fail("期望 '" + s + "'")
	}
	p.advance()
}

func (p *Parser) expectKw(s string) {
	if !p.atKw(s) {
		p.fail("期望关键字 '" + s + "'")
	}
	p.advance()
}

func (p *Parser) expect(t TokenType) {
	if !p.at(t) {
		p.fail("期望 " + t.String())
	}
	p.advance()
}

func (p *Parser) expectName() string {
	t := p.peek()
	if t.Type != TkName {
		p.fail("期望标识符")
	}
	p.advance()
	return t.Text
}

func (p *Parser) fail(msg string) {
	panic(&SyntaxError{Line: p.peek().Line, Msg: msg + "，实际遇到 " + p.peek().String()})
}

// endSimple 结束一条简单语句
func (p *Parser) endSimple() {
	if p.at(TkNewline) {
		p.advance()
		return
	}
	if p.at(TkEOF) {
		return
	}
	p.fail("语句结束处出现多余内容")
}

func (p *Parser) skipNewlines() {
	for p.at(TkNewline) || p.at(TkIndent) || p.at(TkDedent) {
		p.advance()
	}
}

// ---------- 顶层 ----------

func (p *Parser) parseProgram() (*Program, error) {
	prog := &Program{}
	p.skipNewlines()
	for !p.at(TkEOF) {
		if p.at(TkNewline) || p.at(TkIndent) || p.at(TkDedent) {
			p.advance()
			continue
		}
		prog.Body = append(prog.Body, p.parseStatement())
	}
	return prog, nil
}

func (p *Parser) parseStatement() Stmt {
	switch {
	case p.atKw("if"):
		return p.parseIf()
	case p.atKw("while"):
		return p.parseWhile()
	case p.atKw("for"):
		return p.parseFor()
	case p.atKw("def"):
		return p.parseFuncDef()
	case p.atKw("class"):
		return p.parseClassDef()
	case p.atKw("try"):
		return p.parseTry()
	case p.atKw("with"):
		return p.parseWith()
	case p.atKw("import"):
		return p.parseImport()
	case p.atKw("from"):
		return p.parseFromImport()
	case p.atKw("return"):
		p.advance()
		st := &ReturnStmt{}
		if !p.atEnd() {
			st.Value = p.parseExpr()
		}
		p.endSimple()
		return st
	case p.atKw("yield"):
		p.advance()
		st := &YieldStmt{}
		if !p.atEnd() {
			st.Value = p.parseExpr()
		}
		p.endSimple()
		return st
	case p.atKw("break"):
		p.advance()
		p.endSimple()
		return &BreakStmt{}
	case p.atKw("continue"):
		p.advance()
		p.endSimple()
		return &ContinueStmt{}
	case p.atKw("pass"):
		p.advance()
		p.endSimple()
		return &PassStmt{}
	case p.atKw("raise"):
		p.advance()
		st := &RaiseStmt{}
		if !p.atEnd() {
			st.Value = p.parseExpr()
		}
		p.endSimple()
		return st
	case p.atKw("assert"):
		p.advance()
		st := &AssertStmt{Cond: p.parseExpr()}
		if p.acceptOp(",") {
			st.Msg = p.parseExpr()
		}
		p.endSimple()
		return st
	case p.atKw("del"):
		p.advance()
		st := &DeleteStmt{}
		st.Targets = append(st.Targets, p.parseOr())
		for p.acceptOp(",") {
			st.Targets = append(st.Targets, p.parseOr())
		}
		p.endSimple()
		return st
	case p.atKw("global"):
		p.advance()
		st := &GlobalStmt{}
		st.Names = append(st.Names, p.expectName())
		for p.acceptOp(",") {
			st.Names = append(st.Names, p.expectName())
		}
		p.endSimple()
		return st
	case p.atKw("nonlocal"):
		p.advance()
		st := &NonlocalStmt{}
		st.Names = append(st.Names, p.expectName())
		for p.acceptOp(",") {
			st.Names = append(st.Names, p.expectName())
		}
		p.endSimple()
		return st
	case p.atOp("@"):
		return p.parseDecorated()
	}
	st := p.parseSimpleStmtRaw()
	p.endSimple()
	return st
}

// parseDecorated 解析 @decorator 装饰器（可堆叠）+ def/class
func (p *Parser) parseDecorated() Stmt {
	var decs []Expr
	for p.atOp("@") {
		p.advance()
		decs = append(decs, p.parsePostfix())
		p.endSimple()
	}
	var st Stmt
	switch {
	case p.atKw("def"):
		st = p.parseFuncDef()
	case p.atKw("class"):
		st = p.parseClassDef()
	default:
		p.fail("装饰器后应为 def 或 class")
	}
	return &DecoratedStmt{Decorators: decs, Target: st}
}

// parseBlock 解析一个缩进块（也支持 `if x: pass` 这样的单行块）
func (p *Parser) parseBlock() []Stmt {
	p.expectOp(":")
	if p.at(TkNewline) {
		p.advance()
		p.expect(TkIndent)
		var body []Stmt
		for !p.at(TkDedent) && !p.at(TkEOF) {
			if p.at(TkNewline) {
				p.advance()
				continue
			}
			body = append(body, p.parseStatement())
		}
		if p.at(TkDedent) {
			p.advance()
		}
		return body
	}
	// 单行块
	body := []Stmt{p.parseSimpleStmtRaw()}
	p.endSimple()
	return body
}

// ---------- 复合语句 ----------

func (p *Parser) parseIf() Stmt {
	p.expectKw("if")
	st := &IfStmt{Cond: p.parseExpr(), Body: p.parseBlock()}
	for p.atKw("elif") {
		p.advance()
		st.Elifs = append(st.Elifs, &ElifClause{Cond: p.parseExpr(), Body: p.parseBlock()})
	}
	if p.atKw("else") {
		p.advance()
		st.Else = p.parseBlock()
	}
	return st
}

func (p *Parser) parseWhile() Stmt {
	p.expectKw("while")
	st := &WhileStmt{Cond: p.parseExpr(), Body: p.parseBlock()}
	if p.atKw("else") {
		p.advance()
		st.Else = p.parseBlock()
	}
	return st
}

func (p *Parser) parseFor() Stmt {
	p.expectKw("for")
	st := &ForStmt{Targets: p.parseTargetList()}
	p.expectKw("in")
	st.Iter = p.parseExpr()
	st.Body = p.parseBlock()
	if p.atKw("else") {
		p.advance()
		st.Else = p.parseBlock()
	}
	return st
}

func (p *Parser) parseFuncDef() Stmt {
	p.expectKw("def")
	name := p.expectName()
	p.expectOp("(")
	params := p.parseParams()
	p.expectOp(")")
	return &FuncDef{Name: name, Params: params, Body: p.parseBlock()}
}

func (p *Parser) parseParams() []Param {
	var params []Param
	seenDefault := false
	seenStar2 := false
	for !p.atOp(")") && !p.atEnd() {
		star, star2 := false, false
		if p.acceptOp("**") {
			star2 = true
		} else if p.acceptOp("*") {
			star = true
		}
		name := p.expectName()
		var def Expr
		if !star && !star2 && p.acceptOp("=") {
			def = p.parseExpr()
			seenDefault = true
		} else if seenDefault && !star && !star2 {
			p.fail("非默认参数不能出现在默认参数之后")
		}
		if star2 {
			seenStar2 = true
		}
		params = append(params, Param{Name: name, Default: def, Star: star, Star2: star2})
		if p.acceptOp(",") {
			continue
		}
		break
	}
	_ = seenStar2
	return params
}

func (p *Parser) parseClassDef() Stmt {
	p.expectKw("class")
	st := &ClassDef{Name: p.expectName()}
	if p.acceptOp("(") {
		if !p.atOp(")") {
			st.Bases = append(st.Bases, p.parseConditional())
			for p.acceptOp(",") {
				st.Bases = append(st.Bases, p.parseConditional())
			}
		}
		p.expectOp(")")
	}
	st.Body = p.parseBlock()
	return st
}

func (p *Parser) parseTry() Stmt {
	p.expectKw("try")
	st := &TryStmt{Body: p.parseBlock()}
	for p.atKw("except") {
		p.advance()
		clause := &ExceptClause{}
		if !p.atOp(":") {
			clause.ExcType = p.parseExpr()
			if p.acceptKw("as") {
				clause.Name = p.expectName()
			}
		}
		clause.Body = p.parseBlock()
		st.Handlers = append(st.Handlers, clause)
	}
	if len(st.Handlers) == 0 {
		p.fail("try 语句缺少 except 或 finally")
	}
	if p.atKw("else") {
		p.advance()
		st.Else = p.parseBlock()
	}
	if p.atKw("finally") {
		p.advance()
		st.Finally = p.parseBlock()
	}
	return st
}

// parseWith 解析 with 语句：with expr [as name] (, expr [as name])* : block
func (p *Parser) parseWith() Stmt {
	p.expectKw("with")
	st := &WithStmt{}
	for {
		item := &WithItem{CtxExpr: p.parseConditional()}
		if p.acceptKw("as") {
			item.Alias = p.expectName()
		}
		st.Items = append(st.Items, item)
		if p.acceptOp(",") {
			continue
		}
		break
	}
	st.Body = p.parseBlock()
	return st
}

func (p *Parser) parseImport() Stmt {
	p.expectKw("import")
	st := &ImportStmt{}
	for {
		path := []string{p.expectName()}
		for p.atOp(".") {
			p.advance()
			path = append(path, p.expectName())
		}
		alias := ""
		if p.acceptKw("as") {
			alias = p.expectName()
		}
		st.Names = append(st.Names, ImportAlias{Path: path, Alias: alias})
		if p.acceptOp(",") {
			continue
		}
		break
	}
	p.endSimple()
	return st
}

func (p *Parser) parseFromImport() Stmt {
	p.expectKw("from")
	st := &FromImportStmt{}
	dots := 0
	for p.atOp(".") {
		p.advance()
		dots++
	}
	if dots == 0 {
		st.Module = append(st.Module, p.expectName())
		for p.atOp(".") {
			p.advance()
			st.Module = append(st.Module, p.expectName())
		}
	}
	p.expectKw("import")
	if p.acceptOp("(") {
		// 括号形式：from x import (a, b as c)
		for !p.atOp(")") && !p.atEnd() {
			name := p.expectName()
			alias := ""
			if p.acceptKw("as") {
				alias = p.expectName()
			}
			st.Names = append(st.Names, ImportAlias{Path: []string{name}, Alias: alias})
			if p.acceptOp(",") {
				continue
			}
			break
		}
		p.expectOp(")")
	} else if p.acceptOp("*") {
		st.Names = append(st.Names, ImportAlias{Path: []string{"*"}})
	} else {
		for {
			name := p.expectName()
			alias := ""
			if p.acceptKw("as") {
				alias = p.expectName()
			}
			st.Names = append(st.Names, ImportAlias{Path: []string{name}, Alias: alias})
			if p.acceptOp(",") {
				continue
			}
			break
		}
	}
	p.endSimple()
	return st
}

// ---------- 简单语句 ----------

func (p *Parser) parseSimpleStmtRaw() Stmt {
	first := p.parseExpr()
	// 增量赋值：+= -= *= /= //= %= **= &= |= ^=
	switch p.peek().Text {
	case "+=", "-=", "*=", "/=", "//=", "%=", "**=", "&=", "|=", "^=", "<<=", ">>=":
		if p.peek().Type != TkOp {
			break
		}
		op := p.advance().Text
		return &AugAssign{Target: first, Op: op, Value: p.parseExpr()}
	}
	if p.atOp("=") {
		targets := unwrapTargets(first)
		for p.atOp("=") {
			p.advance()
			value := p.parseExpr()
			if p.atOp("=") {
				targets = append(targets, value)
				continue
			}
			return &Assign{Targets: targets, Value: value}
		}
	}
	return &ExprStmt{Value: first}
}

func unwrapTargets(e Expr) []Expr {
	if t, ok := e.(*TupleLit); ok {
		return t.Items
	}
	return []Expr{e}
}

// parseTarget 解析一个赋值/迭代目标。刻意不调用比较层，
// 否则 for 语句里的 `in` 会被当成成员运算符提前消费。
func (p *Parser) parseTarget() Expr {
	return p.parsePostfix()
}

func (p *Parser) parseTargetList() []Expr {
	items := []Expr{p.parseTarget()}
	for p.acceptOp(",") {
		items = append(items, p.parseTarget())
	}
	return items
}

// ---------- 表达式 ----------

// parseConditional 解析条件表达式（优先级仅高于 lambda，低于 or）
func (p *Parser) parseConditional() Expr {
	e := p.parseOr()
	if !p.atKw("if") {
		return e
	}
	p.advance()
	cond := p.parseOr()
	p.expectKw("else")
	return &CondExpr{Body: e, Cond: cond, Else: p.parseConditional()}
}

// parseExpr 解析表达式，支持无括号元组 `a, b`
func (p *Parser) parseExpr() Expr {
	first := p.parseConditional()
	if !p.atOp(",") {
		return first
	}
	items := []Expr{first}
	for p.atOp(",") {
		p.advance()
		if p.atOp(")") || p.atOp("]") || p.atOp("}") || p.atEnd() {
			break
		}
		items = append(items, p.parseConditional())
	}
	return &TupleLit{Items: items}
}

func (p *Parser) parseOr() Expr {
	left := p.parseAnd()
	if !p.atKw("or") {
		return left
	}
	vals := []Expr{left}
	for p.atKw("or") {
		p.advance()
		vals = append(vals, p.parseAnd())
	}
	return &BoolOp{Op: "or", Values: vals}
}

func (p *Parser) parseAnd() Expr {
	left := p.parseNot()
	if !p.atKw("and") {
		return left
	}
	vals := []Expr{left}
	for p.atKw("and") {
		p.advance()
		vals = append(vals, p.parseNot())
	}
	return &BoolOp{Op: "and", Values: vals}
}

func (p *Parser) parseNot() Expr {
	if p.atKw("not") {
		p.advance()
		return &UnaryOp{Op: "not", Operand: p.parseNot()}
	}
	return p.parseComparison()
}

func (p *Parser) comparisonOp() (string, bool) {
	if p.atKw("in") {
		p.advance()
		return "in", true
	}
	if p.atKw("not") && p.peekAt(1).Type == TkName && p.peekAt(1).Text == "in" {
		p.advance()
		p.advance()
		return "not in", true
	}
	if p.atKw("is") {
		p.advance()
		if p.acceptKw("not") {
			return "is not", true
		}
		return "is", true
	}
	t := p.peek()
	if t.Type == TkOp {
		switch t.Text {
		case "<", ">", "==", "!=", "<=", ">=":
			p.advance()
			return t.Text, true
		}
	}
	return "", false
}

func (p *Parser) parseComparison() Expr {
	left := p.parseBitOr()
	var ops []string
	var comps []Expr
	for {
		op, ok := p.comparisonOp()
		if !ok {
			break
		}
		ops = append(ops, op)
		comps = append(comps, p.parseBitOr())
	}
	if len(ops) == 0 {
		return left
	}
	return &Compare{Left: left, Ops: ops, Comps: comps}
}

func (p *Parser) parseBitOr() Expr {
	left := p.parseBitXor()
	for p.atOp("|") {
		p.advance()
		left = &BinOp{Op: "|", Left: left, Right: p.parseBitXor()}
	}
	return left
}

func (p *Parser) parseBitXor() Expr {
	left := p.parseBitAnd()
	for p.atOp("^") {
		p.advance()
		left = &BinOp{Op: "^", Left: left, Right: p.parseBitAnd()}
	}
	return left
}

func (p *Parser) parseBitAnd() Expr {
	left := p.parseShift()
	for p.atOp("&") {
		p.advance()
		left = &BinOp{Op: "&", Left: left, Right: p.parseShift()}
	}
	return left
}

func (p *Parser) parseShift() Expr {
	left := p.parseAddSub()
	for p.atOp("<<") || p.atOp(">>") {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseAddSub()}
	}
	return left
}

func (p *Parser) parseAddSub() Expr {
	left := p.parseTerm()
	for p.atOp("+") || p.atOp("-") {
		op := p.advance().Text
		left = &BinOp{Op: op, Left: left, Right: p.parseTerm()}
	}
	return left
}

func (p *Parser) parseTerm() Expr {
	left := p.parseUnary()
	for {
		t := p.peek()
		if t.Type != TkOp {
			break
		}
		switch t.Text {
		case "*", "/", "//", "%":
			p.advance()
			left = &BinOp{Op: t.Text, Left: left, Right: p.parseUnary()}
		default:
			return left
		}
	}
	return left
}

func (p *Parser) parseUnary() Expr {
	t := p.peek()
	if t.Type == TkOp && (t.Text == "-" || t.Text == "+" || t.Text == "~") {
		p.advance()
		return &UnaryOp{Op: t.Text, Operand: p.parseUnary()}
	}
	return p.parsePower()
}

func (p *Parser) parsePower() Expr {
	base := p.parsePostfix()
	if p.atOp("**") {
		p.advance()
		// 右结合，且右侧允许一元运算（2 ** -1）
		return &BinOp{Op: "**", Left: base, Right: p.parseUnary()}
	}
	return base
}

func (p *Parser) parsePostfix() Expr {
	e := p.parseAtom()
	for {
		switch {
		case p.atOp("("):
			p.advance()
			args := p.parseCallArgs()
			p.expectOp(")")
			e = &Call{Func: e, Args: args}
		case p.atOp("["):
			p.advance()
			idx := p.parseSubscript()
			p.expectOp("]")
			e = &Subscript{Value: e, Index: idx}
		case p.atOp("."):
			p.advance()
			e = &Attribute{Value: e, Attr: p.expectName()}
		default:
			return e
		}
	}
}

func (p *Parser) parseCallArgs() []CallArg {
	var args []CallArg
	for !p.atOp(")") && !p.atEnd() {
		if p.atOp("*") || p.atOp("**") {
			star2 := p.atOp("**")
			p.advance()
			if star2 && p.atOp("*") {
				p.fail("调用参数中不能出现 ***")
			}
			args = append(args, CallArg{Star: !star2, Star2: star2, Value: p.parseConditional()})
		} else if p.at(TkName) && p.peekAt(1).Type == TkOp && p.peekAt(1).Text == "=" {
			name := p.advance().Text
			p.advance()
			args = append(args, CallArg{Name: name, Value: p.parseConditional()})
		} else if p.at(TkName) && p.peekAt(1).Type == TkOp && p.peekAt(1).Text == ":=" {
			p.fail("不支持海象运算符")
		} else {
			v := p.parseConditional()
			// 免括号生成器实参：sum(x for x in it)（只能是最后一个实参）
			if p.atKw("for") {
				clauses := p.parseCompClauses()
				args = append(args, CallArg{Value: &GenExpr{Elem: v, Clauses: clauses}})
				break
			}
			args = append(args, CallArg{Value: v})
		}
		if p.acceptOp(",") {
			continue
		}
		break
	}
	return args
}

func (p *Parser) parseSubscript() Expr {
	var start, stop, step Expr
	hasColon := false
	if !p.atOp(":") && !p.atOp("]") {
		start = p.parseConditional()
	}
	if p.atOp(":") {
		hasColon = true
		p.advance()
		if !p.atOp(":") && !p.atOp("]") {
			stop = p.parseConditional()
		}
		if p.atOp(":") {
			p.advance()
			if !p.atOp("]") {
				step = p.parseConditional()
			}
		}
	}
	if !hasColon {
		if start == nil {
			p.fail("空的下标")
		}
		return start
	}
	return &Slice{Start: start, Stop: stop, Step: step}
}

func (p *Parser) parseAtom() Expr {
	t := p.peek()
	switch t.Type {
	case TkInt:
		p.advance()
		v, err := strconv.Atoi(t.Text)
		if err != nil {
			p.fail("整数超出范围: " + t.Text)
		}
		return &IntLit{Value: v}
	case TkFloat:
		p.advance()
		v, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			p.fail("浮点数非法: " + t.Text)
		}
		return &FloatLit{Value: v}
	case TkString:
		p.advance()
		return &StrLit{Value: t.Text}
	case TkFString:
		p.advance()
		return p.parseFString(t.Text, t.Line)
	case TkName:
		switch t.Text {
		case "True":
			p.advance()
			return &BoolLit{Value: true}
		case "False":
			p.advance()
			return &BoolLit{Value: false}
		case "None":
			p.advance()
			return &NoneLit{}
		case "lambda":
			return p.parseLambda()
		case "and", "or", "not", "in", "is", "if", "else", "elif", "while", "for",
			"def", "class", "return", "break", "continue", "pass", "try", "except",
			"finally", "import", "from", "as", "raise", "assert", "del", "global",
			"with":
			p.fail("此处不应出现关键字 '" + t.Text + "'")
		}
		p.advance()
		return &Name{Id: t.Text}
	case TkOp:
		switch t.Text {
		case "[":
			return p.parseListOrComp()
		case "{":
			return p.parseDictSetOrComp()
		case "(":
			p.advance()
			if p.atOp(")") {
				p.advance()
				return &TupleLit{}
			}
			e := p.parseExpr()
			// 生成器表达式：(x * x for x in it if c)
			if p.atKw("for") {
				clauses := p.parseCompClauses()
				p.expectOp(")")
				return &GenExpr{Elem: e, Clauses: clauses}
			}
			p.expectOp(")")
			return e
		}
	}
	p.fail("无法识别的表达式起始记号")
	return nil
}

func (p *Parser) parseListOrComp() Expr {
	p.expectOp("[")
	if p.atOp("]") {
		p.advance()
		return &ListLit{}
	}
	first := p.parseConditional()
	if p.atKw("for") {
		clauses := p.parseCompClauses()
		p.expectOp("]")
		return &ListComp{Elem: first, Clauses: clauses}
	}
	items := []Expr{first}
	for p.atOp(",") {
		p.advance()
		if p.atOp("]") {
			break
		}
		items = append(items, p.parseConditional())
	}
	p.expectOp("]")
	return &ListLit{Items: items}
}

func (p *Parser) parseDictSetOrComp() Expr {
	p.expectOp("{")
	if p.atOp("}") {
		p.advance()
		return &DictLit{}
	}
	first := p.parseConditional()
	if p.atOp(":") {
		p.advance()
		val := p.parseConditional()
		if p.atKw("for") {
			clauses := p.parseCompClauses()
			p.expectOp("}")
			return &DictComp{Key: first, Value: val, Clauses: clauses}
		}
		keys := []Expr{first}
		vals := []Expr{val}
		for p.atOp(",") {
			p.advance()
			if p.atOp("}") {
				break
			}
			k := p.parseConditional()
			p.expectOp(":")
			v := p.parseConditional()
			keys = append(keys, k)
			vals = append(vals, v)
		}
		p.expectOp("}")
		return &DictLit{Keys: keys, Vals: vals}
	}
	// 集合字面量
	items := []Expr{first}
	for p.atOp(",") {
		p.advance()
		if p.atOp("}") {
			break
		}
		items = append(items, p.parseConditional())
	}
	p.expectOp("}")
	return &Call{Func: &Name{Id: "set"}, Args: []CallArg{{Value: &ListLit{Items: items}}}}
}

func (p *Parser) parseCompClauses() []CompClause {
	var clauses []CompClause
	for p.atKw("for") {
		p.advance()
		c := CompClause{Targets: p.parseTargetList()}
		p.expectKw("in")
		c.Iter = p.parseOr()
		for p.atKw("if") {
			p.advance()
			c.Ifs = append(c.Ifs, p.parseOr())
		}
		clauses = append(clauses, c)
	}
	if len(clauses) == 0 {
		p.fail("推导式缺少 for 子句")
	}
	return clauses
}

func (p *Parser) parseLambda() Expr {
	p.expectKw("lambda")
	st := &Lambda{}
	if !p.atOp(":") {
		for {
			name := p.expectName()
			var def Expr
			if p.acceptOp("=") {
				def = p.parseOr()
			}
			st.Params = append(st.Params, Param{Name: name, Default: def})
			if p.acceptOp(",") {
				continue
			}
			break
		}
	}
	p.expectOp(":")
	st.Body = p.parseConditional()
	return st
}

// ---------- f-string ----------

type fseg struct {
	text   string
	isExpr bool
}

func splitFString(raw string, line int) []fseg {
	var segs []fseg
	var lit strings.Builder
	i := 0
	for i < len(raw) {
		c := raw[i]
		if c == '{' {
			if i+1 < len(raw) && raw[i+1] == '{' {
				lit.WriteByte('{')
				i += 2
				continue
			}
			depth := 1
			j := i + 1
			inStr := false
			var q byte
			for j < len(raw) {
				ch := raw[j]
				if inStr {
					if ch == '\\' {
						j += 2
						continue
					}
					if ch == q {
						inStr = false
					}
					j++
					continue
				}
				switch ch {
				case '"', '\'':
					inStr = true
					q = ch
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						goto found
					}
				}
				j++
			}
			panic(&SyntaxError{Line: line, Msg: "f-string 中的 '{' 没有匹配的 '}'"})
		found:
			if lit.Len() > 0 {
				segs = append(segs, fseg{text: lit.String(), isExpr: false})
				lit.Reset()
			}
			segs = append(segs, fseg{text: raw[i+1 : j], isExpr: true})
			i = j + 1
			continue
		}
		if c == '}' {
			if i+1 < len(raw) && raw[i+1] == '}' {
				lit.WriteByte('}')
				i += 2
				continue
			}
			panic(&SyntaxError{Line: line, Msg: "f-string 中的 '}' 需要写成 '}}'"})
		}
		lit.WriteByte(c)
		i++
	}
	if lit.Len() > 0 {
		segs = append(segs, fseg{text: lit.String(), isExpr: false})
	}
	return segs
}

// topLevelColon 返回不在引号与括号内的第一个 ':' 下标，没有则返回 -1
func topLevelColon(s string) int {
	depth := 0
	inStr := false
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == q {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			q = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func (p *Parser) parseFString(raw string, line int) Expr {
	out := &FStrLit{}
	for _, seg := range splitFString(raw, line) {
		if !seg.isExpr {
			out.Parts = append(out.Parts, FStrPart{Text: unescapeFStrLiteral(seg.text)})
			continue
		}
		src := seg.text
		conv := ""
		spec := ""
		if ci := topLevelColon(src); ci >= 0 {
			spec = src[ci+1:]
			src = src[:ci]
		}
		if bi := strings.LastIndex(src, "!"); bi >= 0 {
			switch strings.TrimSpace(src[bi+1:]) {
			case "r":
				conv = "r"
			case "s":
				conv = "s"
			case "a":
				conv = "a"
			}
			src = src[:bi]
		}
		e, err := ParseExprString(src, line)
		if err != nil {
			panic(err)
		}
		out.Parts = append(out.Parts, FStrPart{Value: e, Conv: conv, Spec: spec})
	}
	return out
}

// unescapeFStrLiteral 处理 f-string 字面片段中的转义序列
func unescapeFStrLiteral(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			sb.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case '\\':
			sb.WriteByte('\\')
		case '"':
			sb.WriteByte('"')
		case '\'':
			sb.WriteByte('\'')
		case '{':
			sb.WriteByte('{')
		case '}':
			sb.WriteByte('}')
		default:
			sb.WriteByte('\\')
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}
