package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gopy <file.py>")
		os.Exit(1)
	}
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("read error: %v\n", err)
		os.Exit(1)
	}
	interp := NewInterpreter()
	interp.Run(string(src))
}

// ============ 数据结构 ============

type stmt struct {
	indent int
	text   string
}

// Func 表示用户自定义函数
type Func struct {
	params []string
	body   []stmt
}

// flowKind 表示语句执行后的控制流信号
type flowKind int

const (
	flowNormal flowKind = iota
	flowBreak
	flowContinue
	flowReturn
)

// Dict 表示保持插入顺序的字典（Python 3.7+ 字典有序）
type Dict struct {
	keys []string
	vals map[string]interface{}
}

func NewDict() *Dict {
	return &Dict{vals: make(map[string]interface{})}
}

func (d *Dict) Set(k string, v interface{}) {
	if _, ok := d.vals[k]; !ok {
		d.keys = append(d.keys, k)
	}
	d.vals[k] = v
}

func (d *Dict) Get(k string) (interface{}, bool) {
	v, ok := d.vals[k]
	return v, ok
}

func (d *Dict) Len() int {
	return len(d.keys)
}

type Interpreter struct {
	env      map[string]interface{}
	returned bool
	retVal   interface{}
}

func NewInterpreter() *Interpreter {
	return &Interpreter{env: make(map[string]interface{})}
}

func (i *Interpreter) Run(src string) {
	stmts := parseStmts(src)
	i.execStmts(stmts)
}

// parseStmts 把源码转成带缩进信息的语句列表（跳过空行与注释）
func parseStmts(src string) []stmt {
	var out []stmt
	for _, raw := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, stmt{indent: countIndent(raw), text: trimmed})
	}
	return out
}

func countIndent(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

// findBlockEnd 返回块结束位置（不含），即下一个缩进 <= 当前缩进的行
func findBlockEnd(stmts []stmt, pc int) int {
	base := stmts[pc].indent
	for j := pc + 1; j < len(stmts); j++ {
		if stmts[j].indent <= base {
			return j
		}
	}
	return len(stmts)
}

// ============ 语句执行 ============

func (i *Interpreter) execStmts(stmts []stmt) flowKind {
	pc := 0
	for pc < len(stmts) {
		s := stmts[pc]
		text := s.text

		// 函数定义
		if strings.HasPrefix(text, "def ") && strings.HasSuffix(text, ":") {
			bodyEnd := findBlockEnd(stmts, pc)
			body := stmts[pc+1 : bodyEnd]
			header := text[4 : len(text)-1]
			if lp := strings.Index(header, "("); lp > 0 {
				name := strings.TrimSpace(header[:lp])
				paramsStr := strings.TrimSpace(header[lp+1 : len(header)-1])
				var params []string
				if paramsStr != "" {
					for _, p := range strings.Split(paramsStr, ",") {
						params = append(params, strings.TrimSpace(p))
					}
				}
				i.env[name] = Func{params: params, body: body}
			}
			pc = bodyEnd
			continue
		}

		// for 循环
		if strings.HasPrefix(text, "for ") && strings.HasSuffix(text, ":") {
			bodyEnd := findBlockEnd(stmts, pc)
			body := stmts[pc+1 : bodyEnd]
			header := text[4 : len(text)-1]
			parts := strings.Split(header, " in ")
			if len(parts) == 2 {
				varName := strings.TrimSpace(parts[0])
				iterVal, err := i.evalExpr(strings.TrimSpace(parts[1]))
				if err == nil {
					switch list := iterVal.(type) {
					case []interface{}:
						for _, v := range list {
							i.env[varName] = v
							f := i.execStmts(body)
							if f == flowBreak {
								break
							}
							if f == flowReturn {
								return flowReturn
							}
						}
					case []string:
						for _, v := range list {
							i.env[varName] = v
							f := i.execStmts(body)
							if f == flowBreak {
								break
							}
							if f == flowReturn {
								return flowReturn
							}
						}
					}
				}
			}
			pc = bodyEnd
			continue
		}

		// while 循环
		if strings.HasPrefix(text, "while ") && strings.HasSuffix(text, ":") {
			bodyEnd := findBlockEnd(stmts, pc)
			body := stmts[pc+1 : bodyEnd]
			cond := text[6 : len(text)-1]
			for {
				v, err := i.evalExpr(cond)
				if err != nil || !toBool(v) {
					break
				}
				f := i.execStmts(body)
				if f == flowBreak {
					break
				}
				if f == flowReturn {
					return flowReturn
				}
			}
			pc = bodyEnd
			continue
		}

		// if / elif / else
		if strings.HasPrefix(text, "if ") && strings.HasSuffix(text, ":") {
			done := false
			cur := pc
			for cur < len(stmts) && stmts[cur].indent == s.indent {
				cs := stmts[cur]
				isBranch := false
				bodyEnd := findBlockEnd(stmts, cur)
				body := stmts[cur+1 : bodyEnd]
				take := false
				switch {
				case strings.HasPrefix(cs.text, "if ") && strings.HasSuffix(cs.text, ":"):
					isBranch = true
					v, err := i.evalExpr(cs.text[3 : len(cs.text)-1])
					take = err == nil && toBool(v)
					if take {
						done = true
					}
				case strings.HasPrefix(cs.text, "elif ") && strings.HasSuffix(cs.text, ":"):
					isBranch = true
					v, err := i.evalExpr(cs.text[5 : len(cs.text)-1])
					take = !done && err == nil && toBool(v)
					if take {
						done = true
					}
				case cs.text == "else:":
					isBranch = true
					take = !done
					done = true
				}
				if !isBranch {
					// 遇到非分支行：结束 if 链，pc 指向该行交由外层继续执行（不能 cur++，否则会跳过该行）
					break
				}
				if take {
					f := i.execStmts(body)
					if f != flowNormal {
						return f
					}
				}
				if bodyEnd <= cur {
					bodyEnd = cur + 1
				}
				cur = bodyEnd
			}
			pc = cur
			continue
		}

		// break / continue
		if text == "break" {
			return flowBreak
		}
		if text == "continue" {
			return flowContinue
		}

		// return
		if text == "return" || strings.HasPrefix(text, "return ") {
			expr := strings.TrimSpace(text[len("return"):])
			if expr == "" {
				i.retVal = nil
			} else if v, err := i.evalExpr(expr); err == nil {
				i.retVal = v
			} else {
				i.retVal = nil
			}
			return flowReturn
		}

		// print
		if strings.HasPrefix(text, "print(") && strings.HasSuffix(text, ")") {
			inner := strings.TrimSpace(text[len("print(") : len(text)-1])
			args := splitArgs(inner)
			var out []string
			for _, a := range args {
				v, err := i.evalExpr(a)
				if err != nil {
					out = append(out, a)
				} else {
					out = append(out, formatValue(v))
				}
			}
			fmt.Println(strings.Join(out, " "))
			pc++
			continue
		}

		// 赋值
		if idx := strings.Index(text, "="); idx > 0 {
			left := text[:idx]
			if !strings.ContainsAny(left, "!<>=") {
				name := strings.TrimSpace(left)
				if name != "" && !strings.ContainsAny(name, " ()[.") {
					expr := strings.TrimSpace(text[idx+1:])
					v, err := i.evalExpr(expr)
					if err == nil {
						i.env[name] = v
						pc++
						continue
					}
				}
			}
		}

		// 下标赋值：d["k"] = v 或 nums[i] = v
		if eq := strings.Index(text, "="); eq > 0 && !strings.ContainsAny(text[:eq], "!<>") {
			target := strings.TrimSpace(text[:eq])
			if ti := strings.Index(target, "["); ti > 0 && strings.HasSuffix(target, "]") {
				objName := strings.TrimSpace(target[:ti])
				keyStr := strings.TrimSpace(target[ti+1 : len(target)-1])
				if v, ok := i.env[objName]; ok {
					if vv, err := i.evalExpr(strings.TrimSpace(text[eq+1:])); err == nil {
						switch obj := v.(type) {
						case *Dict:
							if kv, err2 := i.evalExpr(keyStr); err2 == nil {
								obj.Set(fmt.Sprintf("%v", kv), vv)
								pc++
								continue
							}
						case []interface{}:
							if iv, err2 := i.evalExpr(keyStr); err2 == nil {
								n := toInt(iv)
								if n >= 0 && n < len(obj) {
									obj[n] = vv
									pc++
									continue
								}
							}
						}
					}
				}
			}
		}

		// 表达式语句：list.append(x) 需要写回
		if di := strings.Index(text, ".append("); di > 0 && strings.HasSuffix(text, ")") {
			objName := strings.TrimSpace(text[:di])
			if v, ok := i.env[objName]; ok {
				if list, ok := v.([]interface{}); ok {
					argStr := strings.TrimSpace(text[di+len(".append(") : len(text)-1])
					if av, err := i.evalExpr(argStr); err == nil {
						i.env[objName] = append(list, av)
						pc++
						continue
					}
				}
			}
		}

		// 其他表达式语句
		i.evalExpr(text)
		pc++
	}
	return flowNormal
}

// ============ 函数调用 ============

func (i *Interpreter) callFunc(fn Func, args []interface{}) interface{} {
	saved := i.env
	local := make(map[string]interface{})
	for k, v := range saved {
		local[k] = v
	}
	for idx, p := range fn.params {
		if idx < len(args) {
			local[p] = args[idx]
		} else {
			local[p] = nil
		}
	}
	i.env = local
	i.retVal = nil
	f := i.execStmts(fn.body)
	res := i.retVal
	i.env = saved
	i.retVal = nil
	if f != flowReturn {
		res = nil
	}
	return res
}

// ============ 表达式求值 ============

func (i *Interpreter) evalExpr(expr string) (interface{}, error) {
	expr = strings.TrimSpace(expr)
	// 普通字符串字面量
	if len(expr) >= 2 && ((expr[0] == '"' && expr[len(expr)-1] == '"') || (expr[0] == '\'' && expr[len(expr)-1] == '\'')) {
		return expr[1 : len(expr)-1], nil
	}
	// f-string
	if strings.HasPrefix(expr, "f\"") && strings.HasSuffix(expr, "\"") {
		return evalFString(expr, i.env)
	}
	if strings.HasPrefix(expr, "f'") && strings.HasSuffix(expr, "'") {
		return evalFString(expr, i.env)
	}
	// 布尔字面量
	if expr == "True" {
		return true, nil
	}
	if expr == "False" {
		return false, nil
	}
	// str(...) 内联
	if strings.Contains(expr, "str(") {
		expr = strings.TrimSpace(inlineStrCalls(expr, i))
	}
	// 列表字面量
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		inner := expr[1 : len(expr)-1]
		var items []interface{}
		if strings.TrimSpace(inner) != "" {
			for _, part := range splitArgs(inner) {
				v, err := i.evalExpr(part)
				if err != nil {
					v = part
				}
				items = append(items, v)
			}
		}
		return items, nil
	}
	// 字典字面量 {"a": 1, "b": 2}
	if strings.HasPrefix(expr, "{") && strings.HasSuffix(expr, "}") {
		inner := expr[1 : len(expr)-1]
		d := NewDict()
		if strings.TrimSpace(inner) != "" {
			for _, part := range splitArgs(inner) {
				if ci := strings.Index(part, ":"); ci > 0 {
					k, err := i.evalExpr(strings.TrimSpace(part[:ci]))
					v, err2 := i.evalExpr(strings.TrimSpace(part[ci+1:]))
					if err == nil && err2 == nil {
						d.Set(fmt.Sprintf("%v", k), v)
					}
				}
			}
		}
		return d, nil
	}
	// 函数调用 / 方法调用
	if strings.Contains(expr, "(") && strings.Contains(expr, ")") {
		if idx := strings.Index(expr, "("); idx > 0 {
			callee := strings.TrimSpace(expr[:idx])
			argsStr := strings.TrimSpace(expr[idx+1 : len(expr)-1])
			if dotIdx := strings.LastIndex(callee, "."); dotIdx > 0 {
				objName := strings.TrimSpace(callee[:dotIdx])
				method := strings.TrimSpace(callee[dotIdx+1:])
				var objVal interface{}
				if isStringLiteral(objName) {
					objVal = objName[1 : len(objName)-1]
				} else if v, ok := i.env[objName]; ok {
					objVal = v
				} else {
					return nil, fmt.Errorf("unknown var %s", objName)
				}
				return callMethod(method, objVal, argsStr, i)
			}
			switch callee {
			case "str":
				inner, _ := i.evalExpr(argsStr)
				return fmt.Sprintf("%v", inner), nil
			case "len":
				av, _ := i.evalExpr(argsStr)
				switch x := av.(type) {
				case []interface{}:
					return len(x), nil
				case []string:
					return len(x), nil
				case string:
					return len(x), nil
				case *Dict:
					return x.Len(), nil
				}
				return 0, nil
			case "range":
				parts := splitArgs(argsStr)
				start, end := 0, 0
				if len(parts) == 1 {
					v, _ := i.evalExpr(parts[0])
					end = toInt(v)
				} else if len(parts) >= 2 {
					v1, _ := i.evalExpr(parts[0])
					start = toInt(v1)
					v2, _ := i.evalExpr(parts[1])
					end = toInt(v2)
				}
				var out []interface{}
				for k := start; k < end; k++ {
					out = append(out, k)
				}
				return out, nil
			default:
				if fn, ok := i.env[callee].(Func); ok {
					var argVals []interface{}
					if argsStr != "" {
						for _, a := range splitArgs(argsStr) {
							av, err := i.evalExpr(a)
							if err != nil {
								av = nil
							}
							argVals = append(argVals, av)
						}
					}
					return i.callFunc(fn, argVals), nil
				}
				return nil, fmt.Errorf("unknown func %s", callee)
			}
		}
	}
	// 数字
	if v, ok := parseNumber(expr); ok {
		return v, nil
	}
	// 变量
	if v, ok := i.env[expr]; ok {
		return v, nil
	}
	// 索引访问与切片：nums[0] / d["k"] / letters[1:3] / text[0:5]
	if li := strings.Index(expr, "["); li > 0 && strings.HasSuffix(expr, "]") {
		objName := strings.TrimSpace(expr[:li])
		idxStr := strings.TrimSpace(expr[li+1 : len(expr)-1])
		if v, ok := i.env[objName]; ok {
			// 切片
			if strings.Contains(idxStr, ":") {
				parts := strings.Split(idxStr, ":")
				start := 0
				end := -1
				if ps := strings.TrimSpace(parts[0]); ps != "" {
					if sv, err := i.evalExpr(ps); err == nil {
						start = toInt(sv)
					}
				}
				if len(parts) > 1 {
					if pe := strings.TrimSpace(parts[1]); pe != "" {
						if ev, err := i.evalExpr(pe); err == nil {
							end = toInt(ev)
						}
					}
				}
				switch obj := v.(type) {
				case []interface{}:
					if end < 0 || end > len(obj) {
						end = len(obj)
					}
					if start < 0 {
						start = 0
					}
					if start > len(obj) {
						start = len(obj)
					}
					return obj[start:end], nil
				case []string:
					if end < 0 || end > len(obj) {
						end = len(obj)
					}
					if start < 0 {
						start = 0
					}
					if start > len(obj) {
						start = len(obj)
					}
					return obj[start:end], nil
				case string:
					if end < 0 || end > len(obj) {
						end = len(obj)
					}
					if start < 0 {
						start = 0
					}
					if start > len(obj) {
						start = len(obj)
					}
					return obj[start:end], nil
				}
			}
			// 普通索引
			iv, err := i.evalExpr(idxStr)
			if err == nil {
				switch obj := v.(type) {
				case *Dict:
					key := fmt.Sprintf("%v", iv)
					if val, ok := obj.Get(key); ok {
						return val, nil
					}
				case []interface{}:
					n := toInt(iv)
					if n >= 0 && n < len(obj) {
						return obj[n], nil
					}
				case []string:
					n := toInt(iv)
					if n >= 0 && n < len(obj) {
						return obj[n], nil
					}
				case string:
					n := toInt(iv)
					if n >= 0 && n < len(obj) {
						return string(obj[n]), nil
					}
				}
			}
		}
	}
	// 复合表达式
	return evalWithPrecedence(expr, i.env)
}

func splitArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inString := false
	quote := byte(0)
	paren := 0
	bracket := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			cur.WriteByte(c)
			if c == quote && i > 0 && s[i-1] != '\\' {
				inString = false
			}
			continue
		}
		if c == '"' || c == '\'' {
			inString = true
			quote = c
			cur.WriteByte(c)
			continue
		}
		if c == '(' {
			paren++
		} else if c == ')' {
			if paren > 0 {
				paren--
			}
		} else if c == '[' {
			bracket++
		} else if c == ']' {
			if bracket > 0 {
				bracket--
			}
		}
		if c == ',' && paren == 0 && bracket == 0 {
			args = append(args, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		args = append(args, strings.TrimSpace(cur.String()))
	}
	return args
}

// inlineStrCalls 把表达式中的 str(...) 求值为字符串字面量并替换
func inlineStrCalls(expr string, i *Interpreter) string {
	const marker = "str("
	for {
		pos := strings.Index(expr, marker)
		if pos < 0 {
			break
		}
		if pos > 0 {
			prev := expr[pos-1]
			if unicode.IsLetter(rune(prev)) || unicode.IsDigit(rune(prev)) || prev == '_' || prev == '.' {
				break
			}
		}
		start := pos + len(marker)
		depth := 1
		end := -1
		for j := start; j < len(expr); j++ {
			if expr[j] == '(' {
				depth++
			} else if expr[j] == ')' {
				depth--
				if depth == 0 {
					end = j
					break
				}
			}
		}
		if end < 0 {
			break
		}
		inner := expr[start:end]
		val, err := i.evalExpr(inner)
		rep := "''"
		if err == nil {
			rep = strconv.Quote(fmt.Sprintf("%v", val))
		}
		expr = expr[:pos] + rep + expr[end+1:]
	}
	return expr
}

func parseNumber(s string) (interface{}, bool) {
	if strings.Contains(s, ".") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
		return nil, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return nil, false
}

// ============ 表达式引擎：词法 + 调度场 + 逆波兰 ============

func evalWithPrecedence(expr string, env map[string]interface{}) (interface{}, error) {
	tokens := tokenize(expr)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty expr")
	}
	prec := map[string]int{
		"or": 1,
		"and": 2,
		"==": 3, "!=": 3, ">": 3, "<": 3, ">=": 3, "<=": 3,
		"+": 4, "-": 4,
		"*": 5, "/": 5,
		"not": 6,
	}
	leftAssoc := map[string]bool{
		"or": true, "and": true,
		"==": true, "!=": true, ">": true, "<": true, ">=": true, "<=": true,
		"+": true, "-": true, "*": true, "/": true,
		"not": false,
	}
	var output []string
	var ops []string
	for _, tok := range tokens {
		if isOperatorTok(tok) {
			for len(ops) > 0 && ops[len(ops)-1] != "(" {
				top := ops[len(ops)-1]
				if prec[top] > prec[tok] || (prec[top] == prec[tok] && leftAssoc[tok]) {
					output = append(output, top)
					ops = ops[:len(ops)-1]
				} else {
					break
				}
			}
			ops = append(ops, tok)
			continue
		}
		if tok == "(" {
			ops = append(ops, tok)
			continue
		}
		if tok == ")" {
			for len(ops) > 0 && ops[len(ops)-1] != "(" {
				output = append(output, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
			if len(ops) > 0 {
				ops = ops[:len(ops)-1]
			}
			continue
		}
		output = append(output, tok)
	}
	for len(ops) > 0 {
		output = append(output, ops[len(ops)-1])
		ops = ops[:len(ops)-1]
	}
	var stack []interface{}
	for _, tok := range output {
		if isOperatorTok(tok) {
			if tok == "not" {
				if len(stack) < 1 {
					return nil, fmt.Errorf("bad not")
				}
				v := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				stack = append(stack, !toBool(v))
				continue
			}
			if len(stack) < 2 {
				return nil, fmt.Errorf("not enough operands for %s", tok)
			}
			r := stack[len(stack)-1]
			l := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			res, err := applyOp(tok, l, r)
			if err != nil {
				return nil, err
			}
			stack = append(stack, res)
			continue
		}
		val, err := resolveToken(tok, env)
		if err != nil {
			val = tok
		}
		stack = append(stack, val)
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("bad expr: %v", stack)
	}
	return stack[0], nil
}

func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inString := false
	quote := byte(0)
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			cur.WriteByte(c)
			if c == quote && i > 0 && s[i-1] != '\\' {
				inString = false
				flush()
			}
			continue
		}
		if c == '"' || c == '\'' {
			flush()
			inString = true
			quote = c
			cur.WriteByte(c)
			continue
		}
		if unicode.IsSpace(rune(c)) {
			flush()
			continue
		}
		if i+1 < len(s) {
			two := string([]byte{c, s[i+1]})
			if two == "==" || two == "!=" || two == ">=" || two == "<=" {
				flush()
				tokens = append(tokens, two)
				i++
				continue
			}
		}
		if strings.ContainsRune("()+-*/><=!.", rune(c)) {
			flush()
			tokens = append(tokens, string(c))
			continue
		}
		cur.WriteByte(c)
	}
	flush()
	return tokens
}

func isOperatorTok(s string) bool {
	switch s {
	case "+", "-", "*", "/", ">", "<", ">=", "<=", "==", "!=", "and", "or", "not":
		return true
	}
	return false
}

func isStringLiteral(s string) bool {
	return len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\''))
}

func resolveToken(tok string, env map[string]interface{}) (interface{}, error) {
	if v, ok := parseNumber(tok); ok {
		return v, nil
	}
	if isStringLiteral(tok) {
		return tok[1 : len(tok)-1], nil
	}
	if v, ok := env[tok]; ok {
		if _, isFunc := v.(Func); !isFunc {
			return v, nil
		}
	}
	if tok == "True" {
		return true, nil
	}
	if tok == "False" {
		return false, nil
	}
	return tok, nil
}

func applyOp(op string, l, r interface{}) (interface{}, error) {
	switch op {
	case "and":
		return toBool(l) && toBool(r), nil
	case "or":
		return toBool(l) || toBool(r), nil
	case "==":
		return compareEqual(l, r), nil
	case "!=":
		return !compareEqual(l, r), nil
	case ">", "<", ">=", "<=":
		ln, lok := toFloat(l)
		rn, rok := toFloat(r)
		if lok && rok {
			switch op {
			case ">":
				return ln > rn, nil
			case "<":
				return ln < rn, nil
			case ">=":
				return ln >= rn, nil
			case "<=":
				return ln <= rn, nil
			}
		}
		return false, nil
	case "+":
		if li, lok := l.(int); lok {
			if ri, rok := r.(int); rok {
				return li + ri, nil
			}
		}
		ln, lok2 := toFloat(l)
		rn, rok2 := toFloat(r)
		if lok2 && rok2 {
			return ln + rn, nil
		}
		return fmt.Sprintf("%v%v", l, r), nil
	case "-":
		if li, lok := l.(int); lok {
			if ri, rok := r.(int); rok {
				return li - ri, nil
			}
		}
		ln, lok2 := toFloat(l)
		rn, rok2 := toFloat(r)
		if lok2 && rok2 {
			return ln - rn, nil
		}
		return nil, fmt.Errorf("invalid -")
	case "*":
		if li, lok := l.(int); lok {
			if ri, rok := r.(int); rok {
				return li * ri, nil
			}
		}
		ln, lok2 := toFloat(l)
		rn, rok2 := toFloat(r)
		if lok2 && rok2 {
			return ln * rn, nil
		}
		return nil, fmt.Errorf("invalid *")
	case "/":
		ln, lok := toFloat(l)
		rn, rok := toFloat(r)
		if lok && rok && rn != 0 {
			return ln / rn, nil
		}
		return 0.0, nil
	}
	return nil, fmt.Errorf("unknown op %s", op)
}

func toBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	case nil:
		return false
	}
	return true
}

func toFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func toInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	return 0
}

func compareEqual(a, b interface{}) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func formatValue(v interface{}) string {
	switch x := v.(type) {
	case int:
		return fmt.Sprintf("%d", x)
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%.1f", x)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "True"
		}
		return "False"
	case []string:
		var items []string
		for _, it := range x {
			items = append(items, "'"+it+"'")
		}
		return "[" + strings.Join(items, ", ") + "]"
	case []interface{}:
		var items []string
		for _, it := range x {
			if s, ok := it.(string); ok {
				items = append(items, "'"+s+"'")
			} else {
				items = append(items, formatValue(it))
			}
		}
		return "[" + strings.Join(items, ", ") + "]"
	case *Dict:
		var items []string
		for _, k := range x.keys {
			v := x.vals[k]
			if s, ok := v.(string); ok {
				items = append(items, "'"+k+"': '"+s+"'")
			} else {
				items = append(items, "'"+k+"': "+formatValue(v))
			}
		}
		return "{" + strings.Join(items, ", ") + "}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func callMethod(method string, obj interface{}, argsStr string, i *Interpreter) (interface{}, error) {
	if s, ok := obj.(string); ok {
		switch method {
		case "strip":
			return strings.TrimSpace(s), nil
		case "upper":
			return strings.ToUpper(s), nil
		case "lower":
			return strings.ToLower(s), nil
		case "split":
			sep := ","
			if argsStr != "" {
				sepVal, _ := i.evalExpr(argsStr)
				if sv, ok := sepVal.(string); ok {
					sep = sv
				}
			}
			return strings.Split(s, sep), nil
		case "join":
			listVal, _ := i.evalExpr(argsStr)
			if parts, ok := listVal.([]string); ok {
				return strings.Join(parts, s), nil
			}
			if parts, ok := listVal.([]interface{}); ok {
				var strs []string
				for _, p := range parts {
					strs = append(strs, fmt.Sprintf("%v", p))
				}
				return strings.Join(strs, s), nil
			}
			return "", nil
		}
	}
	if list, ok := obj.([]interface{}); ok {
		switch method {
		case "append":
			av, _ := i.evalExpr(argsStr)
			return append(list, av), nil
		}
	}
	return nil, fmt.Errorf("unknown method %s", method)
}

func evalFString(s string, env map[string]interface{}) (interface{}, error) {
	if len(s) < 3 {
		return "", nil
	}
	inner := s[2 : len(s)-1]
	var out strings.Builder
	idx := 0
	for idx < len(inner) {
		if inner[idx] == '{' {
			j := idx + 1
			depth := 1
			for j < len(inner) && depth > 0 {
				if inner[j] == '{' {
					depth++
				} else if inner[j] == '}' {
					depth--
				}
				j++
			}
			expr := strings.TrimSpace(inner[idx+1 : j-1])
			val, err := evalWithPrecedence(expr, env)
			if err != nil {
				out.WriteString("{}")
			} else {
				out.WriteString(formatValue(val))
			}
			idx = j
			continue
		}
		out.WriteByte(inner[idx])
		idx++
	}
	return out.String(), nil
}
