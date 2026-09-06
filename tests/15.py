# tests/15.py - 字符串增强与 printf 风格格式化（%s %d %x %f 等）

# ---- printf 风格 % 格式化 ----
print("%s %d %x %f" % ("hi", 255, 255, 3.14159))
print("%5d|%-5d|%05d" % (7, 7, 7))
print("%+d % d" % (5, 5))
print("%.2f" % 3.14159)
print("%10s" % "x")
print("%% %d" % 10)
print("%c" % 65)
print("%r" % "abc")
print("%(name)s=%(val)d" % {"name": "cnt", "val": 3})
print("%#x %#o" % (255, 64))

# ---- 新增字符串方法 ----
s = "hello world"
print(s.partition("o"), s.rpartition("o"))
print(s.removeprefix("hello "), s.removesuffix(" world"))
print("hello world".istitle(), "Hello World".istitle(), "Hello world".istitle())
print("123".isnumeric(), "123".isdecimal(), "12a".isnumeric())
print("AbC".casefold())
print("a\tb".expandtabs(4))

# ---- str.format ----
print("{} {} {}".format(1, "two", 3.5))
print("{0} {1} {0}".format("a", "b"))
print("{name}={score}".format(name="tom", score=90))
print("{:.2f} {:>5} {:05d} {:x}".format(3.14159, "hi", 7, 255))
print("{:*^10}".format("hi"))
print("{{literal}} {x}".format(x=1))
