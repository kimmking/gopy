# Round 6: collections 模块 —— Counter / defaultdict / OrderedDict / deque
from collections import Counter, defaultdict, OrderedDict, deque

# ---- Counter ----
c = Counter("abracadabra")
print(c)
print(c["a"], c["z"])
c.update("abc")
print(c)
print(c.most_common(2))
print(sorted(c.elements()))
print(len(c), "a" in c, "z" in c)
print(sorted(c))
print(Counter([1, 2, 2, 3, 3, 3]))
print(Counter("aabbcc").most_common())

# ---- defaultdict ----
groups = defaultdict(list)
words = ["apple", "avocado", "banana"]
for w in words:
    groups[w[0]].append(w)
print(groups)
print(groups["c"])
print(len(groups))

counts = defaultdict(int)
for ch in "hello":
    counts[ch] += 1
print(counts)

pairs = defaultdict(str)
pairs["k"] = "v"
print(pairs)

# ---- OrderedDict ----
od = OrderedDict()
od["x"] = 1
od["y"] = 2
od["z"] = 3
print(od)
od.move_to_end("x")
print(od)
print(od.pop("y"), od)
print(od.popitem())
print(od)

# ---- deque ----
dq = deque([1, 2, 3])
dq.append(4)
dq.appendleft(0)
print(dq)
print(dq.pop(), dq.popleft(), dq)
print(len(dq))
dq.extend([7, 8])
dq.extendleft([9])
print(dq)
dq.rotate(2)
print(dq)
dq.rotate(-1)
print(dq)
print(dq[0], dq[-1], list(dq))
dq.remove(3)
print(dq)
dq.clear()
print(dq, len(dq))
