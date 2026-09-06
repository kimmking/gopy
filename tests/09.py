# tests/9.py - 函数与作用域：递归、闭包、默认参数、可变参数、lambda

# 1. 递归（返回值不再丢失）
def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)

print(fib(15))

# 2. 闭包
def make_adder(n):
    def add(x):
        return x + n
    return add

add5 = make_adder(5)
print(add5(3))

# 3. 默认参数与关键字参数
def greet(name, prefix="Hello"):
    return prefix + ", " + name

print(greet("KimmKing"))
print(greet("KimmKing", prefix="Hi"))

# 4. 可变参数
def total(base, *args, **kwargs):
    s = base
    for a in args:
        s = s + a
    return s

print(total(1, 2, 3))
print(total(10))

# 5. 嵌套函数共享外层变量
def counter():
    count = 0
    def inc():
        return count + 1
    return inc()

print(counter())

# 6. lambda 与高阶函数
words = ["banana", "apple", "cherry"]
print(sorted(words))
print(sorted(words, key=len))
print(list(map(lambda w: w.upper(), words)))
print(list(filter(lambda w: "a" in w, words)))

# 7. 推导式拥有独立作用域，不泄漏循环变量
n = 99
print([n for n in range(3)])
print(n)

# 8. 其它内置函数
print(max([3, 1, 2]), min(3, 1, 2), sum([1, 2, 3]))
print(abs(-7), divmod(17, 5), round(3.14159, 3))
print(hex(255), oct(8), bin(5), chr(65), ord("A"))
print(any([0, 1]), all([1, 2]), any([]), all([]))
print(list(enumerate(["a", "b"])))
print(list(zip([1, 2], ["x", "y"])))
print(list(reversed([1, 2, 3])))
