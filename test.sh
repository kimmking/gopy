go build -o gopy .
for f in tests/1.py tests/2.py tests/3.py tests/4.py tests/5.py example.py; do
  diff <(python3 $f) <(./gopy $f) >/dev/null && echo "$f  ✅ " || echo "$f  ❌ "
done

