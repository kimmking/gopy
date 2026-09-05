# gopy - Go 语言实现的 Python 子集解释器

用 Go 编写的 Python 子集解释器，采用标准的 **词法 → 语法（AST）→ 树遍历求值** 三段式架构。
以 `python3` 的输出为基准做 TDD 回归验证，可用于学习解释器原理或做轻量脚本原型。

## 目录结构

```
gopy/
├── go.mod
├── main.go          # CLI 入口
├── lexer.go         # 词法分析：缩进 INDENT/DEDENT、f-string、注释、括号续行
├── ast.go           # AST 节点定义（语句 / 表达式）
├── parser.go        # 递归下降语法分析器
├── objects.go       # 运行时对象、作用域链、repr/str、运算符与切片语义
├── builtins.go      # 内置函数、内建类型方法、math/os/sys 模块
├── interpreter.go   # 树遍历求值器（控制流、异常、类、推导式）
├── example.py       # 综合示例
├── tests/           # 回归用例，1~7 为基础语法，8~11 为进阶特性
└── test.sh          # 与 python3 输出逐文件 diff
```

## 构建与运行

```bash
go build -o gopy .
./gopy example.py
# 或
go run . path/to/script.py
```

回归测试（需要 `python3`）：

```bash
./test.sh
```

当前状态：`tests/1~11.py` 与 `example.py` **全部与 python3 输出完全一致**。

## 架构说明

```
源码 → Tokenize()     词法分析，产出 token 流（含 INDENT / DEDENT）
     → Parse()        递归下降解析，产出 AST
     → Interpreter    树遍历求值（Environment 作用域链 + signal 控制流）
```

### 1. 词法分析（`lexer.go`）

- 缩进敏感：维护缩进栈，产出 `INDENT` / `DEDENT`，缩进不一致时报 `SyntaxError`。
- 括号内换行被忽略，天然支持隐式续行；同时支持反斜杠显式续行。
- 行尾 `#` 注释在词法阶段剥离，因此 `x = 1  # 注释` 不会被当成表达式的一部分。
- 字符串支持三引号与转义序列；f-string 保留 RAW 内容，交由 parser 切分插值表达式。
- 支持 `0x` 十六进制、科学计数法、多字符运算符（`**` `//` `<<` `>>` `+=` 等）。

### 2. 语法分析（`parser.go`）

递归下降，优先级由低到高：

```
or < and < not < 比较(含 in/is) < | < ^ < & < 移位 < +- < */ // % < 一元 < ** < 后缀 < 原子
```

关键点：`not` 的层级低于比较运算，因此 `not a == b` 解析为 `not (a == b)`，与 Python 一致。
`for` / 推导式的迭代目标使用受限的 `parseTarget()`，避免 `in` 被误判为成员运算符。

### 3. 求值（`interpreter.go` + `objects.go`）

- **作用域链**：`Environment` 单向链表，赋值在当前作用域建立绑定（Python 语义），读取沿链向上查找。函数闭包捕获定义时的 `Env`。
- **控制流信号**：`return` / `break` / `continue` 通过 `signal` 逐层返回，不再使用共享字段，因此递归调用的返回值不会互相覆盖。
- **异常**：运行时错误统一为 `*PyException`（实现 `error` 接口），`try/except` 按类型匹配捕获，支持 `except X as e`、`else`、`finally`、`raise`。
- **对象模型**：基础值直接用 Go 原生类型（`int` / `float64` / `bool` / `string`），容器与可调用对象用指针类型（`*List` / `*Dict` / `*Function` / `*Instance` ...），天然具备引用语义。
- **字典与集合**：以 `keyOf()` 归一化键（数值按值归一化，故 `1` 与 `1.0` 同键），并单独维护插入顺序以还原 Python 3.7+ 的有序语义。

## 已支持的语法

**数据与运算**

- `int` / `float` / `bool` / `str` / `None`；`/` 恒返回 float
- 运算符：`+ - * / // % **`、比较 `> < >= <= == !=`、链式比较 `1 < x < 10`
- 逻辑：`and` / `or` / `not`（短路求值）；位运算 `& | ^ ~ << >>`
- 成员与身份：`in` / `not in` / `is` / `is not`
- 增量赋值 `+= -= *= /= //= %= **=`，链式赋值 `a = b = 1`，元组解包（含嵌套）
- 序列运算：字符串/列表/元组拼接，`str * n` 与 `list * n` 重复

**控制流**

- `if` / `elif` / `else`、`while ... else`、`for ... in ... else`
- `break` / `continue` / `pass`
- 遍历目标：list / tuple / str / dict（键）/ set / range

**函数**

- `def` 定义，位置参数、默认参数、关键字参数、`*args`、`**kwargs`
- 递归、闭包、嵌套函数、作用域隔离
- `lambda` 与 `map` / `filter` / `sorted(key=...)` 等高阶函数

**字符串、列表、字典、集合、元组**

- f-string：插值、转换符 `!r` `!s`、格式说明符（宽度/对齐/精度/`d x b o e f g %`）
- 字符串：`upper lower capitalize title swapcase strip lstrip rstrip split rsplit splitlines join startswith endswith find rfind index count replace isdigit isalpha isalnum isspace isupper islower ljust rjust center zfill`
- 列表：`append extend insert remove pop clear index count sort(key/reverse) reverse copy`
- 字典：`keys values items get pop setdefault update clear copy`（保持插入顺序）
- 集合：`add remove discard clear copy union intersection difference`
- 元组：`count index`
- 切片：完整 Python 语义，支持负数下标、负步长（`s[::-1]`）、越界截断

**类与对象**

- `class` 定义、`__init__` 构造、`self` 自动绑定
- 单继承（方法沿父链查找）、`isinstance`、类属性
- `print(obj)` 优先调用 `__str__`，否则输出 `<Dog object at 0x...>`

**推导式**

- 列表推导式、字典推导式，支持多层 `for` 与 `if` 过滤，且拥有独立作用域

**异常处理**

- `try` / `except` / `except X as e` / `else` / `finally`
- `raise ValueError("msg")`、`assert`
- 内建异常类型：`ValueError TypeError IndexError KeyError ZeroDivisionError NameError AttributeError RuntimeError AssertionError NotImplementedError RecursionError ...`
- 未捕获的异常输出到 stderr 并以非零码退出，解释器本身不崩溃

**模块**

- `import math` / `import os` / `import sys`，支持 `import x as y`、`from x.y import z`
- `math`：`floor ceil trunc sqrt fabs exp log log10 log2 pow hypot sin cos tan asin acos atan atan2 degrees radians fmod copysign modf factorial gcd`，常量 `pi e tau inf nan`
- `os`：`getcwd listdir getpid name sep`，以及 `os.path.join exists isfile isdir basename dirname abspath splitext`
- `sys`：`argv version platform exit`

**内置函数**

`print len str repr type int float bool list tuple dict set range abs round pow divmod min max sum sorted reversed enumerate zip map filter any all chr ord hex oct bin isinstance hasattr getattr setattr id input exit`

## 实现要点与已知取舍

- **`math.Degrees` / `math.Radians`**：Go 标准库没有这两个函数，按定义 `f * (180 / π)`、`f * (π / 180)` 换算；系数先算再乘，与 CPython 结果逐位一致。
- **包级初始化循环**：`Repr` 需要调用实例 `__str__`，而求值器又依赖方法表。通过 `instanceStrHook` 与 `callObjectRef` 两个函数变量间接引用来打破循环。
- **UTF-8**：词法分析按字节扫描，多字节字符必须原样输出（`string([]byte{c})`），不能走 `string(c)` 的 code point 转换，否则中文会变成乱码。
- **尚未实现**：`with` 语句、生成器与 `yield`、`str.format`、装饰器、多继承、关键字-only 参数、`global` 之外的 `nonlocal`、模块文件导入（只能导入内置模块）。
- 数字使用 Go `int` / `float64`，因此没有 Python 的任意精度整数。

## 常见问题

Q: 输出与 python3 不一致？
A: 用 `./test.sh` 定位到具体文件，按 TDD 方式补全解释器能力（不要修改测试文件）。

Q: 运行时报 `SyntaxError`？
A: 检查是否使用了"尚未实现"中列出的语法。

## 作者

项目创建于 WorkBuddy 工作空间，用于演示 Go 实现解释器。
