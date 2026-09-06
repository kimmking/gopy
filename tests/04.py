# tests/4.py - 控制流与函数：for/range、while、def、列表操作

# 1. for 循环 + range
print("for 循环:")
for i in range(3):
    print("i =", i)

total = 0
for n in range(1, 5):
    total = total + n
print("1到4求和:", total)

# 2. while 循环
count = 3
print("while 循环:")
while count > 0:
    print("count =", count)
    count = count - 1

# 3. 函数定义与调用
def add(a, b):
    return a + b

def greet(name):
    msg = "Hello, " + name
    return msg

print("add(3, 4) =", add(3, 4))
print(greet("KimmKing"))

# 4. 列表操作
nums = [10, 20, 30]
print("列表:", nums)
print("长度:", len(nums))
print("第一个:", nums[0])
nums.append(40)
print("追加后:", nums)

# 5. 综合：函数内使用循环
def sum_to(n):
    s = 0
    i = 1
    while i <= n:
        s = s + i
        i = i + 1
    return s

print("sum_to(5) =", sum_to(5))
