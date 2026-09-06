# tests/14.py - 类型转换与数值操作
# 注：python3 已移除 long，这里 long = int 以兼容基准；
# gopy 内建提供 long（等价于 int，支持 base）。

long = int

print(int(3.7), int(-3.9), int(True), int(False))
print(int("10", 2), int("10", 8), int("10", 16))
print(int("  42  "))
print(long(10), long("ff", 16))
print(float(3), float("3.5"), float("-2.5"))
print(str(123), str(True), str([1, 2, 3]), str((1, 2)))
print(repr("hi"), repr(123), repr([1, 2]))
print(tuple([1, 2, 3]), tuple("abc"))
print(list((1, 2, 3)), list("abc"))
print(chr(65), chr(0x4e2d))
print(ord('A'), ord('中'))
print(hex(255), oct(255), bin(255))
print(int(255), int(255.0))
