# tests/10.py - 类与异常：继承、__str__、类属性、raise / except as / finally

# 1. 继承与类属性
class Animal:
    kind = "animal"

    def __init__(self, name):
        self.name = name

    def speak(self):
        return "..."

class Dog(Animal):
    def speak(self):
        return "woof " + self.name

    def __str__(self):
        return "<Dog " + self.name + ">"

d = Dog("Rex")
print(d.speak())
print(d)
print(isinstance(d, Animal), isinstance(d, Dog))
print(d.kind, d.name)

# 2. 方法之间共享 self 状态
class Counter:
    def __init__(self):
        self.value = 0

    def add(self, n):
        self.value = self.value + n
        return self.value

    def double(self):
        return self.value * 2

c = Counter()
c.add(7)
print(c.value, c.double())

# 3. raise + except ... as + finally
try:
    raise ValueError("boom")
except ValueError as e:
    print("caught", e)
finally:
    print("finally")

# 4. 按异常类型分派
try:
    x = [1][5]
except IndexError:
    print("index error")
except Exception:
    print("other")

try:
    d = {}
    print(d["missing"])
except KeyError as e:
    print("key error", e)

# 5. try / else
try:
    ok = 1
except Exception:
    print("不会执行")
else:
    print("try else")

# 6. assert
assert 1 + 1 == 2
print("assert passed")

# 7. 未捕获的异常由解释器兜底（此处用 except 包住以免中断）
try:
    y = 1 / 0
except ZeroDivisionError as e:
    print("zero division")
