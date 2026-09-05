# tests/11.py - 赋值、推导式、f-string 格式、集合与模块

# 1. 增量赋值、链式赋值、元组解包
x = 1
x += 2
x *= 3
print(x)

a = b = 5
print(a, b)

p, q = 1, 2
print(p, q)

(m, n) = (10, 20)
print(m, n)

nested = [(1, 2), (3, 4)]
for i, j in nested:
    print(i, j)

# 2. 括号内的隐式续行与反斜杠续行
z = 1 + \
    2
nums = [
    1,
    2,
]
print(z, nums)

# 3. 行尾注释不应影响表达式
y = 5  # 行尾注释
print(y)

# 4. for / while 的 else 子句
for i in range(3):
    pass
else:
    print("for else")

for i in range(3):
    if i == 1:
        break
else:
    print("不会执行")

# 5. 推导式
print([x * x for x in range(5)])
print([x for x in range(10) if x % 3 == 0])
print({x: x * x for x in range(4)})
print([(x, y) for x in range(2) for y in range(2)])

# 6. f-string 格式说明符
pi = 3.14159
print(f"pi={pi:.2f}")
print(f"[{pi:>10.2f}]")
print(f"n={42:05d}")
print(f"hex={255:#x}")
print(f"{'abc':^9}")
print(f"{0.25:.1%}")
print(f"r={[1, 2]!r}")

# 7. 集合
s = {3, 1, 2, 1}
print(sorted(s), len(s), 2 in s)
s.add(9)
print(sorted(s))

# 8. 模块
import math
print(math.degrees(math.pi), math.radians(180.0))
print(math.gcd(12, 18), math.factorial(5))
print(math.pi, math.e)

from math import pi, sqrt
print(round(pi, 3), sqrt(9.0))

import os
print(os.sep, os.name)
print(type(os.path.basename("/a/b/c.txt")))
print(os.path.join("a", "b", "c.txt"))
