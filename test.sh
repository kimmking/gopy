#!/usr/bin/env bash
# 以 python3 的输出为基准，逐个比对 gopy 的执行结果（TDD 回归）
set -u
cd "$(dirname "$0")" || exit 1

go build -o gopy . || exit 1

fail=0
for f in tests/*.py example.py; do
  if diff <(python3 "$f" 2>/dev/null) <(./gopy "$f" 2>&1) > /dev/null; then
    echo "$f  ✅"
  else
    echo "$f  ❌"
    diff <(python3 "$f" 2>/dev/null) <(./gopy "$f" 2>&1)
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "全部用例与 python3 输出一致"
fi
exit "$fail"
