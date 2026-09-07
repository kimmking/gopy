# Round 10: json 模块基础 —— dumps / loads
import json

# ---- dumps 基本类型 ----
print(json.dumps({"name": "Alice", "age": 30}))
print(json.dumps([1, "two", None, True]))
print(json.dumps({"b": 2, "a": 1}, sort_keys=True))
print(json.dumps({"a": 1, "b": 2}, separators=(",", ":")))
print(json.dumps([1, 2], separators=(", ", ": ")))

# ---- 浮点与布尔 ----
print(json.dumps(3.5), json.dumps(2), json.dumps(True), json.dumps(None))
print(json.dumps([98.5, 87, False]))

# ---- 字符串转义与 ensure_ascii ----
print(json.dumps("héllo"))
print(json.dumps("héllo", ensure_ascii=False))
print(json.dumps('say "hi"\n\ttab'))
print(json.dumps({"k\n": "v\""}))

# ---- indent ----
print(json.dumps({"a": [1, 2], "b": {"c": 3}}, indent=2))
print(json.dumps({}, indent=2))
print(json.dumps([], indent=2))

# ---- sort_keys 与嵌套 ----
print(json.dumps({"outer": {"z": 1, "a": 2}, "mid": [3, 1]}, sort_keys=True))
print(json.dumps({1: "int-key"}))

# ---- loads ----
print(json.loads('{"x": [1, 2.5, "s", true, false, null], "y": {"z": -7}}'))
print(json.loads('[1, 2, 3]'))
print(json.loads('"text"'))
print(json.loads('true'), json.loads('false'), json.loads('null'))
print(json.loads('42'), json.loads('-17'), json.loads('3.14'), json.loads('-2.5e2'))
print(json.loads('{"nested": {"deep": [1, {"a": null}]}}'))
print(json.loads('{"multi\\nline": "tab\\there"}'))

# ---- round trip ----
obj = {"k": [1, {"m": "v"}], "n": None, "f": 2.25}
back = json.loads(json.dumps(obj))
print(back == obj)
print(json.dumps(json.loads(json.dumps(obj))))
