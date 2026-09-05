# tests/13.py - math 与 os 模块增强

import math
import os
from math import isqrt, comb

# 三角函数 / 反三角函数（取 0 或 nice point，避免末位误差）
print(math.sin(0.0), math.cos(0.0), math.tan(0.0))
print(math.asin(0.0), math.acos(1.0), math.atan(0.0))

# 双曲函数
print(math.sinh(0.0), math.cosh(0.0), math.tanh(0.0))
print(math.asinh(0.0), math.acosh(1.0), math.atanh(0.0))

# 对数 / 指数变体
print(math.log1p(0.0), math.expm1(0.0))

# 误差函数与伽马函数
print(math.erf(0.0), math.erfc(0.0), math.gamma(5.0))

# 整数平方根
print(isqrt(10), isqrt(16), isqrt(0))

# 欧氏距离
print(math.dist((0, 0), (3, 4)))

# 组合、排列与最小公倍数
print(comb(5, 2), math.perm(5, 2))
print(math.comb(10, 0), math.perm(10, 10))
print(math.lcm(4, 6), math.lcm(0, 5), math.lcm(-4, 6))

# 乘积
print(math.prod([1, 2, 3, 4]))
print(math.prod([2, 3], start=10))

# 数值判定
print(math.isfinite(1.0), math.isfinite(float("inf")))
print(math.isinf(float("inf")), math.isinf(1.0))
print(math.isnan(float("nan")), math.isnan(1.0))

# 浮点余数与相邻浮点
print(math.remainder(5.0, 2.0))
print(round(math.nextafter(1.0, 2.0), 20))

# 尾数与指数分解
print(math.frexp(8.0))

# os 基本信息
print(os.sep, os.name)
print(os.getcwd() == os.path.abspath("."))

# os 环境变量
print("HOME" in os.environ and "PATH" in os.environ)
print(os.getenv("HOME") != "", os.getenv("NO_SUCH_VAR_XYZ"))

# os.path 子模块
print(os.path.basename("/a/b/c.txt"))
print(os.path.dirname("/a/b/c.txt"))
print(os.path.join("a", "b", "c.txt"))
print(os.path.splitext("/a/b/c.txt"))
print(os.path.split("/a/b/c.txt"))
print(os.path.normpath("/a/./b/../c"))
print(os.path.isabs("/a/b"), os.path.isabs("a/b"))
print(os.path.relpath("/a/b/c", "/a"))
print(os.path.commonpath(["/a/b/c", "/a/b/d", "/a/b"]))
print(os.path.exists("tests/13.py"), os.path.isfile("tests/13.py"), os.path.isdir("tests/13.py"))
print(os.path.exists("no_such_file_xyz"), os.path.isfile("no_such_file_xyz"))
print(os.path.getsize("tests/13.py"))
print(os.path.abspath("tests/13.py") == os.path.realpath("tests/13.py"))
