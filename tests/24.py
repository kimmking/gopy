# Round 9: 字符串与内置函数第二轮
# ---- str.translate / str.maketrans ----
table = str.maketrans("abc", "xyz")
print("alphabet".translate(table))
table2 = str.maketrans("abc", "xyz", "l")
print("alphabet".translate(table2))

# ---- startswith / endswith 元组参数 ----
names = ["report.pdf", "photo.jpg", "notes.txt"]
print([n for n in names if n.endswith((".jpg", ".png"))])
print([n for n in names if n.startswith(("rep", "not"))])

# ---- center / ljust / rjust 填充字符 ----
print("ab".center(6, "*"))
print("ab".ljust(5, "."), "ab".rjust(5, "."))

# ---- dict.fromkeys ----
print(dict.fromkeys(["a", "b"], 0))
print(dict.fromkeys("abc"))

# ---- dict | 合并与 |= ----
d1 = {"x": 1, "y": 2}
d2 = {"y": 20, "z": 30}
merged = d1 | d2
print(merged)
d1 |= {"w": 9}
print(d1)

# ---- 集合运算符 ----
s1 = {1, 2, 3}
s2 = {3, 4}
print(s1 | s2)
print(s1 & s2)
print(s1 - s2)
print(s1 ^ s2)

# ---- min / max 的 default 与 key ----
print(min([], default="empty"))
print(max([], default=0))
words = ["kiwi", "fig", "banana"]
print(min(words, key=len), max(words, key=len))

# ---- format / callable 内置函数 ----
print(format(3.14159, ".2f"))
print(format(255, "04x"))
print(format("hi", ">5"))
print(callable(print), callable(3))
def fn():
    pass
print(callable(fn))

# ---- sum 的 start 关键字 ----
print(sum([1, 2, 3], start=10))
