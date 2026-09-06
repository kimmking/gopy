# ===== 类与对象 =====
class Dog:
    def __init__(self, name):
        self.name = name
    def speak(self):
        return "汪 " + self.name

d = Dog("旺财")
print(d.name)
print(d.speak())

# ===== 异常处理 =====
try:
    x = 1 / 0
    print("不会执行")
except:
    print("捕获到错误")

# ===== import 模块 =====
import math
print(math.floor(3.7))
print(math.sqrt(16))
