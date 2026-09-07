# Round 5: 多继承、C3 方法解析顺序、super()、issubclass
class A:
    def who(self):
        return "A"

    def hello(self):
        return "hello from A"


class B(A):
    def who(self):
        return "B"


class C(A):
    def who(self):
        return "C"

    def hello(self):
        return "hello from C"


class D(B, C):
    def who(self):
        return "D->" + super().who()


d = D()
print(d.who())
print(d.hello())

print(isinstance(d, A), isinstance(d, B), isinstance(d, C))
print(issubclass(D, A), issubclass(B, C), issubclass(D, D), issubclass(B, D))

# ---- super().__init__ 链 ----
class Base:
    def __init__(self, tag):
        self.tag = tag


class Mid1(Base):
    def __init__(self, tag, extra):
        super().__init__(tag)
        self.extra = extra


m = Mid1("t", "e")
print(m.tag, m.extra)

# ---- 菱形继承：协作式 __init__ 只初始化 Root 一次 ----
calls = []


class Root:
    def __init__(self):
        calls.append("root")


class Left(Root):
    def __init__(self):
        calls.append("left")
        super().__init__()


class Right(Root):
    def __init__(self):
        calls.append("right")
        super().__init__()


class Leaf(Left, Right):
    def __init__(self):
        calls.append("leaf")
        super().__init__()


Leaf()
print(calls)

# ---- Mixin ----
class JsonMixin:
    def to_json(self):
        return '{"name": "' + self.name + '"}'


class SaveMixin:
    def save(self):
        return "saved " + self.name


class User(JsonMixin, SaveMixin):
    def __init__(self, name):
        self.name = name


u = User("kim")
print(u.to_json(), u.save())

# ---- super 在类方法中 ----
class P:
    @classmethod
    def kind(cls):
        return "P:" + cls.__name__


class Q(P):
    @classmethod
    def kind(cls):
        return "Q(" + super().kind() + ")"


print(Q.kind())

# ---- 三层继承的属性/方法查找顺序 ----
class G1:
    def f(self):
        return "g1"


class G2(G1):
    def f(self):
        return "g2"

    def g(self):
        return "only-g2"


class G3(G2):
    pass


g = G3()
print(g.f(), g.g())
print(isinstance(g, G1), issubclass(G3, G1))
