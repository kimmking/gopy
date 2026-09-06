# tests/8.py - 运算符与切片：// % ** 位运算、not 优先级、链式比较、负步长切片

# 1. 算术运算符
print(7 // 2, -7 // 2, 7 % 3, -7 % 3, 2 ** 10, 2 ** 0.5)

# 2. 位运算
print(5 & 3, 5 | 3, 5 ^ 3, 1 << 4, 256 >> 4)

# 3. not 的优先级低于比较运算：等价于 not (True == False)
print(not True == False)

# 4. 链式比较
print(1 < 2 < 3, 3 > 2 > 5)

# 5. 切片
a = [1, 2, 3, 4, 5]
print(a[::-1])
print(a[-2:])
print(a[:-3])
print(a[1:4:2])
print("abcdef"[::-2])

# 6. range 支持步长
print(list(range(10, 0, -2)))
print(range(1, 10, 3))
print(len(range(0, 10, 2)))

# 7. 序列重复与其它运算
print([1, 2] * 2)
print("ab" * 3)
print([1] + [2, 3])
print((1, 2) + (3,))
