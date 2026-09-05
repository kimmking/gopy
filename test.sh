go build -o gopy .
for f in tests/*.py; do
  diff <(python3 $f) <(./gopy $f) >/dev/null && echo "$f  ✅ " || echo "$f  ❌ "
done

