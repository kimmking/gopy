package main

// gopy - Go 实现的 Python 子集解释器
//
// 架构：
//   lexer.go        词法分析（缩进 / f-string / 注释 / 括号续行）
//   ast.go          AST 节点定义
//   parser.go       递归下降语法分析
//   objects.go      运行时对象、作用域链、运算符与切片语义
//   builtins.go     内置函数、类型方法、内置模块
//   interpreter.go  树遍历求值器
//   main.go         CLI 入口

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gopy <file.py>")
		os.Exit(1)
	}
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gopy: 无法读取 %s: %v\n", path, err)
		os.Exit(1)
	}

	prog, perr := Parse(string(src))
	if perr != nil {
		fmt.Fprintf(os.Stderr, "gopy: %s: %v\n", path, perr)
		os.Exit(1)
	}

	argv := make([]Object, 0, len(os.Args)-1)
	for _, a := range os.Args[1:] {
		argv = append(argv, a)
	}

	interp := NewInterpreter(argv)
	if rerr := interp.Run(prog); rerr != nil {
		fmt.Fprintf(os.Stderr, "Traceback (most recent call last):\n  %v\n", rerr)
		os.Exit(1)
	}
}
