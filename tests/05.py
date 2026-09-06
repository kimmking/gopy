# tests/5.py - break/continue、字典、切片、for...in list

# 1. for ... in list
nums = [1, 2, 3, 4, 5]
print("遍历列表:")
for n in nums:
    print("n =", n)

# 2. break
print("break 示例:")
for n in nums:
    if n == 3:
        break
    print("break n =", n)

# 3. continue
print("continue 示例:")
for n in nums:
    if n == 2:
        continue
    print("cont n =", n)

# 4. while + break/continue
i = 0
print("while 示例:")
while i < 5:
    i = i + 1
    if i == 2:
        continue
    if i == 4:
        break
    print("while i =", i)

# 5. 字典
d = {"name": "KimmKing", "age": 18}
print("字典:", d)
print("name:", d["name"])
d["city"] = "Shanghai"
print("新增后:", d)
print("字典长度:", len(d))

# 6. 切片
letters = ["a", "b", "c", "d", "e"]
print("切片 [1:3]:", letters[1:3])
print("切片 [:2]:", letters[:2])
print("切片 [3:]:", letters[3:])
text = "hello world"
print("字符串切片 [0:5]:", text[0:5])

# 7. 遍历字符串列表
print("遍历字符列表:")
for ch in ["x", "y", "z"]:
    print("ch =", ch)
