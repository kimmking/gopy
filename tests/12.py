# tests/12.py - 综合：条件表达式、global、嵌套结构、三引号字符串、del

# 1. while True + break
while True:
    break
print("ok")

# 2. 分支中定义函数
flag = True
if flag:
    def f():
        return 1
else:
    def f():
        return 2
print(f())

# 3. 排序与自定义键
print(sorted([3, 1, 2], reverse=True))
print(sorted(["b", "A", "a"], key=lambda s: s.lower()))

# 4. 嵌套容器
data = {"a": [1, 2, {"b": 3}]}
print(data)
print(data["a"][2]["b"])

# 5. 三引号字符串
s = """line1
line2"""
print(s)

# 6. del 删除元素
lst = [1, 2, 3, 4]
del lst[1]
print(lst)
d = {"x": 1, "y": 2}
del d["x"]
print(d)

# 7. global 声明真正写入全局作用域
counter = 0

def bump():
    global counter
    counter = counter + 1

bump()
bump()
print(counter)

# 8. 元组与相等性
t = (1,)
print(t, len(t))
print((1, 2) == (1, 2), [1, 2] == [1, 2], {"a": 1} == {"a": 1})

# 9. 布尔参与算术、数值归一化比较
print(True + 1, False * 3)
print(1 == 1.0, None is None)

# 10. 三元条件表达式（含嵌套与在下标中使用）
print("abc" if 1 < 2 else "def")
print("x" if 0 else "y" if 1 else "z")
print([1, 2, 3][1] if len("ab") == 2 else 0)
