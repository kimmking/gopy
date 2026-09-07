# Round 7: functools 与 itertools
from functools import reduce, partial, lru_cache
from itertools import (count, cycle, repeat, chain, islice, accumulate,
                       product, permutations, combinations, takewhile,
                       dropwhile, starmap)

# ---- functools.reduce ----
print(reduce(lambda a, b: a + b, [1, 2, 3, 4]))
print(reduce(lambda a, b: a + b, [1, 2, 3, 4], 10))
print(reduce(lambda a, b: a * b, range(1, 6)))

# ---- functools.partial ----
def power(base, exp):
    return base ** exp

square = partial(power, exp=2)
print(square(5))
cube = partial(power, 3)
print(cube(4))
print(list(map(partial(power, exp=2), [1, 2, 3])))
print(square(2, exp=5))

# ---- functools.lru_cache ----
calls = []

@lru_cache(maxsize=128)
def fib(n):
    calls.append(n)
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)

print(fib(10))
print(len(calls), calls[0], calls[-1])
print(fib.cache_info())
print(fib(10))
print(fib.cache_info())

# ---- itertools.count / islice ----
c = count(10, 5)
print(next(c), next(c), next(c))
print(list(islice(count(0, 2), 5)))
print(list(islice(count(0), 3, 8)))

# ---- cycle / repeat / chain ----
print(list(islice(cycle([1, 2, 3]), 7)))
print(list(repeat("x", 3)))
print(list(chain([1, 2], (3, 4), "ab")))

# ---- accumulate ----
print(list(accumulate([1, 2, 3, 4])))
print(list(accumulate([1, 2, 3, 4], lambda a, b: a * b)))

# ---- product / permutations / combinations ----
print(list(product([1, 2], "ab")))
print(list(permutations([1, 2, 3], 2)))
print(list(combinations([1, 2, 3], 2)))

# ---- takewhile / dropwhile / starmap ----
print(list(takewhile(lambda x: x < 4, [1, 2, 5, 1])))
print(list(dropwhile(lambda x: x < 4, [1, 2, 5, 1])))
print(list(starmap(lambda a, b: a + b, [(1, 2), (3, 4)])))
