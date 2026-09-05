package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ============ Token 定义 ============

type TokenType int

const (
	TkEOF TokenType = iota
	TkNewline
	TkIndent
	TkDedent
	TkName
	TkInt
	TkFloat
	TkString
	TkFString // 原始内容（未反转义），由 parser 负责切分插值表达式
	TkOp      // 运算符与标点，Text 保存其字面量
)

var tokenNames = map[TokenType]string{
	TkEOF:     "EOF",
	TkNewline: "NEWLINE",
	TkIndent:  "INDENT",
	TkDedent:  "DEDENT",
	TkName:    "NAME",
	TkInt:     "INT",
	TkFloat:   "FLOAT",
	TkString:  "STRING",
	TkFString: "FSTRING",
	TkOp:      "OP",
}

func (t TokenType) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	return "UNKNOWN"
}

type Token struct {
	Type TokenType
	Text string
	Line int
}

func (t Token) String() string {
	switch t.Type {
	case TkNewline:
		return "NEWLINE"
	case TkIndent:
		return "INDENT"
	case TkDedent:
		return "DEDENT"
	case TkEOF:
		return "EOF"
	case TkOp:
		return fmt.Sprintf("OP(%s)", t.Text)
	}
	return fmt.Sprintf("%s(%s)", t.Type, t.Text)
}

// SyntaxError 表示词法或语法阶段的错误
type SyntaxError struct {
	Line int
	Msg  string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("SyntaxError: %s (line %d)", e.Msg, e.Line)
}

// ============ 词法分析器 ============

type Lexer struct {
	src         []byte
	pos         int
	line        int
	toks        []Token
	indents     []int
	parenDepth  int
	atLineStart bool
}

// Tokenize 把源码转换为 token 列表，处理缩进块、注释与括号内的隐式续行
func Tokenize(src string) ([]Token, error) {
	l := &Lexer{
		src:         []byte(strings.ReplaceAll(src, "\r\n", "\n")),
		pos:         0,
		line:        1,
		indents:     []int{0},
		atLineStart: true,
	}
	return l.run()
}

func (l *Lexer) emit(t TokenType, text string) {
	l.toks = append(l.toks, Token{Type: t, Text: text, Line: l.line})
}

func (l *Lexer) run() ([]Token, error) {
	for l.pos < len(l.src) {
		if l.atLineStart {
			if err := l.handleLineStart(); err != nil {
				return nil, err
			}
			if l.pos >= len(l.src) {
				break
			}
		}
		c := l.src[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			l.pos++
		case c == '\\' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '\n':
			// 反斜杠显式续行
			l.pos += 2
			l.line++
		case c == '\n':
			l.pos++
			l.line++
			if l.parenDepth == 0 {
				l.emit(TkNewline, "\n")
				l.atLineStart = true
			}
			// 括号内换行直接忽略，实现隐式续行
		case c == '#':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		case c == '"' || c == '\'':
			if err := l.readString(); err != nil {
				return nil, err
			}
		default:
			if err := l.readOther(); err != nil {
				return nil, err
			}
		}
	}
	// 结尾补齐 NEWLINE 与 DEDENT
	if len(l.toks) > 0 && l.toks[len(l.toks)-1].Type != TkNewline {
		l.emit(TkNewline, "\n")
	}
	for len(l.indents) > 1 {
		l.indents = l.indents[:len(l.indents)-1]
		l.emit(TkDedent, "")
	}
	l.emit(TkEOF, "")
	return l.toks, nil
}

// handleLineStart 跳过空行/注释行，并依据缩进产出 INDENT / DEDENT
func (l *Lexer) handleLineStart() error {
	for l.pos < len(l.src) {
		blank := true
		j := l.pos
		for j < len(l.src) && l.src[j] != '\n' {
			ch := l.src[j]
			if ch == '#' {
				break
			}
			if ch != ' ' && ch != '\t' && ch != '\r' {
				blank = false
				break
			}
			j++
		}
		if !blank {
			break
		}
		for l.pos < len(l.src) && l.src[l.pos] != '\n' {
			l.pos++
		}
		if l.pos < len(l.src) {
			l.pos++
			l.line++
		}
	}
	if l.pos >= len(l.src) {
		l.atLineStart = false
		return nil
	}

	col := 0
	for l.pos < len(l.src) {
		if l.src[l.pos] == ' ' {
			col++
			l.pos++
		} else if l.src[l.pos] == '\t' {
			col += 4
			l.pos++
		} else {
			break
		}
	}
	top := l.indents[len(l.indents)-1]
	switch {
	case col > top:
		l.indents = append(l.indents, col)
		l.emit(TkIndent, "")
	case col < top:
		for len(l.indents) > 1 && l.indents[len(l.indents)-1] > col {
			l.indents = l.indents[:len(l.indents)-1]
			l.emit(TkDedent, "")
		}
		if l.indents[len(l.indents)-1] != col {
			return &SyntaxError{Line: l.line, Msg: "缩进不一致"}
		}
	}
	l.atLineStart = false
	return nil
}

// readString 读取普通字符串（支持三引号与转义）
func (l *Lexer) readString() error {
	q := l.src[l.pos]
	triple := l.pos+2 < len(l.src) && l.src[l.pos+1] == q && l.src[l.pos+2] == q
	var sb strings.Builder
	if triple {
		l.pos += 3
		for {
			if l.pos >= len(l.src) {
				return &SyntaxError{Line: l.line, Msg: "字符串未闭合"}
			}
			if l.src[l.pos] == q && l.pos+2 < len(l.src) && l.src[l.pos+1] == q && l.src[l.pos+2] == q {
				l.pos += 3
				break
			}
			s, err := l.readEsc()
			if err != nil {
				return err
			}
			sb.WriteString(s)
		}
	} else {
		l.pos++
		for {
			if l.pos >= len(l.src) || l.src[l.pos] == '\n' {
				return &SyntaxError{Line: l.line, Msg: "字符串未闭合"}
			}
			if l.src[l.pos] == q {
				l.pos++
				break
			}
			s, err := l.readEsc()
			if err != nil {
				return err
			}
			sb.WriteString(s)
		}
	}
	l.emit(TkString, sb.String())
	return nil
}

// readFString 读取 f-string 的 RAW 内容（保留 {} 与转义原样，交由 parser 处理）
func (l *Lexer) readFString() error {
	// 此时 l.pos 指向前缀后的引号
	q := l.src[l.pos]
	triple := l.pos+2 < len(l.src) && l.src[l.pos+1] == q && l.src[l.pos+2] == q
	var sb strings.Builder
	if triple {
		l.pos += 3
		for {
			if l.pos >= len(l.src) {
				return &SyntaxError{Line: l.line, Msg: "f-string 未闭合"}
			}
			if l.src[l.pos] == q && l.pos+2 < len(l.src) && l.src[l.pos+1] == q && l.src[l.pos+2] == q {
				l.pos += 3
				break
			}
			if l.src[l.pos] == '\n' {
				l.line++
			}
			sb.WriteByte(l.src[l.pos])
			l.pos++
		}
	} else {
		l.pos++
		for {
			if l.pos >= len(l.src) || l.src[l.pos] == '\n' {
				return &SyntaxError{Line: l.line, Msg: "f-string 未闭合"}
			}
			if l.src[l.pos] == q {
				l.pos++
				break
			}
			sb.WriteByte(l.src[l.pos])
			l.pos++
		}
	}
	l.emit(TkFString, sb.String())
	return nil
}

// readEsc 读取一个字符（处理反斜杠转义），返回解码后的字符串。
// 注意：必须按字节原样输出，否则 UTF-8 多字节字符会被错误地按 code point 重新编码。
func (l *Lexer) readEsc() (string, error) {
	if l.pos >= len(l.src) {
		return "", &SyntaxError{Line: l.line, Msg: "字符串未闭合"}
	}
	c := l.src[l.pos]
	if c != '\\' {
		l.pos++
		if c == '\n' {
			l.line++
		}
		return string([]byte{c}), nil
	}
	if l.pos+1 >= len(l.src) {
		return "", &SyntaxError{Line: l.line, Msg: "字符串未闭合"}
	}
	e := l.src[l.pos+1]
	l.pos += 2
	switch e {
	case 'n':
		return "\n", nil
	case 't':
		return "\t", nil
	case 'r':
		return "\r", nil
	case 'a':
		return "\a", nil
	case 'b':
		return "\b", nil
	case 'f':
		return "\f", nil
	case 'v':
		return "\v", nil
	case '0':
		return "\x00", nil
	case '\\':
		return "\\", nil
	case '\'':
		return "'", nil
	case '"':
		return "\"", nil
	case '\n':
		l.line++
		return "", nil
	default:
		return "\\" + string([]byte{e}), nil
	}
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// readOther 读取标识符 / 数字 / 运算符
func (l *Lexer) readOther() error {
	c := l.src[l.pos]
	if isNameStart(c) {
		start := l.pos
		for l.pos < len(l.src) && isNameChar(l.src[l.pos]) {
			l.pos++
		}
		word := string(l.src[start:l.pos])
		// f-string 前缀
		if (word == "f" || word == "F") && l.pos < len(l.src) && (l.src[l.pos] == '"' || l.src[l.pos] == '\'') {
			return l.readFString()
		}
		l.emit(TkName, word)
		return nil
	}
	if isDigit(c) || (c == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1])) {
		return l.readNumber()
	}
	return l.readOperator()
}

func (l *Lexer) readNumber() error {
	start := l.pos
	isFloat := false
	// 十六进制
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) && (l.src[l.pos+1] == 'x' || l.src[l.pos+1] == 'X') {
		l.pos += 2
		s := l.pos
		for l.pos < len(l.src) && (isDigit(l.src[l.pos]) || (l.src[l.pos] >= 'a' && l.src[l.pos] <= 'f') || (l.src[l.pos] >= 'A' && l.src[l.pos] <= 'F')) {
			l.pos++
		}
		if s == l.pos {
			return &SyntaxError{Line: l.line, Msg: "非法的十六进制字面量"}
		}
		v, err := strconv.ParseInt(string(l.src[s:l.pos]), 16, 64)
		if err != nil {
			return &SyntaxError{Line: l.line, Msg: "十六进制字面量超出范围"}
		}
		l.emit(TkInt, strconv.FormatInt(v, 10))
		return nil
	}
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		isFloat = true
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		save := l.pos
		j := l.pos + 1
		if j < len(l.src) && (l.src[j] == '+' || l.src[j] == '-') {
			j++
		}
		if j < len(l.src) && isDigit(l.src[j]) {
			isFloat = true
			l.pos = j
			for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
				l.pos++
			}
		} else {
			l.pos = save
		}
	}
	text := string(l.src[start:l.pos])
	if isFloat {
		if _, err := strconv.ParseFloat(text, 64); err != nil {
			return &SyntaxError{Line: l.line, Msg: "非法的数字字面量 " + text}
		}
		l.emit(TkFloat, text)
		return nil
	}
	if _, err := strconv.ParseInt(text, 10, 64); err != nil {
		return &SyntaxError{Line: l.line, Msg: "非法的数字字面量 " + text}
	}
	l.emit(TkInt, text)
	return nil
}

// 多字符运算符按长度降序匹配
var operators = []string{
	"**=", "//=", ">>=", "<<=",
	"==", "!=", "<=", ">=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "->", ":=",
	"**", "//", "<<", ">>",
	"+", "-", "*", "/", "%", "=", "<", ">",
	"(", ")", "[", "]", "{", "}", ",", ":", ".", ";", "@", "~", "&", "|", "^",
}

func (l *Lexer) readOperator() error {
	for _, op := range operators {
		if l.pos+len(op) <= len(l.src) && string(l.src[l.pos:l.pos+len(op)]) == op {
			switch op {
			case "(", "[", "{":
				l.parenDepth++
			case ")", "]", "}":
				if l.parenDepth > 0 {
					l.parenDepth--
				}
			}
			l.pos += len(op)
			l.emit(TkOp, op)
			return nil
		}
	}
	return &SyntaxError{Line: l.line, Msg: fmt.Sprintf("非法字符 %q", string(l.src[l.pos:l.pos+1]))}
}
