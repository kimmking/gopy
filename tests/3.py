# tests/3.py - 四类测试：代数运算、逻辑运算、条件判断、字符串操作

# 1. 常见代数运算
a = 12
b = 7
c = a + b * 3 - 4 / 2
print("代数运算结果:", c)

x = 15
y = 20
z = x * y + 10
print("乘法加法:", z)

# 2. 逻辑运算
p = True
q = False
r = p and q
s = p or q
t = not p
print("逻辑 and:", r)
print("逻辑 or:", s)
print("逻辑 not:", t)

# 3. 条件判断
if x > y:
    print("x 大于 y")
elif x == y:
    print("x 等于 y")
else:
    print("x 小于 y")

# 4. 字符串操作
name = "  KimmKing  "
clean = name.strip()
upper = name.upper()
lower = name.lower()
print("原字符串:", name)
print("strip 后:", clean)
print("大写:", upper)
print("小写:", lower)

text = "hello,world,python"
parts = text.split(",")
print("split 结果:", parts)
joined = "-".join(parts)
print("join 结果:", joined)

msg = f"a={a}, b={b}, sum={a+b}"
print(msg)
