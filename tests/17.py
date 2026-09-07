# Round 2: 生成器与 yield、生成器表达式、next() 默认值
# ---- 基本生成器 ----
def gen():
    yield 1
    yield 2
    yield 3

g = gen()
print(next(g), next(g))
print(next(g))
try:
    next(g)
except StopIteration:
    print("stop")

g2 = gen()
print(list(g2))
print(list(g2))  # 耗尽后再迭代为空
print(sum(gen()))
print(tuple(gen()))

# ---- 循环与状态 ----
def counter(n):
    i = 0
    while i < n:
        yield i
        i += 1

print(list(counter(5)))
print([x * x for x in counter(4)])

def fib_gen(limit):
    a, b = 0, 1
    for _ in range(limit):
        yield a
        a, b = b, a + b

print(list(fib_gen(10)))

# ---- next 的默认值 ----
print(next(counter(2)), next(counter(2)), next(counter(2), "done"))

# ---- 生成器表达式 ----
sq = (x * x for x in range(6))
print(next(sq))
print(list(sq))
print(sum(x for x in range(1, 11)))
print(list(x for x in range(10) if x % 3 == 0))
pairs = ((a, b) for a in range(3) for b in range(2))
print(list(pairs))

# ---- 生成器管道 ----
def double_gen(items):
    for x in items:
        yield x * 2

def pipeline(items):
    for y in double_gen(items):
        yield y + 1

print(list(pipeline(range(4))))

def add_prefix(prefix, items):
    for it in items:
        yield prefix + str(it)

print(list(add_prefix("n=", [1, 2])))

# ---- 惰性求值验证 ----
touched = []
def trace_gen(n):
    for i in range(n):
        touched.append(i)
        yield i

tg = trace_gen(100)
first = next(tg)
print(first, touched)
rest = list(tg)
print(rest, touched)

# ---- 生成器返回值即生成器对象，可多次引用 ----
def once():
    yield "only"

lazy = once()
print(next(lazy))

# ---- for 直接遍历生成器 ----
total = 0
for v in counter(4):
    total += v
print(total)
