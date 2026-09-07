# gopy - Go 语言实现的 Python 子集解释器

用 Go 编写的 Python 子集解释器，采用标准的 **词法 → 语法（AST）→ 树遍历求值** 三段式架构。
以 `python3` 的输出为基准做 TDD 回归验证，可用于学习解释器原理或做轻量脚本原型。

> 设计目标不是 100% 兼容 CPython，而是覆盖最常用的语法与内置能力，并在每条特性上做到与 `python3` 输出**逐字节一致**（见 `tests/` 回归用例）。

## 目录结构

```
gopy/
├── go.mod
├── main.go          # CLI 入口：解析参数、构造全局环境、调用解释器
├── lexer.go         # 词法分析：缩进 INDENT/DEDENT、f-string、注释、括号续行、多字符运算符
├── ast.go           # AST 节点定义（语句 / 表达式 / 模式）
├── parser.go        # 递归下降语法分析器：含运算符优先级、推导式、异常处理
├── objects.go       # 运行时对象、作用域链、repr/str、运算符与切片语义、字符串格式化
├── builtins.go      # 内置函数、内建类型方法（str/list/dict/tuple/set）、math/os/sys 模块
├── interpreter.go   # 树遍历求值器（控制流、异常、类、推导式、import）
├── example.py       # 综合示例
├── tests/           # 回归用例 01~15（两位补零命名，便于排序），覆盖基础到进阶特性
└── test.sh          # 与 python3 输出逐文件 diff 的 TDD 回归脚本
```

## 快速开始

```bash
go build -o gopy .        # 编译解释器
./gopy example.py         # 运行脚本
# 或
go run . path/to/script.py
```

回归测试（需要本机安装 `python3` 作为基准）：

```bash
./test.sh
```

当前状态：`tests/01.py` … `tests/25.py` 与 `example.py` **全部与 `python3` 输出完全一致**。

## 整体架构

```
源码 ──► Tokenize()   词法分析，产出 token 流（含 INDENT / DEDENT、f-string 片段）
    ──► Parse()      递归下降解析，产出 AST（语句与表达式节点）
    ──► Interpreter  树遍历求值（Environment 作用域链 + signal 控制流信号）
```

解释器是**树遍历（tree-walking）** 式，没有字节码/虚拟机层，结构与教学型解释器一致，便于阅读与扩展。

## 设计详解

### 1. 词法分析（`lexer.go`）

- **缩进敏感**：维护一个缩进栈。遇到比栈顶更深的缩进时压栈并产出 `INDENT`；遇到更浅的缩进时连续弹栈产出 `DEDENT`。缩进不一致（既不匹配栈内任何层级）直接报 `SyntaxError`。
- **隐式 / 显式续行**：处于 `()` / `[]` / `{}` 内时忽略换行，天然支持多行表达式；同时支持行尾反斜杠 `\` 显式续行。
- **注释剥离**：行尾 `#` 注释在词法阶段被去除，因此 `x = 1  # 注释` 不会被当成表达式的一部分。
- **字符串**：支持普通字符串、三引号字符串与转义序列；f-string 在词法阶段保留 RAW 文本，交由 parser 切分 `{...}` 插值表达式。
- **字面量与运算符**：支持 `0x` 十六进制、科学计数法（如 `2e10`）、多字符运算符（`**`、`//`、`<<`、`+=` 等），通过运算符表匹配最长匹配。

### 2. 语法分析（`parser.go`）

递归下降，运算符优先级由低到高（与 Python 一致）：

```
or < and < not < 比较(含 in/is) < | < ^ < & < 移位 < +- < */ // % < 一元 < ** < 后缀 < 原子
```

关键细节：

- `not` 的层级低于比较运算，因此 `not a == b` 解析为 `not (a == b)`，与 Python 一致。
- `for` / 推导式的迭代目标使用受限的 `parseTarget()`，避免 `in` 被误判为成员运算符。
- 支持 `if/elif/else`、`while ... else`、`for ... in ... else`、`try/except/else/finally`、推导式、类定义、异常 `raise/assert` 等复合结构。

### 3. 求值器（`interpreter.go` + `objects.go`）

- **作用域链**：`Environment` 是一个单向链表。赋值在当前作用域建立绑定（Python 语义），读取沿链表向上查找。函数闭包在定义时捕获其外层 `Env`，因此嵌套函数、闭包、递归都能正确工作。
- **控制流信号**：`return` / `break` / `continue` 通过 `signal` 结构体逐层返回，而不是共享的全局字段——这样递归调用各自的返回/中断信号不会互相覆盖（支持如 `fib()` 递归、`make_adder()` 闭包）。
- **异常**：运行时错误统一为 `*PyException`（实现 Go 的 `error` 接口），携带异常类型与消息。`try/except` 按类型匹配，支持 `except X as e`、`else`、`finally`、`raise`。未捕获异常输出到 stderr 并以非零码退出，解释器本身不会崩溃。
- **对象模型**：基础值直接用 Go 原生类型（`int` / `float64` / `bool` / `string`）；容器与可调用对象用指针类型（`*List` / `*Dict` / `*Tuple` / `*Set` / `*Function` / `*Instance` / `*Class` …），天然具备引用语义。
- **字典与集合**：用 `keyOf()` 归一化键——数值按值归一化，因此 `1` 与 `1.0` 视为同一键；并单独维护插入顺序以还原 Python 3.7+ 的有序字典语义。
- **切片与下标**：实现完整 Python 语义，支持负数下标、`s[::-1]` 负步长、`s[1:4:2]` 区间步长，以及越界安全截断。

### 4. 模块系统（`builtins.go`）

`import math` / `import os` / `import sys` 加载内置 `*Module`；支持 `import x as y`、`from x import a, b`。`Module` 是一组绑定了 `Builtin`/`func` 的 `Attrs`，`os.path` 作为子模块挂载在 `os.Attrs["path"]` 上。

### 5. 字符串格式化实现（`objects.go`）

两种格式化能力都实现在运行时的运算符/方法层，并以 `python3` 输出为基准校准：

- **printf 风格 `%` 运算符**：在 `binaryOp` 的 `%` 分支中，当左操作数是字符串时调用 `percentFormat`。它扫描格式串，解析 `%` 转换说明符（`[[#0- +-]width[.prec]type]` 以及 `%(name)s` 字典映射），再用共享的 `fmtInt` / `fmtFloat` 与 `padFieldA` 生成结果。支持 `%s %r %d %i %u %o %x %X %b %e %E %f %F %g %G %c %%`，以及宽度、精度、对齐、符号与 `#` 交替形式标志。
- **`str.format(...)`**：`formatString` 解析 `{...}` 占位符，支持自动编号 `{}`、`{0}` 位置索引、`{name}` 关键字（`**kwargs`）、`!r` / `!s` 转换，以及 `:` 之后的格式说明符（填充/对齐/符号/`#`/`0`/宽度/`.`精度/类型）。底层复用同一套 `fmtInt` / `fmtFloat` / `padFieldA` 渲染逻辑，保证两种格式化风格行为一致。

## 已支持的语法

**数据与运算**

- `int` / `float` / `bool` / `str` / `None`；`/` 恒返回 float
- 运算符：`+ - * / // % **`、比较 `> < >= <= == !=`、链式比较 `1 < x < 10`
- 逻辑：`and` / `or` / `not`（短路求值）；位运算 `& | ^ ~ << >>`
- 成员与身份：`in` / `not in` / `is` / `is not`
- 增量赋值 `+= -= *= /= //= %= **= &= |= ^= <<= >>=`，链式赋值 `a = b = 1`，元组解包（含嵌套）
- 集合运算符 `| & - ^`；字典合并 `|` 与 `|=`（Python 3.9+）
- 序列运算：字符串/列表/元组拼接，`str * n` 与 `list * n` 重复
- 三元条件表达式：`a if cond else b`

**控制流**

- `if` / `elif` / `else`、`while ... else`、`for ... in ... else`
- `break` / `continue` / `pass`
- `with` 语句：多上下文项（`with a() as x, b() as y:`）、按相反顺序调用 `__exit__`、异常信息传入 `__exit__`、返回 `True` 抑制异常；`__enter__`/`__exit__` 协议
- 遍历目标：list / tuple / str / dict（键）/ set / range / 生成器

**生成器**

- `yield` 语句：生成器函数惰性求值（协程式实现，函数体在独立 goroutine 中运行，通过无缓冲信道与调用方交替执行）
- 生成器表达式 `(x*x for x in it if c)`，含免括号调用实参形式 `sum(x for x in it)`
- `next(g)` / `next(g, default)` / `iter(g)`；`list` / `tuple` / `sum` / `for` 等直接消费生成器；耗尽后再次迭代为空

**函数**

- `def` 定义，位置参数、默认参数、关键字参数、`*args`、`**kwargs`
- 调用处 `*iterable` / `**mapping` 参数展开（如 `f(*nums, **opts)`）
- 递归、闭包、嵌套函数、作用域隔离；`global` 与 `nonlocal` 声明
- 装饰器：`@dec`、堆叠装饰、带参数的装饰器工厂，可修饰函数与类（以及类中的方法）
- `lambda` 与 `map` / `filter` / `sorted(key=...)` 等高阶函数
- 函数 `__name__` 属性

**字符串、列表、字典、集合、元组**

- f-string：插值、转换符 `!r` `!s`、格式说明符（宽度/对齐/精度/`d x b o e f g %`）
- 字符串：`upper lower capitalize title swapcase strip lstrip rstrip split rsplit splitlines join startswith endswith find rfind index count replace isdigit isalpha isalnum isspace isupper islower ljust rjust center zfill partition rpartition istitle isnumeric isdecimal casefold removeprefix removesuffix expandtabs maketrans translate`（startswith/endswith 支持元组，ljust/rjust/center 支持填充字符），以及 printf 风格格式化 `str % (...)`（`%s %r %d %i %u %o %x %X %b %e %E %f %F %g %G %c %%`，支持宽度/精度/对齐/`#`/`+`/` ` 标志与 `%(name)s` 字典映射）与 `str.format(...)`（`{}` 自动编号、`{0}`/`{name}`、`:` 格式说明符、`!r`/`!s` 转换）
- 列表：`append extend insert remove pop clear index count sort(key/reverse) reverse copy`
- 字典：`keys values items get pop setdefault update clear copy fromkeys`（保持插入顺序）
- min/max：`key` 与 `default` 关键字；`sum`：`start` 关键字
- 集合：`add remove discard clear copy union intersection difference`
- 元组：`count index`
- 切片：完整 Python 语义，支持负数下标、负步长（`s[::-1]`）、越界截断

**类与对象**

- `class` 定义、`__init__` 构造、`self` 自动绑定
- 多继承与 C3 方法解析顺序（MRO）、零参 `super()`（菱形继承协作式 `__init__` 正确）、`isinstance` / `issubclass`、类属性（含类属性赋值 `Cls.attr = v`）
- 运算符重载：`__add__ __sub__ __mul__ __truediv__ __floordiv__ __mod__ __pow__` 及其反射形式 `__r*__`、`__eq__ __ne__ __lt__ __le__ __gt__ __ge__`、`__len__ __getitem__ __setitem__ __contains__ __call__ __bool__`
- `__repr__` / `__str__` 分工与 CPython 一致：`repr()` 用 `__repr__`，`str()` 优先 `__str__` 回退 `__repr__`
- `@property` / `@staticmethod` / `@classmethod` 描述符（classmethod 绑定动态类）
- `print(obj)` 优先调用 `__str__`，否则输出 `<Dog object at 0x...>`

**推导式**

- 列表推导式、字典推导式，支持多层 `for` 与 `if` 过滤，且拥有独立作用域（不向外层泄漏循环变量）

**异常处理**

- `try` / `except` / `except X as e` / `else` / `finally`
- `raise ValueError("msg")`、`raise X from Y`（`__cause__`）、`assert`、try/finally（无 except）
- 自定义异常类：`class MyError(Exception)` 继承内建异常类型，`except` 按继承链匹配
- 异常实例即对象：`e.args`、`str(e)`/`repr(e)`（KeyError 的 str 显示 key 的 repr）、`type(e).__name__`、可先创建再 raise
- `except (A, B)` 元组匹配
- 内建异常类型：`ValueError TypeError IndexError KeyError ZeroDivisionError NameError AttributeError RuntimeError AssertionError NotImplementedError RecursionError ...`（含 CPython 继承链，如 `issubclass(ValueError, Exception)`）
- 未捕获的异常输出到 stderr 并以非零码退出，解释器本身不崩溃

**模块**

- `import math` / `import os` / `import sys` / `import collections` / `import functools` / `import itertools` / `import json`，支持 `import x as y`、`from x.y import z`、括号 `from x import (a, b)`

**内置函数**

`print len str repr type int float bool list tuple dict set range abs round pow divmod min max sum sorted reversed enumerate zip map filter any all chr ord hex oct bin isinstance issubclass hasattr getattr setattr id input exit long next iter property staticmethod classmethod super format callable`

> `long` 为兼容 Python 2 的长整型转换，在本解释器中与 `int` 等价（支持 `base`）。

## 内置模块清单

**`math`**：`floor ceil trunc sqrt fabs exp log log10 log2 pow hypot sin cos tan asin acos atan atan2 degrees radians fmod copysign modf factorial gcd sinh cosh tanh asinh acosh atanh log1p expm1 erf erfc gamma lgamma isqrt dist comb perm lcm prod isfinite isinf isnan remainder nextafter ulp frexp`，常量 `pi e tau inf nan`

**`os`**：`getcwd listdir getpid getenv environ mkdir makedirs remove unlink rmdir rename chdir name sep`，以及 `os.path` 子模块：`join exists isfile isdir basename dirname abspath splitext split normpath realpath isabs relpath commonpath getsize`

**`sys`**：`argv version platform exit`

**`json`**：`dumps`（`indent` / `sort_keys` / `separators` / `ensure_ascii`、整数键转换）、`loads`（完整 JSON 解析：对象/数组/字符串转义含 `\uXXXX` 代理对/整数与浮点/字面量）

**`functools`**：`reduce partial lru_cache`（带 `cache_info()`，可作装饰器或装饰器工厂）

**`itertools`**：`count cycle repeat chain islice accumulate product permutations combinations takewhile dropwhile starmap`（惰性迭代器，可被 `next` / `list` / `for` 消费）

**`collections`**：`Counter`（`most_common elements update subtract`、缺失键返回 0、repr 按 most_common 排序）、`defaultdict`（缺失键调用工厂函数）、`OrderedDict`（`move_to_end popitem`）、`deque`（`append appendleft pop popleft extend extendleft rotate remove clear reverse copy index count`）

## 测试目录（回归用例）

`test.sh` 会依次用 `python3` 与 `./gopy` 运行每个脚本并 `diff`，全部一致才通过。每个用例聚焦一类特性：

| 文件 | 测试内容点 |
| --- | --- |
| `tests/01.py` | 基础赋值与算术：`*`、`+`，`print` 多参数拼接 |
| `tests/02.py` | 变量运算、`str()` 拼接、字符串字面量输出 |
| `tests/03.py` | 代数式求值、逻辑 `and/or/not`、条件分支、字符串 `strip/upper/lower`、`split/join`、f-string 插值 |
| `tests/04.py` | `for`+`range`、`while`、函数定义/调用、列表 `append`、函数内循环（求和） |
| `tests/05.py` | `for ... in list`、`break`/`continue`、字典增删查与 `len`、列表/字符串切片 |
| `tests/06.py` | 字典 `keys/values/items`、负数索引、切片步长（`[::2]`）、`list.sort()`/`pop()` |
| `tests/07.py` | 类与 `__init__`/`self`、异常 `try/except`、`import math` 与基础数学函数 |
| `tests/08.py` | `// % **`、位运算、`not` 优先级、链式比较、负步长切片、`range` 步长、序列重复/拼接 |
| `tests/09.py` | 递归 `fib`、闭包 `make_adder`、默认/关键字参数、`*args/**kwargs`、嵌套函数、`lambda`/`map`/`filter`/`sorted`、推导式独立作用域、`max/min/sum/divmod/round/hex/oct/bin/chr/ord/any/all/enumerate/zip/reversed` |
| `tests/10.py` | 类继承与类属性、`__str__`、方法共享 `self` 状态、`raise`/`except ... as`/`finally`、按类型分派异常、`try/else`、`assert` |
| `tests/11.py` | 增量/链式赋值、元组解包、隐式/反斜杠续行、行尾注释、`for/while ... else`、列表/字典推导式、f-string 格式说明符、集合、`from math import` 与 `os` 基础 |
| `tests/12.py` | `while True`+`break`、分支内定义函数、`sorted(reverse/key)`、嵌套容器、三引号字符串、`del`、`global` 声明、单元素元组、容器相等性、布尔参与算术、`1 == 1.0`、三元条件表达式 |
| `tests/13.py` | `math` 增强：三角/反三角/双曲、`log1p/expm1`、`erf/erfc/gamma`、`isqrt/dist/comb/perm/lcm/prod`、`isfinite/isinf/isnan`、`remainder/nextafter/frexp`；`os` 增强：`sep/name/getcwd/environ/getenv`、`os.path` 全套方法 |
| `tests/14.py` | 类型转换与数值操作：`int/long/float/str/repr/tuple/list/chr/ord/hex/oct/bin`（含 `base` 进制、字符串参数、布尔转换） |
| `tests/15.py` | 字符串增强与格式化：printf 风格 `%s %d %x %f`（宽度/精度/对齐/符号/`#`/字典映射）、新增方法 `partition/rpartition/istitle/isnumeric/isdecimal/casefold/removeprefix/removesuffix/expandtabs`、`str.format` 自动编号/位置/关键字/格式说明符 |
| `tests/16.py` | `nonlocal` 闭包计数、调用处 `*args/**kwargs` 展开、装饰器（堆叠/带参/透传/记录调用）、函数 `__name__` |
| `tests/17.py` | 生成器：`yield`、`next()`/默认值/StopIteration、惰性求值、生成器表达式（含免括号实参）、生成器管道、耗尽后再迭代 |
| `tests/18.py` | `with` 语句：进入/退出顺序、异常传播与抑制（`__exit__` 返回 True）、嵌套与多上下文项、资源类、`return`/`break` 穿过 with、`exc_type.__name__` |
| `tests/19.py` | 运算符重载（算术/反射/比较/`len`/下标/`in`/`__call__`/`__bool__`）、`__repr__`/`__str__` 分工、`@property`/`@staticmethod`/`@classmethod`、类属性赋值 |
| `tests/20.py` | 多继承 C3 MRO、零参 `super()`（方法链/菱形协作 `__init__`/类方法中）、`isinstance`/`issubclass`、Mixin |
| `tests/21.py` | collections：Counter（计数/most_common/elements/update）、defaultdict（list/int/str 工厂）、OrderedDict（move_to_end/popitem）、deque（双端操作/rotate） |
| `tests/22.py` | functools：reduce/partial/lru_cache（含递归缓存与 cache_info）；itertools：count/cycle/repeat/chain/islice/accumulate/product/permutations/combinations/takewhile/dropwhile/starmap；括号 from-import |
| `tests/23.py` | 自定义异常类继承链匹配、`e.args`、`raise from` 与 `__cause__`、except 元组、异常实例即对象（str/repr）、try/finally return 覆盖 |
| `tests/24.py` | `str.maketrans/translate`、元组 startswith/endswith、填充字符、`dict.fromkeys`、dict/set 运算符、min/max default、`format`/`callable`、`sum(start=)` |
| `tests/25.py` | json：dumps（indent/sort_keys/separators/ensure_ascii/转义/整数键）、loads（对象/数组/字符串/数字/字面量）、round trip |
| `example.py` | 综合示例（脚本级冒烟测试） |

## 实现要点与已知取舍

- **`math.Degrees` / `math.Radians`**：Go 标准库没有这两个函数，按定义 `f * (180 / π)`、`f * (π / 180)` 换算；系数先算再乘，与 CPython 结果逐位一致。
- **包级初始化循环**：`Repr` 需要调用实例 `__str__`，而求值器又依赖方法表。通过 `instanceStrHook` 与 `callObjectRef` 两个函数变量间接引用来打破循环。
- **UTF-8**：词法分析按字节扫描，多字节字符必须原样输出（`string([]byte{c})`），不能走 `string(c)` 的 code point 转换，否则中文会变成乱码。
- **数字精度**：使用 Go `int` / `float64`，因此没有 Python 的任意精度整数；超大整数运算与 CPython 行为可能存在差异。
- **`newExc` 格式化**：内部使用 `fmt.Sprintf`，错误信息中的字面量 `%` 必须写成 `%%`，否则会被误判为格式动词。
- **尚未实现**：关键字-only 参数、模块文件导入（只能导入内置模块）、`json.dump/load` 文件接口。

## 常见问题

Q: 输出与 `python3` 不一致？
A: 用 `./test.sh` 定位到具体文件，按 TDD 方式补全解释器能力（不要修改测试文件）。

Q: 运行时报 `SyntaxError`？
A: 检查是否使用了"尚未实现"中列出的语法。

Q: 如何新增一个回归用例？
A: 在 `tests/` 下新增两位补零命名的 `.py`（如 `tests/16.py`），用 `python3` 能跑通即可；`test.sh` 会自动纳入对比。

## 作者

- 项目创建于 WorkBuddy 工作空间，用于演示 Go 实现解释器。
- 13-15 是使用 codebuddy 生成的测试用例和代码，用于验证解释器的基本功能。
- 16-25 是使用 zcode 生成的测试用例和代码，用于验证解释器的高级功能。跑了一个小时，增加了约 3000 行代码，消耗了 ZLM-3.5 flash模型 5000w tokens。
