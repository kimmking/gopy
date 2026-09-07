# Round 1: nonlocal、装饰器、调用处 *args/**kwargs 展开、函数 __name__
# ---- nonlocal ----
def make_counter():
    count = 0
    def inc():
        nonlocal count
        count += 1
        return count
    return inc

c = make_counter()
print(c(), c(), c())

d = make_counter()
print(d(), c())

def outer():
    x = 10
    def middle():
        def inner():
            nonlocal x
            x = x + 5
        inner()
    middle()
    return x

print(outer())

def make_acc():
    total = 0
    def add(v):
        nonlocal total
        total += v
        return total
    def reset():
        nonlocal total
        total = 0
    return add, reset

add, reset = make_acc()
print(add(3), add(4))
reset()
print(add(1))

# ---- 调用处 *args / **kwargs ----
def add3(a, b, c):
    return a + b + c

nums = [1, 2, 3]
print(add3(*nums))
print(add3(1, *nums[1:]))
kw = {"b": 20, "c": 30}
print(add3(10, **kw))

def show(*args, **kwargs):
    return (args, kwargs)

t = (1, 2)
k = {"x": 9}
print(show(*t, **k))
print(show(0, *t, y=8))

# ---- 装饰器 ----
def shout(fn):
    def wrapper(*args, **kwargs):
        return fn(*args, **kwargs).upper()
    return wrapper

@shout
def greet(name):
    return "hello " + name

print(greet("bob"))

# 装饰器堆叠：靠近 def 的先应用
def stars(fn):
    def w(*args, **kwargs):
        return "*** " + fn(*args, **kwargs) + " ***"
    return w

@stars
@shout
def msg():
    return "hi there"

print(msg())

# 带参数的装饰器（装饰器工厂）
def repeat(n):
    def deco(fn):
        def w(*args, **kwargs):
            return fn(*args, **kwargs) * n
        return w
    return deco

@repeat(3)
def tail(s):
    return s

print(tail("ab"))

# 记录调用的装饰器 + fn.__name__
calls = []
def trace(fn):
    def w(*args, **kwargs):
        calls.append(fn.__name__)
        return fn(*args, **kwargs)
    return w

@trace
def f1(x):
    return x * 2

@trace
def f2():
    return "z"

print(f1(21), f2(), calls)
print(greet.__name__)

# 闭包计数装饰器
def counted(fn):
    n = 0
    def w(*args, **kwargs):
        nonlocal n
        n += 1
        return (fn(*args, **kwargs), n)
    return w

@counted
def square(x):
    return x * x

print(square(3))
print(square(4))

# 装饰器返回原函数（透传）
def passthrough(fn):
    return fn

@passthrough
def direct():
    return "direct"

print(direct())
