# tests/6.py - 字典方法、负数索引、切片步长、list.pop()/sort()

# 1. 字典方法
d = {"a": 1, "b": 2, "c": 3}
print("keys:", list(d.keys()))
print("values:", list(d.values()))
print("items:", list(d.items()))

# 2. 负数索引
nums = [10, 20, 30, 40]
print("最后一个:", nums[-1])
print("倒数第二:", nums[-2])
text = "hello"
print("字符串最后一个:", text[-1])

# 3. 切片步长
letters = ["a", "b", "c", "d", "e", "f"]
print("步长2:", letters[::2])
print("区间步长:", letters[1:5:2])
text2 = "abcdef"
print("字符串步长2:", text2[::2])

# 4. list.pop() / sort()
lst = [3, 1, 2]
lst.sort()
print("排序后:", lst)
p = lst.pop()
print("pop 出:", p)
print("pop 后:", lst)
