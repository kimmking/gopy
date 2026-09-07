# Round 3: with 语句与上下文管理器协议 (__enter__/__exit__)
log = []

class CM:
    def __init__(self, name, suppress=False):
        self.name = name
        self.suppress = suppress

    def __enter__(self):
        log.append("enter " + self.name)
        return self.name

    def __exit__(self, exc_type, exc_val, tb):
        if exc_type is None:
            log.append("exit " + self.name)
        else:
            log.append("exit " + self.name + " with " + exc_type.__name__)
        return self.suppress

# ---- 正常路径：进入/退出顺序 ----
with CM("a") as x:
    log.append("body " + x)

print(log)
log.clear()

# ---- 异常传播（__exit__ 返回 None 不抑制） ----
try:
    with CM("b"):
        raise ValueError("boom")
except ValueError as e:
    log.append("caught " + str(e))

print(log)
log.clear()

# ---- 异常抑制（__exit__ 返回 True） ----
with CM("c", suppress=True):
    raise KeyError("swallowed")

log.append("after suppress")
print(log)
log.clear()

# ---- 嵌套 with ----
with CM("outer") as o:
    with CM("inner") as n:
        log.append(o + "/" + n)

print(log)
log.clear()

# ---- 嵌套 with 异常时由内向外调用 __exit__ ----
try:
    with CM("o2"):
        with CM("i2"):
            raise TypeError("t")
except TypeError:
    log.append("caught t")

print(log)
log.clear()

# ---- 同一 with 多个上下文项 ----
with CM("m1") as a, CM("m2") as b:
    log.append(a + "+" + b)

print(log)
log.clear()

# ---- 文件风格资源管理（模拟） ----
class Resource:
    opened = []
    closed = []

    def __init__(self, rid):
        self.rid = rid

    def __enter__(self):
        Resource.opened.append(self.rid)
        return self

    def read(self):
        return "data-" + str(self.rid)

    def __exit__(self, *args):
        Resource.closed.append(self.rid)
        return False

with Resource(1) as r1, Resource(2) as r2:
    log.append(r1.read() + "|" + r2.read())

print(log)
print(Resource.opened, Resource.closed)
log.clear()

# ---- with 体中的 return/break/continue 也会触发 __exit__ ----
def find_first():
    with CM("fn"):
        for i in range(10):
            if i == 3:
                return i
    return -1

print(find_first(), log)
