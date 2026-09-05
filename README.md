# gopy - Go 语言 Python 脚本解释器

本项目实现一个轻量级的 Python 子集解释器，使用 Go 语言编写。核心实现集中在 `main.go`（词法 / 表达式引擎 / 语句执行），支持 Python 常见语法子集，可用于学习解释器原理或快速原型验证。

## 目录结构

```
gopy/
├── go.mod
├── main.go          # 入口 + 词法/表达式引擎 + 语句执行引擎（核心）
├── lexer.go         # 早期词法分析器（保留）
├── parser.go        # 早期语法分析器（保留）
├── interpreter.go   # 早期求值器（保留）
├── example.py       # 示例脚本
├── tests/
│   ├── 1.py         # 算术与 print 多参数
│   ├── 2.py         # 字符串拼接与 str()
│   ├── 3.py         # 代数/逻辑/条件/字符串四类语法
│   ├── 4.py         # for+range / while / def+return / 列表操作
│   ├── 5.py         # break/continue / 字典 / 切片 / for...in list
│   ├── 6.py         # 字典方法 / 负数索引 / 切片步长 / pop / sort
│   └── 7.py         # 类与对象 / try-except 异常处理 / import math
└── README.md
```

## 环境要求

- Go 1.21 或更高（当前环境 go1.25.5）
- macOS / Linux / Windows
- 对比测试需要 `python3`

## 构建

```bash
cd /Users/kimmking/WorkBuddy/kimmking/gopy
go build -o gopy .
```

## 运行

```bash
./gopy example.py
# 或
go run . example.py
./gopy path/to/your_script.py
```

## 已支持的 Python 语法

**数据与运算**
- 变量赋值：`name = expr`
- 数字：`int` / `float`（`/` 恒返回 float，输出带 `.0`）
- 字符串：`"..."` / `'...'`，`+` 拼接
- 运算符优先级：`* /` 高于 `+ -`（调度场算法 + 逆波兰求值）
- 比较与逻辑：`> < >= <= == !=`、`and` / `or` / `not`
- 布尔字面量：`True` / `False`（输出 `True` / `False`）

**控制流**
- `if` / `elif` / `else`（基于缩进的块）
- `for x in range(n)` / `range(a, b)`
- `while cond:`
- `break` / `continue`（在 for / while 内生效）
- `def name(args):` + `return`（含函数内循环、参数绑定、局部作用域）

**字符串、列表与字典**
- f-string：`f"a={a}, b={b}"`
- 字符串方法：`strip()` / `upper()` / `lower()` / `split(sep)` / `sep.join(list)`
- 列表字面量 `[1, 2, 3]`、索引 `nums[0]`（支持负数 `nums[-1]`）、`len(list)`、`list.append(x)`
- `for x in list:` 直接遍历列表元素（也可遍历 `range()`）
- 切片：列表 `letters[1:3]` / `[:2]` / `[3:]` / `letters[::2]` / `letters[1:5:2]`，字符串同理（支持负数索引与步长）
- 列表原地方法：`list.sort()`、`list.pop()`
- 字典：`{"k": v}` 字面量、`d["k"]` 取值、`d["k"] = v` 赋值、`len(d)`（保持插入顺序）
- 字典方法：`d.keys()` / `d.values()` / `d.items()`（元组输出 `('a', 1)`）

**类与对象**
- `class Name:` 定义类，`def __init__(self, ...)` 构造器，`self.x = ...` 属性赋值
- 实例化 `obj = Name(args)`，`obj.attr` 读属性，`obj.method()` 调方法（`self` 自动绑定）
- 方法内可通过 `self.other()` 互相调用，`self.x` 在方法间共享

**异常处理**
- `try:` / `except:` 捕获运行时错误（如除零 `1 / 0`）；try 块抛错时跳到 except 块执行
- 未捕获的运行时错误由顶层 `recover` 兜底，不会导致解释器崩溃

**import 模块**
- `import math`：内置 `math.floor(x)` / `math.sqrt(x)` / `math.pi`

**内置函数**
- `print(...)` 多参数（空格分隔）、`str()`、`len()`、`range()`、`list()`

## 测试（TDD：以 python3 输出为基准）

每个测试文件都用 `python3` 执行结果作为基准，再与 `gopy` 输出逐行 diff：

```bash
cd /Users/kimmking/WorkBuddy/kimmking/gopy
go build -o gopy .
for f in tests/1.py tests/2.py tests/3.py tests/4.py tests/5.py tests/6.py tests/7.py example.py; do
  if diff <(python3 "$f") <(./gopy "$f") > /dev/null; then
    echo "$f ✓ 一致"
  else
    echo "$f ✗ 不一致"; diff <(python3 "$f") <(./gopy "$f")
  fi
done
```

当前状态：八个文件 **全部与 python3 输出完全一致**。

| 测试文件 | 覆盖点 |
|---|---|
| `tests/1.py` | 算术、print 多参数 |
| `tests/2.py` | 字符串拼接、`str()` |
| `tests/3.py` | 代数运算优先级、逻辑运算、if/elif/else、字符串方法与 f-string |
| `tests/4.py` | for+range、while、def+return、列表字面量/索引/len/append |
| `tests/5.py` | break/continue、字典、切片、for...in list |
| `tests/6.py` | 字典方法 keys/values/items、负数索引、切片步长 `[::2]`、`list.pop()` / `sort()` |
| `tests/7.py` | 类与对象、`try/except` 异常处理、`import math` |
| `example.py` | 综合示例 |

### tests/4.py 基准输出（python3）

```
for 循环:
i = 0
i = 1
i = 2
1到4求和: 10
while 循环:
count = 3
count = 2
count = 1
add(3, 4) = 7
Hello, KimmKing
列表: [10, 20, 30]
长度: 3
第一个: 10
追加后: [10, 20, 30, 40]
sum_to(5) = 15
```

## 实现要点

- 语句执行：源码先按缩进解析为 `stmt` 列表，再用 `execStmts` 递归执行；`for` / `while` 重复执行子块，`def` 注册函数体，调用时切换局部 `env`。
- 表达式求值：`tokenize` → 调度场（shunting-yard）→ 逆波兰（RPN）求值。
- `str(...)` 通过 `inlineStrCalls` 内联为字符串字面量，使 `"x + y = " + str(x + y)` 这类混合表达式回落表达式引擎。
- 列表用 `[]interface{}` 表示，`formatValue` 输出 Python 风格（`[10, 20, 30]`；字符串元素带引号）。
- 控制流信号：`execStmts` 返回 `flowKind`（normal / break / continue / return / error），循环据此响应 break/continue，函数用 `retVal` 传递返回值。
- 字典用有序结构 `Dict{keys, vals}`：Go 原生 map 无序，无法还原 Python 3.7+ 的插入顺序，必须单独维护 key 顺序才能保证 print 输出一致。
- 类与对象：`ClassDef` 记录方法体，`Instance` 用 `fields` 存属性；实例化时调用 `__init__`，`callMethodOn` 把 `self` 绑定到实例并跳过 `self` 形参，属性读写经 `env["self"]` 落到 `Instance.fields`。
- 异常处理：`try` 块用独立的 `execStmts` 子调用执行；运行时错误（如除零）通过 `panic(runtimeError)` 抛出，`execStmts` 顶层 `recover` 捕获并转为 `flowError` 信号，交由 `except` 块处理；未捕获则顶层兜底，解释器不崩溃。
- 调用结果内联：`inlineCalls` 把参与运算的调用（如 `self.double() * 2`、`len(x) + 1`）求值后替换为字面量，再回落表达式引擎，避免调用分支吞掉外围运算符。
- 点号处理：`tokenize` 让 `obj.attr` / `math.floor` / `3.7` 保持为单个 token，`resolveToken` 负责实例属性与模块常量解析。

## 下一步可扩展

更多内置模块（`random` / `datetime`）、`except` 指定异常类型、`raise` 主动抛错、继承、列表/字典推导式、lambda 表达式、关键字参数。

## 常见问题

Q: 运行报错 `read error`
A: 检查脚本路径是否正确，且文件可读。

Q: 输出与 python3 不一致？
A: 用上面的 for 循环 diff 定位，按 TDD 方式补全解释器能力（不要修改测试文件）。

## 作者

项目创建于 WorkBuddy 工作空间，用于演示 Go 实现解释器。
