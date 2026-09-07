# Round 4: 运算符重载与描述符风格特性
class Vec:
    def __init__(self, x, y):
        self.x = x
        self.y = y

    def __repr__(self):
        return "Vec(%r, %r)" % (self.x, self.y)

    def __add__(self, other):
        return Vec(self.x + other.x, self.y + other.y)

    def __sub__(self, other):
        return Vec(self.x - other.x, self.y - other.y)

    def __mul__(self, k):
        return Vec(self.x * k, self.y * k)

    def __rmul__(self, k):
        return self * k

    def __eq__(self, other):
        return isinstance(other, Vec) and self.x == other.x and self.y == other.y

    def __lt__(self, other):
        return self.x + self.y < other.x + other.y

    def __len__(self):
        return 2

    def __getitem__(self, i):
        if i == 0:
            return self.x
        if i == 1:
            return self.y
        raise IndexError("vec index out of range")

    def __setitem__(self, i, v):
        if i == 0:
            self.x = v
        else:
            self.y = v

    def __contains__(self, v):
        return v == self.x or v == self.y

    def __call__(self, scale):
        return Vec(self.x * scale, self.y * scale)


a = Vec(1, 2)
b = Vec(3, 4)
print(a + b)
print(a - b)
print(a * 3)
print(2 * a)
print(a == Vec(1, 2), a == b, a != b)
print(a < b, b < a, a < Vec(3, 0))
print(len(a))
print(a[0], a[1])
try:
    a[5]
except IndexError as e:
    print("idx", e)
a[0] = 10
print(a)
print(3 in a, 9 in a)
print(a(10))

# ---- __str__ / __repr__ 分工 ----
class Point:
    def __init__(self, x):
        self.x = x

    def __repr__(self):
        return "Point(" + str(self.x) + ")"

    def __str__(self):
        return "@" + str(self.x)


p = Point(5)
print(p)
print(repr(p))
print([p])
print(str(p))

# 只有 __repr__ 时 str 回退到 __repr__
class OnlyRepr:
    def __repr__(self):
        return "<OnlyRepr>"

print(OnlyRepr())
print(repr(OnlyRepr()))

# ---- __bool__ ----
class Box:
    def __init__(self, filled):
        self.filled = filled

    def __bool__(self):
        return self.filled


print(bool(Box(True)), bool(Box(False)))
if Box(True):
    print("truthy")

# ---- @property ----
class Circle:
    def __init__(self, r):
        self.r = r

    @property
    def area(self):
        return 3 * self.r * self.r

    @property
    def label(self):
        return "circle r=" + str(self.r)


c = Circle(2)
print(c.area, c.label)

# ---- @staticmethod / @classmethod ----
class Registry:
    count = 0

    @staticmethod
    def describe():
        return "registry"

    @classmethod
    def bump(cls):
        cls.count += 1
        return cls.count


print(Registry.describe())
print(Registry.bump())
inst = Registry()
print(inst.bump())
print(inst.describe())
print(Registry.count, inst.count)
