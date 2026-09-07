# Round 8: 异常增强 —— 自定义异常类、args、raise from、except 元组
class AppError(Exception):
    pass


class ValidationError(AppError):
    pass


def validate(n):
    if n < 0:
        raise ValidationError("negative: " + str(n))
    if n > 100:
        raise AppError("too large")
    return n * 2


print(validate(5))
try:
    validate(-1)
except ValidationError as e:
    print("VE:", e, e.args)
try:
    validate(-1)
except AppError as e:
    print("AE catches VE:", str(e))
try:
    validate(-1)
except Exception as e:
    print("Exc catches:", type(e).__name__)
try:
    validate(200)
except ValidationError:
    print("nope")
except AppError as e:
    print("AE:", e)
print(isinstance(AppError("x"), Exception), issubclass(ValidationError, AppError))
print(issubclass(AppError, Exception), issubclass(ValueError, Exception))

# ---- except 元组 ----
try:
    raise KeyError("k")
except (ValueError, KeyError) as e:
    print("tuple caught:", e)

# ---- raise ... from ... ----
class DataError(Exception):
    pass


try:
    try:
        raise ValueError("inner")
    except ValueError as e:
        raise DataError("outer") from e
except DataError as e2:
    print("cause:", e2.__cause__)

# ---- 异常实例即对象：args / str / repr ----
err = ValueError("plain", 42)
print(err.args)
print(str(err))
print(repr(err))
simple = TypeError("only one")
print(simple.args, str(simple))
empty = ValueError()
print(empty.args, repr(empty))

# ---- finally 中的 return 覆盖 ----
def override():
    try:
        return "try"
    finally:
        return "finally"

print(override())

# ---- 捕获后包装再抛 ----
def level2():
    raise ValueError("deep")

def level1():
    try:
        level2()
    except ValueError as e:
        raise RuntimeError("wrapped: " + str(e))

try:
    level1()
except RuntimeError as e:
    print(e)

# ---- else 与多级匹配 ----
def classify(x):
    try:
        return 10 / x
    except ZeroDivisionError:
        return "zero"
    except (TypeError, ValueError):
        return "bad"

print(classify(2), classify(0), classify("x"))
