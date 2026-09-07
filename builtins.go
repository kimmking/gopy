package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ============ 内建类型方法 ============

type MethodFn func(recv Object, args []Object, kwargs map[string]Object) (Object, error)

func callBuiltinMethod(recv Object, name string, args []Object, kwargs map[string]Object) (Object, error) {
	switch typeName(recv) {
	case "str":
		if fn, ok := strMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
	case "list":
		if fn, ok := listMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
	case "dict":
		if fn, ok := dictMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
	case "tuple":
		if fn, ok := tupleMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
	case "set":
		if fn, ok := setMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
	case "Counter":
		if fn, ok := counterMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
		if fn, ok := dictMethods[name]; ok {
			return fn(recv.(*PyCounter).D, args, kwargs)
		}
	case "defaultdict":
		if fn, ok := dictMethods[name]; ok {
			return fn(recv.(*PyDefaultDict).D, args, kwargs)
		}
	case "OrderedDict":
		if fn, ok := orderedDictMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
		if fn, ok := dictMethods[name]; ok {
			return fn(recv.(*PyOrderedDict).D, args, kwargs)
		}
	case "deque":
		if fn, ok := dequeMethods[name]; ok {
			return fn(recv, args, kwargs)
		}
	}
	return nil, newExc("AttributeError", "'%s' 对象没有属性 '%s'", typeName(recv), name)
}

// ---------- collections 方法 ----------

var counterMethods = map[string]MethodFn{
	"most_common": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		all := recv.(*PyCounter).mostCommon()
		if len(args) == 0 {
			return &List{Items: all}, nil
		}
		n, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "most_common() 的参数必须是整数")
		}
		if n < 0 {
			n = 0
		}
		if n > len(all) {
			n = len(all)
		}
		return &List{Items: all[:n]}, nil
	},
	"elements": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		c := recv.(*PyCounter)
		out := []Object{}
		for _, k := range c.D.Keys {
			n, _ := intVal(c.D.Vals[keyOf(k)])
			for i := 0; i < n; i++ {
				out = append(out, k)
			}
		}
		return &List{Items: out}, nil
	},
	"update": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		c := recv.(*PyCounter)
		for _, a := range args {
			if d, ok := a.(*Dict); ok {
				for _, k := range d.Keys {
					addCount(c.D, k, d.Vals[keyOf(k)])
				}
				continue
			}
			items, err := iterate(a)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				addCount(c.D, it, 1)
			}
		}
		return None, nil
	},
	"subtract": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		c := recv.(*PyCounter)
		for _, a := range args {
			items, err := iterate(a)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				addCount(c.D, it, -1)
			}
		}
		return None, nil
	},
}

// addCount 给计数表中的键加上 delta（保持插入顺序）
func addCount(d *Dict, k Object, delta Object) {
	cur := 0
	if v, ok := d.Get(k); ok {
		if ci, ok2 := intVal(v); ok2 {
			cur = ci
		}
	}
	di, _ := intVal(delta)
	d.Set(k, cur+di)
}

var orderedDictMethods = map[string]MethodFn{
	"move_to_end": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		od := recv.(*PyOrderedDict)
		if len(args) < 1 {
			return nil, argCountErr("move_to_end", len(args), 1)
		}
		key := args[0]
		v, ok := od.D.Get(key)
		if !ok {
			return nil, newExc("KeyError", "%s", Repr(key))
		}
		od.D.Delete(key)
		od.D.Set(key, v)
		return None, nil
	},
	"popitem": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		od := recv.(*PyOrderedDict)
		if od.D.Len() == 0 {
			return nil, newExc("KeyError", "popitem(): dictionary is empty")
		}
		// last=True（默认）弹出最后一个；last=False 弹出第一个
		last := true
		if v, ok := kwargs["last"]; ok {
			last = truthy(v)
		}
		var k Object
		if last {
			k = od.D.Keys[len(od.D.Keys)-1]
		} else {
			k = od.D.Keys[0]
		}
		v, _ := od.D.Get(k)
		od.D.Delete(k)
		return &Tuple{Items: []Object{k, v}}, nil
	},
}

var dequeMethods = map[string]MethodFn{
	"append": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("append", len(args), 1)
		}
		d := recv.(*PyDeque)
		d.Items = append(d.Items, args[0])
		return None, nil
	},
	"appendleft": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("appendleft", len(args), 1)
		}
		d := recv.(*PyDeque)
		d.Items = append([]Object{args[0]}, d.Items...)
		return None, nil
	},
	"pop": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*PyDeque)
		if len(d.Items) == 0 {
			return nil, newExc("IndexError", "pop from an empty deque")
		}
		v := d.Items[len(d.Items)-1]
		d.Items = d.Items[:len(d.Items)-1]
		return v, nil
	},
	"popleft": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*PyDeque)
		if len(d.Items) == 0 {
			return nil, newExc("IndexError", "pop from an empty deque")
		}
		v := d.Items[0]
		d.Items = d.Items[1:]
		return v, nil
	},
	"extend": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("extend", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		d := recv.(*PyDeque)
		d.Items = append(d.Items, items...)
		return None, nil
	},
	"extendleft": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("extendleft", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		d := recv.(*PyDeque)
		// extendleft 按逆序追加到左端
		for i := len(items) - 1; i >= 0; i-- {
			d.Items = append([]Object{items[i]}, d.Items...)
		}
		return None, nil
	},
	"rotate": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("rotate", len(args), 1)
		}
		n, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "rotate() 的参数必须是整数")
		}
		d := recv.(*PyDeque)
		l := len(d.Items)
		if l == 0 {
			return None, nil
		}
		n = ((n % l) + l) % l
		if n == 0 {
			return None, nil
		}
		out := make([]Object, 0, l)
		out = append(out, d.Items[l-n:]...)
		out = append(out, d.Items[:l-n]...)
		d.Items = out
		return None, nil
	},
	"remove": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("remove", len(args), 1)
		}
		d := recv.(*PyDeque)
		for i, it := range d.Items {
			if objectsEqual(it, args[0]) {
				d.Items = append(d.Items[:i], d.Items[i+1:]...)
				return None, nil
			}
		}
		return nil, newExc("ValueError", "deque.remove(x): x not in deque")
	},
	"clear": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		recv.(*PyDeque).Items = nil
		return None, nil
	},
	"reverse": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*PyDeque)
		for i, j := 0, len(d.Items)-1; i < j; i, j = i+1, j-1 {
			d.Items[i], d.Items[j] = d.Items[j], d.Items[i]
		}
		return None, nil
	},
	"copy": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*PyDeque)
		out := make([]Object, len(d.Items))
		copy(out, d.Items)
		return &PyDeque{Items: out}, nil
	},
	"index": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("index", len(args), 1)
		}
		d := recv.(*PyDeque)
		for i, it := range d.Items {
			if objectsEqual(it, args[0]) {
				return i, nil
			}
		}
		return nil, newExc("ValueError", "%s is not in deque", Repr(args[0]))
	},
	"count": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("count", len(args), 1)
		}
		n := 0
		for _, it := range recv.(*PyDeque).Items {
			if objectsEqual(it, args[0]) {
				n++
			}
		}
		return n, nil
	},
}

func kwBool(kwargs map[string]Object, key string) bool {
	if v, ok := kwargs[key]; ok {
		return truthy(v)
	}
	return false
}

func argCountErr(name string, got, want int) error {
	return newExc("TypeError", "%s() 需要 %d 个参数，实际传入 %d 个", name, want, got)
}

// ---------- 字符串方法 ----------

var strMethods = map[string]MethodFn{
	"upper": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		return strings.ToUpper(recv.(string)), nil
	},
	"lower": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		return strings.ToLower(recv.(string)), nil
	},
	"capitalize": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return "", nil
		}
		rs := []rune(s)
		return string(unicode.ToUpper(rs[0])) + strings.ToLower(string(rs[1:])), nil
	},
	"title": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		return titleCase(recv.(string)), nil
	},
	"swapcase": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		rs := []rune(recv.(string))
		for i, r := range rs {
			if unicode.IsUpper(r) {
				rs[i] = unicode.ToLower(r)
			} else if unicode.IsLower(r) {
				rs[i] = unicode.ToUpper(r)
			}
		}
		return string(rs), nil
	},
	"strip": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if len(args) > 0 {
			if cut, ok := args[0].(string); ok {
				return strings.Trim(s, cut), nil
			}
		}
		return strings.TrimSpace(s), nil
	},
	"lstrip": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if len(args) > 0 {
			if cut, ok := args[0].(string); ok {
				return strings.TrimLeft(s, cut), nil
			}
		}
		return strings.TrimLeft(s, " \t\n\r\f\v"), nil
	},
	"rstrip": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if len(args) > 0 {
			if cut, ok := args[0].(string); ok {
				return strings.TrimRight(s, cut), nil
			}
		}
		return strings.TrimRight(s, " \t\n\r\f\v"), nil
	},
	"split": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		var parts []string
		if len(args) == 0 {
			parts = strings.Fields(s)
		} else {
			sep, ok := args[0].(string)
			if !ok {
				return nil, newExc("TypeError", "split() 的分隔符必须是字符串")
			}
			if sep == "" {
				return nil, newExc("ValueError", "empty separator")
			}
			parts = strings.Split(s, sep)
		}
		out := make([]Object, len(parts))
		for i, p := range parts {
			out[i] = p
		}
		return &List{Items: out}, nil
	},
	"rsplit": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		var parts []string
		if len(args) == 0 {
			parts = strings.Fields(s)
		} else {
			sep, ok := args[0].(string)
			if !ok {
				return nil, newExc("TypeError", "rsplit() 的分隔符必须是字符串")
			}
			parts = strings.Split(s, sep)
		}
		out := make([]Object, len(parts))
		for i, p := range parts {
			out[i] = p
		}
		return &List{Items: out}, nil
	},
	"splitlines": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		parts := strings.Split(recv.(string), "\n")
		out := make([]Object, len(parts))
		for i, p := range parts {
			out[i] = p
		}
		return &List{Items: out}, nil
	},
	"join": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("join", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		parts := make([]string, len(items))
		for i, it := range items {
			s, ok := it.(string)
			if !ok {
				return nil, newExc("TypeError", "join() 的元素必须是字符串，实际为 '%s'", typeName(it))
			}
			parts[i] = s
		}
		return strings.Join(parts, recv.(string)), nil
	},
	"startswith": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("startswith", len(args), 1)
		}
		p, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "startswith() 需要字符串参数")
		}
		return strings.HasPrefix(recv.(string), p), nil
	},
	"endswith": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("endswith", len(args), 1)
		}
		p, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "endswith() 需要字符串参数")
		}
		return strings.HasSuffix(recv.(string), p), nil
	},
	"find": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("find", len(args), 1)
		}
		sub, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "find() 需要字符串参数")
		}
		return strings.Index(recv.(string), sub), nil
	},
	"rfind": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("rfind", len(args), 1)
		}
		sub, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "rfind() 需要字符串参数")
		}
		return strings.LastIndex(recv.(string), sub), nil
	},
	"index": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("index", len(args), 1)
		}
		sub, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "index() 需要字符串参数")
		}
		pos := strings.Index(recv.(string), sub)
		if pos < 0 {
			return nil, newExc("ValueError", "substring not found")
		}
		return pos, nil
	},
	"count": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("count", len(args), 1)
		}
		sub, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "count() 需要字符串参数")
		}
		return strings.Count(recv.(string), sub), nil
	},
	"replace": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("replace", len(args), 2)
		}
		oldS, ok1 := args[0].(string)
		newS, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return nil, newExc("TypeError", "replace() 需要字符串参数")
		}
		n := -1
		if len(args) >= 3 {
			if v, ok := intVal(args[2]); ok {
				n = v
			}
		}
		return strings.Replace(recv.(string), oldS, newS, n), nil
	},
	"isdigit": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return false, nil
		}
		for _, r := range s {
			if !unicode.IsDigit(r) {
				return false, nil
			}
		}
		return true, nil
	},
	"isalpha": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return false, nil
		}
		for _, r := range s {
			if !unicode.IsLetter(r) {
				return false, nil
			}
		}
		return true, nil
	},
	"isalnum": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return false, nil
		}
		for _, r := range s {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				return false, nil
			}
		}
		return true, nil
	},
	"isspace": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return false, nil
		}
		for _, r := range s {
			if !unicode.IsSpace(r) {
				return false, nil
			}
		}
		return true, nil
	},
	"isupper": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		has := false
		for _, r := range s {
			if unicode.IsLetter(r) {
				has = true
				if !unicode.IsUpper(r) {
					return false, nil
				}
			}
		}
		return has, nil
	},
	"islower": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		has := false
		for _, r := range s {
			if unicode.IsLetter(r) {
				has = true
				if !unicode.IsLower(r) {
					return false, nil
				}
			}
		}
		return has, nil
	},
	"ljust": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("ljust", len(args), 1)
		}
		w, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "ljust() 需要整数宽度")
		}
		return padString(recv.(string), w, false), nil
	},
	"rjust": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("rjust", len(args), 1)
		}
		w, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "rjust() 需要整数宽度")
		}
		return padString(recv.(string), w, true), nil
	},
	"center": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("center", len(args), 1)
		}
		w, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "center() 需要整数宽度")
		}
		s := recv.(string)
		n := len([]rune(s))
		if w <= n {
			return s, nil
		}
		total := w - n
		left := total / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", total-left), nil
	},
	"zfill": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("zfill", len(args), 1)
		}
		w, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "zfill() 需要整数宽度")
		}
		s := recv.(string)
		neg := strings.HasPrefix(s, "-")
		if neg {
			s = s[1:]
		}
		if w <= len(s) {
			if neg {
				return "-" + s, nil
			}
			return s, nil
		}
		out := strings.Repeat("0", w-len(s)) + s
		if neg {
			return "-" + out, nil
		}
		return out, nil
	},
	"format": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		return formatString(recv.(string), args, kwargs)
	},
	"partition": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("partition", len(args), 1)
		}
		sep, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "partition() 需要字符串分隔符")
		}
		s := recv.(string)
		idx := strings.Index(s, sep)
		if idx < 0 {
			return &Tuple{Items: []Object{s, "", ""}}, nil
		}
		return &Tuple{Items: []Object{s[:idx], sep, s[idx+len(sep):]}}, nil
	},
	"rpartition": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("rpartition", len(args), 1)
		}
		sep, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "rpartition() 需要字符串分隔符")
		}
		s := recv.(string)
		idx := strings.LastIndex(s, sep)
		if idx < 0 {
			return &Tuple{Items: []Object{s, "", ""}}, nil
		}
		return &Tuple{Items: []Object{s[:idx], sep, s[idx+len(sep):]}}, nil
	},
	"istitle": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		title := false
		inWord := false
		for _, r := range s {
			if unicode.IsLetter(r) {
				if !inWord {
					if !unicode.IsUpper(r) {
						return false, nil
					}
					inWord = true
					title = true
				} else if !unicode.IsLower(r) {
					return false, nil
				}
			} else {
				inWord = false
			}
		}
		return title, nil
	},
	"isnumeric": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return false, nil
		}
		for _, r := range s {
			if !unicode.IsDigit(r) {
				return false, nil
			}
		}
		return true, nil
	},
	"isdecimal": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(string)
		if s == "" {
			return false, nil
		}
		for _, r := range s {
			if !unicode.IsDigit(r) {
				return false, nil
			}
		}
		return true, nil
	},
	"casefold": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		return strings.ToLower(recv.(string)), nil
	},
	"removeprefix": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("removeprefix", len(args), 1)
		}
		p, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "removeprefix() 需要字符串参数")
		}
		s := recv.(string)
		if strings.HasPrefix(s, p) {
			return s[len(p):], nil
		}
		return s, nil
	},
	"removesuffix": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("removesuffix", len(args), 1)
		}
		p, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "removesuffix() 需要字符串参数")
		}
		s := recv.(string)
		if strings.HasSuffix(s, p) {
			return s[:len(s)-len(p)], nil
		}
		return s, nil
	},
	"expandtabs": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		tab := 8
		if len(args) >= 1 {
			if t, ok := intVal(args[0]); ok {
				tab = t
			}
		}
		s := recv.(string)
		var sb strings.Builder
		col := 0
		for _, r := range s {
			if r == '\t' {
				n := tab - col%tab
				sb.WriteString(strings.Repeat(" ", n))
				col += n
			} else {
				sb.WriteRune(r)
				if r == '\n' {
					col = 0
				} else {
					col++
				}
			}
		}
		return sb.String(), nil
	},
}

func padString(s string, width int, left bool) string {
	n := len([]rune(s))
	if width <= n {
		return s
	}
	pad := strings.Repeat(" ", width-n)
	if left {
		return pad + s
	}
	return s + pad
}

// titleCase 模拟 Python 的 str.title
func titleCase(s string) string {
	var out strings.Builder
	upper := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			out.WriteRune(r)
			upper = true
		} else if upper {
			out.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			out.WriteRune(unicode.ToLower(r))
		}
	}
	return out.String()
}

// ---------- 列表方法 ----------

var listMethods = map[string]MethodFn{
	"append": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("append", len(args), 1)
		}
		l := recv.(*List)
		l.Items = append(l.Items, args[0])
		return None, nil
	},
	"extend": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("extend", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		l := recv.(*List)
		l.Items = append(l.Items, items...)
		return None, nil
	},
	"insert": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("insert", len(args), 2)
		}
		idx, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "insert() 的下标必须是整数")
		}
		l := recv.(*List)
		n := len(l.Items)
		if idx < 0 {
			idx += n
			if idx < 0 {
				idx = 0
			}
		}
		if idx > n {
			idx = n
		}
		out := make([]Object, 0, n+1)
		out = append(out, l.Items[:idx]...)
		out = append(out, args[1])
		out = append(out, l.Items[idx:]...)
		l.Items = out
		return None, nil
	},
	"remove": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("remove", len(args), 1)
		}
		l := recv.(*List)
		for i, it := range l.Items {
			if objectsEqual(it, args[0]) {
				l.Items = append(l.Items[:i], l.Items[i+1:]...)
				return None, nil
			}
		}
		return nil, newExc("ValueError", "list.remove(x): x not in list")
	},
	"pop": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		l := recv.(*List)
		if len(l.Items) == 0 {
			return nil, newExc("IndexError", "pop from empty list")
		}
		idx := len(l.Items) - 1
		if len(args) > 0 {
			v, ok := intVal(args[0])
			if !ok {
				return nil, newExc("TypeError", "pop() 的下标必须是整数")
			}
			idx = v
			if idx < 0 {
				idx += len(l.Items)
			}
			if idx < 0 || idx >= len(l.Items) {
				return nil, newExc("IndexError", "pop index out of range")
			}
		}
		out := l.Items[idx]
		l.Items = append(l.Items[:idx], l.Items[idx+1:]...)
		return out, nil
	},
	"clear": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		recv.(*List).Items = nil
		return None, nil
	},
	"index": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("index", len(args), 1)
		}
		for i, it := range recv.(*List).Items {
			if objectsEqual(it, args[0]) {
				return i, nil
			}
		}
		return nil, newExc("ValueError", "%s is not in list", Repr(args[0]))
	},
	"count": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("count", len(args), 1)
		}
		n := 0
		for _, it := range recv.(*List).Items {
			if objectsEqual(it, args[0]) {
				n++
			}
		}
		return n, nil
	},
	"sort": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		l := recv.(*List)
		reverse := kwBool(kwargs, "reverse")
		keyFn, hasKey := kwargs["key"]
		if hasKey {
			decorated := make([]Object, 0, len(l.Items))
			for i, it := range l.Items {
				k, err := callObjectRef(keyFn, []Object{it}, nil)
				if err != nil {
					return nil, err
				}
				decorated = append(decorated, &Tuple{Items: []Object{k, i, it}})
			}
			sortedDec := sortObjectsByKey(decorated, reverse)
			out := make([]Object, len(sortedDec))
			for i, d := range sortedDec {
				out[i] = d.(*Tuple).Items[2]
			}
			l.Items = out
			return None, nil
		}
		l.Items = sortObjects(l.Items, reverse)
		return None, nil
	},
	"reverse": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		l := recv.(*List)
		for i, j := 0, len(l.Items)-1; i < j; i, j = i+1, j-1 {
			l.Items[i], l.Items[j] = l.Items[j], l.Items[i]
		}
		return None, nil
	},
	"copy": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		l := recv.(*List)
		out := make([]Object, len(l.Items))
		copy(out, l.Items)
		return &List{Items: out}, nil
	},
}

func sortObjectsByKey(items []Object, reverse bool) []Object {
	out := make([]Object, len(items))
	copy(out, items)
	sort.SliceStable(out, func(i, j int) bool {
		a := out[i].(*Tuple).Items[0]
		b := out[j].(*Tuple).Items[0]
		c, ok := compareValues(a, b)
		if !ok {
			ra, rb := Repr(a), Repr(b)
			if reverse {
				return ra > rb
			}
			return ra < rb
		}
		if reverse {
			return c > 0
		}
		return c < 0
	})
	return out
}

// ---------- 字典方法 ----------

var dictMethods = map[string]MethodFn{
	"keys": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*Dict)
		out := make([]Object, 0, d.Len())
		out = append(out, d.Keys...)
		return &List{Items: out}, nil
	},
	"values": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*Dict)
		out := make([]Object, 0, d.Len())
		for _, k := range d.Keys {
			out = append(out, d.Vals[keyOf(k)])
		}
		return &List{Items: out}, nil
	},
	"items": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*Dict)
		out := make([]Object, 0, d.Len())
		for _, k := range d.Keys {
			out = append(out, &Tuple{Items: []Object{k, d.Vals[keyOf(k)]}})
		}
		return &List{Items: out}, nil
	},
	"get": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("get", len(args), 1)
		}
		if v, ok := recv.(*Dict).Get(args[0]); ok {
			return v, nil
		}
		if len(args) >= 2 {
			return args[1], nil
		}
		return None, nil
	},
	"pop": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("pop", len(args), 1)
		}
		d := recv.(*Dict)
		if v, ok := d.Get(args[0]); ok {
			d.Delete(args[0])
			return v, nil
		}
		if len(args) >= 2 {
			return args[1], nil
		}
		return nil, newExc("KeyError", "%s", Repr(args[0]))
	},
	"setdefault": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("setdefault", len(args), 1)
		}
		d := recv.(*Dict)
		if v, ok := d.Get(args[0]); ok {
			return v, nil
		}
		def := None
		if len(args) >= 2 {
			def = args[1]
		}
		d.Set(args[0], def)
		return def, nil
	},
	"update": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*Dict)
		for _, a := range args {
			other, ok := a.(*Dict)
			if !ok {
				return nil, newExc("TypeError", "update() 需要字典参数")
			}
			for _, k := range other.Keys {
				d.Set(k, other.Vals[keyOf(k)])
			}
		}
		for name, v := range kwargs {
			d.Set(name, v)
		}
		return None, nil
	},
	"clear": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*Dict)
		d.Keys = nil
		d.Vals = map[string]Object{}
		return None, nil
	},
	"copy": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		d := recv.(*Dict)
		out := NewDict()
		for _, k := range d.Keys {
			out.Set(k, d.Vals[keyOf(k)])
		}
		return out, nil
	},
}

// ---------- 元组方法 ----------

var tupleMethods = map[string]MethodFn{
	"count": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("count", len(args), 1)
		}
		n := 0
		for _, it := range recv.(*Tuple).Items {
			if objectsEqual(it, args[0]) {
				n++
			}
		}
		return n, nil
	},
	"index": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("index", len(args), 1)
		}
		for i, it := range recv.(*Tuple).Items {
			if objectsEqual(it, args[0]) {
				return i, nil
			}
		}
		return nil, newExc("ValueError", "tuple.index(x): x not in tuple")
	},
}

// ---------- 集合方法 ----------

var setMethods = map[string]MethodFn{
	"add": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("add", len(args), 1)
		}
		recv.(*Set).Add(args[0])
		return None, nil
	},
	"remove": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("remove", len(args), 1)
		}
		s := recv.(*Set)
		if !s.Has(args[0]) {
			return nil, newExc("KeyError", "%s", Repr(args[0]))
		}
		s.Remove(args[0])
		return None, nil
	},
	"discard": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("discard", len(args), 1)
		}
		recv.(*Set).Remove(args[0])
		return None, nil
	},
	"clear": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		s := recv.(*Set)
		s.Vals = map[string]Object{}
		s.Order = nil
		return None, nil
	},
	"copy": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		out := NewSet()
		for _, it := range recv.(*Set).Order {
			out.Add(it)
		}
		return out, nil
	},
	"union": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		out := NewSet()
		for _, it := range recv.(*Set).Order {
			out.Add(it)
		}
		for _, a := range args {
			items, err := iterate(a)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				out.Add(it)
			}
		}
		return out, nil
	},
	"intersection": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		out := NewSet()
		for _, it := range recv.(*Set).Order {
			all := true
			for _, a := range args {
				s, ok := a.(*Set)
				if !ok || !s.Has(it) {
					all = false
					break
				}
			}
			if all {
				out.Add(it)
			}
		}
		return out, nil
	},
	"difference": func(recv Object, args []Object, kwargs map[string]Object) (Object, error) {
		out := NewSet()
		for _, it := range recv.(*Set).Order {
			found := false
			for _, a := range args {
				s, ok := a.(*Set)
				if ok && s.Has(it) {
					found = true
					break
				}
			}
			if !found {
				out.Add(it)
			}
		}
		return out, nil
	},
}

// ============ 内置函数 ============

var builtinFuncs = map[string]BuiltinFn{
	"print": func(args []Object, kwargs map[string]Object) (Object, error) {
		sep := " "
		if v, ok := kwargs["sep"]; ok {
			sep = Str(v)
		}
		end := "\n"
		if v, ok := kwargs["end"]; ok {
			end = Str(v)
		}
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = Str(a)
		}
		fmt.Print(strings.Join(parts, sep) + end)
		return None, nil
	},
	"len": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("len", len(args), 1)
		}
		switch x := args[0].(type) {
		case *List:
			return len(x.Items), nil
		case *Tuple:
			return len(x.Items), nil
		case *Dict:
			return x.Len(), nil
		case *Set:
			return x.Len(), nil
		case *Range:
			return x.Len(), nil
		case string:
			return len([]rune(x)), nil
		case *Instance:
			if fn, ok := args[0].(*Instance).Class.LookupMethod("__len__"); ok {
				v, err := callObjectRef(&Method{Recv: args[0], Fn: fn}, nil, nil)
				if err != nil {
					return nil, err
				}
				if n, ok := intVal(v); ok {
					return n, nil
				}
				return nil, newExc("TypeError", "__len__ 返回值必须是整数")
			}
		case *PyCounter:
			return x.D.Len(), nil
		case *PyDefaultDict:
			return x.D.Len(), nil
		case *PyOrderedDict:
			return x.D.Len(), nil
		case *PyDeque:
			return len(x.Items), nil
		}
		return nil, newExc("TypeError", "'%s' 对象没有长度", typeName(args[0]))
	},
	"str": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return "", nil
		}
		return Str(args[0]), nil
	},
	"repr": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return "", nil
		}
		return Repr(args[0]), nil
	},
	"type": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("type", len(args), 1)
		}
		return &PyType{Name: typeName(args[0])}, nil
	},
	"int": func(args []Object, kwargs map[string]Object) (Object, error) {
		return convertToInt(args)
	},
	// long 兼容 Python 2 的长整型转换；在本解释器中与 int 等价
	"long": func(args []Object, kwargs map[string]Object) (Object, error) {
		return convertToInt(args)
	},
	"float": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return 0.0, nil
		}
		if f, ok := toFloat(args[0]); ok {
			return f, nil
		}
		return nil, newExc("ValueError", "could not convert to float: %s", Repr(args[0]))
	},
	"bool": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return false, nil
		}
		return truthy(args[0]), nil
	},
	"list": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return &List{}, nil
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(items))
		copy(out, items)
		return &List{Items: out}, nil
	},
	"tuple": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return &Tuple{}, nil
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(items))
		copy(out, items)
		return &Tuple{Items: out}, nil
	},
	"set": func(args []Object, kwargs map[string]Object) (Object, error) {
		out := NewSet()
		if len(args) == 0 {
			return out, nil
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			out.Add(it)
		}
		return out, nil
	},
	"dict": func(args []Object, kwargs map[string]Object) (Object, error) {
		out := NewDict()
		for name, v := range kwargs {
			out.Set(name, v)
		}
		for _, a := range args {
			d, ok := a.(*Dict)
			if !ok {
				items, err := iterate(a)
				if err != nil {
					return nil, err
				}
				for _, it := range items {
					p, ok := it.(*Tuple)
					if !ok || len(p.Items) != 2 {
						return nil, newExc("ValueError", "dict() 的元素必须是 (key, value) 二元组")
					}
					out.Set(p.Items[0], p.Items[1])
				}
				continue
			}
			for _, k := range d.Keys {
				out.Set(k, d.Vals[keyOf(k)])
			}
		}
		return out, nil
	},
	"range": func(args []Object, kwargs map[string]Object) (Object, error) {
		start, stop, step := 0, 0, 1
		switch len(args) {
		case 1:
			v, ok := intVal(args[0])
			if !ok {
				return nil, newExc("TypeError", "range() 需要整数参数")
			}
			stop = v
		case 2, 3:
			v, ok := intVal(args[0])
			if !ok {
				return nil, newExc("TypeError", "range() 需要整数参数")
			}
			start = v
			v2, ok2 := intVal(args[1])
			if !ok2 {
				return nil, newExc("TypeError", "range() 需要整数参数")
			}
			stop = v2
			if len(args) == 3 {
				v3, ok3 := intVal(args[2])
				if !ok3 {
					return nil, newExc("TypeError", "range() 的步长必须是整数")
				}
				step = v3
			}
		default:
			return nil, newExc("TypeError", "range() 需要 1 到 3 个参数")
		}
		if step == 0 {
			return nil, newExc("ValueError", "range() arg 3 must not be zero")
		}
		return &Range{Start: start, Stop: stop, Step: step}, nil
	},
	"abs": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("abs", len(args), 1)
		}
		if i, ok := intVal(args[0]); ok {
			if i < 0 {
				return -i, nil
			}
			return i, nil
		}
		if f, ok := numVal(args[0]); ok {
			return math.Abs(f), nil
		}
		return nil, newExc("TypeError", "abs() 需要数值参数")
	},
	"round": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("round", len(args), 1)
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "round() 需要数值参数")
		}
		ndigits := 0
		if len(args) >= 2 {
			n, ok := intVal(args[1])
			if !ok {
				return nil, newExc("TypeError", "round() 的精度必须是整数")
			}
			ndigits = n
		} else if i, isInt := args[0].(int); isInt {
			return i, nil
		}
		shift := math.Pow(10, float64(ndigits))
		return math.Round(f*shift) / shift, nil
	},
	"pow": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("pow", len(args), 2)
		}
		return binaryOp("**", args[0], args[1])
	},
	"divmod": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("divmod", len(args), 2)
		}
		q, err := binaryOp("//", args[0], args[1])
		if err != nil {
			return nil, err
		}
		r, err := binaryOp("%", args[0], args[1])
		if err != nil {
			return nil, err
		}
		return &Tuple{Items: []Object{q, r}}, nil
	},
	"min": func(args []Object, kwargs map[string]Object) (Object, error) {
		items, err := flattenArgs(args)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, newExc("ValueError", "min() 参数为空")
		}
		return minMax(items, false, kwargs)
	},
	"max": func(args []Object, kwargs map[string]Object) (Object, error) {
		items, err := flattenArgs(args)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, newExc("ValueError", "max() 参数为空")
		}
		return minMax(items, true, kwargs)
	},
	"sum": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return nil, argCountErr("sum", 0, 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		start := 0
		if len(args) >= 2 {
			start = 0
			if i, ok := intVal(args[1]); ok {
				start = i
			} else if f, ok := numVal(args[1]); ok {
				res := f
				for _, it := range items {
					v, ok := numVal(it)
					if !ok {
						return nil, newExc("TypeError", "sum() 只能累加数值")
					}
					res += v
				}
				return res, nil
			}
		}
		res := start
		isFloat := false
		acc := float64(0)
		for _, it := range items {
			if i, ok := intVal(it); ok {
				res += i
				continue
			}
			if f, ok := numVal(it); ok {
				isFloat = true
				acc += f
				continue
			}
			return nil, newExc("TypeError", "sum() 只能累加数值")
		}
		if isFloat {
			return float64(res) + acc, nil
		}
		return res, nil
	},
	"sorted": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("sorted", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		reverse := kwBool(kwargs, "reverse")
		if keyFn, ok := kwargs["key"]; ok {
			decorated := make([]Object, 0, len(items))
			for i, it := range items {
				k, err := callObjectRef(keyFn, []Object{it}, nil)
				if err != nil {
					return nil, err
				}
				decorated = append(decorated, &Tuple{Items: []Object{k, i, it}})
			}
			sd := sortObjectsByKey(decorated, reverse)
			out := make([]Object, len(sd))
			for i, d := range sd {
				out[i] = d.(*Tuple).Items[2]
			}
			return &List{Items: out}, nil
		}
		return &List{Items: sortObjects(items, reverse)}, nil
	},
	"reversed": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("reversed", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(items))
		for i := range items {
			out[i] = items[len(items)-1-i]
		}
		return &List{Items: out}, nil
	},
	"enumerate": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("enumerate", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		start := 0
		if len(args) >= 2 {
			if v, ok := intVal(args[1]); ok {
				start = v
			}
		}
		out := make([]Object, len(items))
		for i, it := range items {
			out[i] = &Tuple{Items: []Object{start + i, it}}
		}
		return &List{Items: out}, nil
	},
	"zip": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("zip", len(args), 2)
		}
		var lists [][]Object
		for _, a := range args {
			items, err := iterate(a)
			if err != nil {
				return nil, err
			}
			lists = append(lists, items)
		}
		n := len(lists[0])
		for _, l := range lists {
			if len(l) < n {
				n = len(l)
			}
		}
		out := make([]Object, n)
		for i := 0; i < n; i++ {
			t := make([]Object, len(lists))
			for j := range lists {
				t[j] = lists[j][i]
			}
			out[i] = &Tuple{Items: t}
		}
		return &List{Items: out}, nil
	},
	"map": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("map", len(args), 2)
		}
		items, err := iterate(args[1])
		if err != nil {
			return nil, err
		}
		out := make([]Object, len(items))
		for i, it := range items {
			v, err := callObjectRef(args[0], []Object{it}, nil)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return &List{Items: out}, nil
	},
	"filter": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("filter", len(args), 2)
		}
		items, err := iterate(args[1])
		if err != nil {
			return nil, err
		}
		out := []Object{}
		for _, it := range items {
			v, err := callObjectRef(args[0], []Object{it}, nil)
			if err != nil {
				return nil, err
			}
			if truthy(v) {
				out = append(out, it)
			}
		}
		return &List{Items: out}, nil
	},
	"any": func(args []Object, kwargs map[string]Object) (Object, error) {
		items, err := flattenArgs(args)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if truthy(it) {
				return true, nil
			}
		}
		return false, nil
	},
	"all": func(args []Object, kwargs map[string]Object) (Object, error) {
		items, err := flattenArgs(args)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if !truthy(it) {
				return false, nil
			}
		}
		return true, nil
	},
	"chr": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("chr", len(args), 1)
		}
		n, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "chr() 需要整数参数")
		}
		return string(rune(n)), nil
	},
	"ord": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("ord", len(args), 1)
		}
		s, ok := args[0].(string)
		if !ok || len([]rune(s)) != 1 {
			return nil, newExc("TypeError", "ord() 需要长度为 1 的字符串")
		}
		return int([]rune(s)[0]), nil
	},
	"hex": func(args []Object, kwargs map[string]Object) (Object, error) {
		n, err := requireInt("hex", args)
		if err != nil {
			return nil, err
		}
		return formatRadix(n, "0x", 16), nil
	},
	"oct": func(args []Object, kwargs map[string]Object) (Object, error) {
		n, err := requireInt("oct", args)
		if err != nil {
			return nil, err
		}
		return formatRadix(n, "0o", 8), nil
	},
	"bin": func(args []Object, kwargs map[string]Object) (Object, error) {
		n, err := requireInt("bin", args)
		if err != nil {
			return nil, err
		}
		return formatRadix(n, "0b", 2), nil
	},
	"isinstance": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("isinstance", len(args), 2)
		}
		return isInstanceOf(args[0], args[1]), nil
	},
	"issubclass": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("issubclass", len(args), 2)
		}
		return isSubclassOf(args[0], args[1]), nil
	},
	"super": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) >= 2 {
			cls, ok := args[0].(*Class)
			if !ok {
				return nil, newExc("TypeError", "super() 的第一个参数必须是类")
			}
			return &Super{Cls: cls, Obj: args[1]}, nil
		}
		if activeInterp == nil || len(activeInterp.frames) == 0 {
			return nil, newExc("TypeError", "super() 需要在类方法中调用")
		}
		fr := activeInterp.frames[len(activeInterp.frames)-1]
		if fr.fn.DefClass == nil {
			return nil, newExc("TypeError", "super(): 当前函数不属于任何类")
		}
		// 取第一个非 star 参数作为 self / cls
		for _, p := range fr.fn.Params {
			if p.Star || p.Star2 {
				continue
			}
			v, ok := fr.local.vars[p.Name]
			if !ok {
				return nil, newExc("RuntimeError", "super(): 无法获取 self")
			}
			return &Super{Cls: fr.fn.DefClass, Obj: v}, nil
		}
		return nil, newExc("TypeError", "super(): 当前函数没有 self 参数")
	},
	"hasattr": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("hasattr", len(args), 2)
		}
		name, ok := args[1].(string)
		if !ok {
			return nil, newExc("TypeError", "hasattr() 的属性名必须是字符串")
		}
		_, err := getAttr(args[0], name)
		return err == nil, nil
	},
	"getattr": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("getattr", len(args), 2)
		}
		name, ok := args[1].(string)
		if !ok {
			return nil, newExc("TypeError", "getattr() 的属性名必须是字符串")
		}
		v, err := getAttr(args[0], name)
		if err != nil {
			if len(args) >= 3 {
				return args[2], nil
			}
			return nil, err
		}
		return v, nil
	},
	"setattr": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 3 {
			return nil, argCountErr("setattr", len(args), 3)
		}
		name, ok := args[1].(string)
		if !ok {
			return nil, newExc("TypeError", "setattr() 的属性名必须是字符串")
		}
		inst, ok := args[0].(*Instance)
		if !ok {
			return nil, newExc("TypeError", "setattr() 只能设置对象属性")
		}
		inst.Fields[name] = args[2]
		return None, nil
	},
	"id": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("id", len(args), 1)
		}
		addrCounter += 0x10
		return 0x1000 + addrCounter, nil
	},
	"input": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) > 0 {
			fmt.Print(Str(args[0]))
		}
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		return "", nil
	},
	"exit": func(args []Object, kwargs map[string]Object) (Object, error) {
		code := 0
		if len(args) > 0 {
			if n, ok := intVal(args[0]); ok {
				code = n
			}
		}
		os.Exit(code)
		return None, nil
	},
	"property": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("property", len(args), 1)
		}
		fn, ok := args[0].(*Function)
		if !ok {
			return nil, newExc("TypeError", "property() 需要函数参数")
		}
		return &Property{Getter: fn}, nil
	},
	"staticmethod": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("staticmethod", len(args), 1)
		}
		fn, ok := args[0].(*Function)
		if !ok {
			return nil, newExc("TypeError", "staticmethod() 需要函数参数")
		}
		return &StaticMethod{Fn: fn}, nil
	},
	"classmethod": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("classmethod", len(args), 1)
		}
		fn, ok := args[0].(*Function)
		if !ok {
			return nil, newExc("TypeError", "classmethod() 需要函数参数")
		}
		return &ClassMethod{Fn: fn}, nil
	},
	"next": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("next", len(args), 1)
		}
		g, ok := args[0].(*Generator)
		if !ok {
			return nil, newExc("TypeError", "next() 的参数必须是生成器，实际为 '%s'", typeName(args[0]))
		}
		if activeInterp == nil {
			return nil, newExc("RuntimeError", "解释器尚未初始化")
		}
		v, has, err := genNextRef(activeInterp, g)
		if err != nil {
			return nil, err
		}
		if !has {
			if len(args) >= 2 {
				return args[1], nil
			}
			return nil, newExc("StopIteration", "")
		}
		return v, nil
	},
	"iter": func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("iter", len(args), 1)
		}
		if g, ok := args[0].(*Generator); ok {
			return g, nil
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		cp := make([]Object, len(items))
		copy(cp, items)
		return &Generator{Items: cp}, nil
	},
}

func requireInt(name string, args []Object) (int, error) {
	if len(args) != 1 {
		return 0, argCountErr(name, len(args), 1)
	}
	n, ok := intVal(args[0])
	if !ok {
		return 0, newExc("TypeError", "%s() 需要整数参数", name)
	}
	return n, nil
}

// formatRadix 按指定进制格式化整数，带前缀（如 0x/0o/0b），与 python 的 hex/oct/bin 一致
func formatRadix(n int, prefix string, radix int) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return sign + prefix + strconv.FormatInt(int64(n), radix)
}

// convertToInt 实现 int() / long() 的转换逻辑（支持进制参数）
func convertToInt(args []Object) (Object, error) {
	if len(args) == 0 {
		return 0, nil
	}
	v := args[0]
	base := 10
	if len(args) >= 2 {
		b, ok := intVal(args[1])
		if !ok {
			return nil, newExc("TypeError", "int() 的进制必须是整数")
		}
		base = b
	}
	switch x := v.(type) {
	case int:
		return x, nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, newExc("ValueError", "cannot convert float %s to integer", formatFloat(x))
		}
		return int(x), nil
	case string:
		s := strings.TrimSpace(x)
		if n, err := strconv.ParseInt(s, base, 64); err == nil {
			return int(n), nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && base == 10 {
			return int(f), nil
		}
		return nil, newExc("ValueError", "invalid literal for int() with base %d: %s", base, Repr(x))
	}
	return nil, newExc("TypeError", "int() 参数无法转换为整数: '%s'", typeName(v))
}

// isqrtInt 返回不大于 √n 的最大整数（整数平方根）
func isqrtInt(n int) int {
	if n <= 0 {
		return 0
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x, y = y, (y+n/y)/2
	}
	return x
}

// lcmInt 计算两个整数的最小公倍数（结果非负）
func lcmInt(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	aa, bb := a, b
	if aa < 0 {
		aa = -aa
	}
	if bb < 0 {
		bb = -bb
	}
	g := 0
	for bb != 0 {
		aa, bb = bb, aa%bb
	}
	g = aa
	return (intValAbs(a) / g) * intValAbs(b)
}

func intValAbs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// mathComb 组合数 C(n, k)
func mathComb(n, k int) int {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	res := 1
	for i := 1; i <= k; i++ {
		res = res * (n - k + i) / i
	}
	return res
}

// mathPerm 排列数 P(n, k)
func mathPerm(n, k int) int {
	if k < 0 || k > n {
		return 0
	}
	res := 1
	for i := n - k + 1; i <= n; i++ {
		res *= i
	}
	return res
}

// flattenArgs 支持 min(1,2,3) 与 min([1,2,3]) 两种写法
func flattenArgs(args []Object) ([]Object, error) {
	if len(args) == 1 {
		if _, ok := args[0].(string); !ok {
			if items, err := iterate(args[0]); err == nil {
				return items, nil
			}
		}
	}
	return args, nil
}

func minMax(items []Object, wantMax bool, kwargs map[string]Object) (Object, error) {
	if keyFn, ok := kwargs["key"]; ok {
		best := items[0]
		bestKey, err := callObjectRef(keyFn, []Object{best}, nil)
		if err != nil {
			return nil, err
		}
		for _, it := range items[1:] {
			k, err := callObjectRef(keyFn, []Object{it}, nil)
			if err != nil {
				return nil, err
			}
			c, ok := compareValues(k, bestKey)
			if !ok {
				c, _ = compareValues(Repr(k), Repr(bestKey))
			}
			if (wantMax && c > 0) || (!wantMax && c < 0) {
				best, bestKey = it, k
			}
		}
		return best, nil
	}
	best := items[0]
	for _, it := range items[1:] {
		c, ok := compareValues(it, best)
		if !ok {
			c, _ = compareValues(Repr(it), Repr(best))
		}
		if (wantMax && c > 0) || (!wantMax && c < 0) {
			best = it
		}
	}
	return best, nil
}

func isInstanceOf(obj Object, cls Object) bool {
	switch c := cls.(type) {
	case *Class:
		inst, ok := obj.(*Instance)
		if !ok {
			return false
		}
		for _, cur := range inst.Class.MRO {
			if cur == c {
				return true
			}
		}
		return false
	case *PyType:
		return typeName(obj) == c.Name
	case *Tuple:
		for _, it := range c.Items {
			if isInstanceOf(obj, it) {
				return true
			}
		}
		return false
	}
	return false
}

// isSubclassOf 判断 sub 是否为 parent 的子类（沿 MRO）
func isSubclassOf(sub, parent Object) bool {
	switch p := parent.(type) {
	case *Class:
		s, ok := sub.(*Class)
		if !ok {
			return false
		}
		for _, cur := range s.MRO {
			if cur == p {
				return true
			}
		}
		return false
	case *PyType:
		s, ok := sub.(*PyType)
		if !ok {
			return false
		}
		return s.Name == p.Name
	case *Tuple:
		for _, it := range p.Items {
			if isSubclassOf(sub, it) {
				return true
			}
		}
		return false
	}
	return false
}

// ============ 内置模块 ============

func newMathModule() *Module {
	gcd := func(a, b int) int {
		if a < 0 {
			a = -a
		}
		if b < 0 {
			b = -b
		}
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	factorial := func(n int) int {
		r := 1
		for k := 2; k <= n; k++ {
			r *= k
		}
		return r
	}
	num1 := func(name string, args []Object, fn func(float64) float64) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr(name, len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "%s() 需要数值参数", name)
		}
		return fn(f), nil
	}
	m := &Module{Name: "math", Attrs: map[string]Object{}}
	bind := func(name string, fn func(args []Object, kwargs map[string]Object) (Object, error)) {
		m.Attrs[name] = &Builtin{Name: "math." + name, Fn: fn}
	}
	bind("floor", func(args []Object, kwargs map[string]Object) (Object, error) {
		v, err := num1("floor", args, math.Floor)
		if err != nil {
			return nil, err
		}
		return int(v.(float64)), nil
	})
	bind("ceil", func(args []Object, kwargs map[string]Object) (Object, error) {
		v, err := num1("ceil", args, math.Ceil)
		if err != nil {
			return nil, err
		}
		return int(v.(float64)), nil
	})
	bind("trunc", func(args []Object, kwargs map[string]Object) (Object, error) {
		v, err := num1("trunc", args, math.Trunc)
		if err != nil {
			return nil, err
		}
		return int(v.(float64)), nil
	})
	bind("sqrt", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("sqrt", args, math.Sqrt)
	})
	bind("fabs", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("fabs", args, math.Abs)
	})
	bind("exp", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("exp", args, math.Exp)
	})
	bind("log", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("log", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "log() 需要数值参数")
		}
		if len(args) >= 2 {
			b, ok := toFloat(args[1])
			if !ok {
				return nil, newExc("TypeError", "log() 的底数必须是数值")
			}
			return math.Log(f) / math.Log(b), nil
		}
		return math.Log(f), nil
	})
	bind("log10", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("log10", args, math.Log10)
	})
	bind("log2", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("log2", args, math.Log2)
	})
	bind("sin", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("sin", args, math.Sin)
	})
	bind("cos", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("cos", args, math.Cos)
	})
	bind("tan", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("tan", args, math.Tan)
	})
	bind("asin", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("asin", args, math.Asin)
	})
	bind("acos", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("acos", args, math.Acos)
	})
	bind("atan", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("atan", args, math.Atan)
	})
	// Go 标准库没有 math.Degrees / math.Radians，按定义换算。
	// 系数先算再乘，与 CPython 的做法一致，避免浮点结果的末位差异。
	bind("degrees", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("degrees", args, func(f float64) float64 { return f * (180 / math.Pi) })
	})
	bind("radians", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("radians", args, func(f float64) float64 { return f * (math.Pi / 180) })
	})
	bind("pow", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("pow", len(args), 2)
		}
		a, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "pow() 需要数值参数")
		}
		b, ok2 := toFloat(args[1])
		if !ok2 {
			return nil, newExc("TypeError", "pow() 需要数值参数")
		}
		return math.Pow(a, b), nil
	})
	bind("hypot", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("hypot", len(args), 2)
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Hypot(a, b), nil
	})
	bind("atan2", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("atan2", len(args), 2)
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Atan2(a, b), nil
	})
	bind("fmod", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("fmod", len(args), 2)
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Mod(a, b), nil
	})
	bind("copysign", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("copysign", len(args), 2)
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Copysign(a, b), nil
	})
	bind("modf", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("modf", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "modf() 需要数值参数")
		}
		i, frac := math.Modf(f)
		return &Tuple{Items: []Object{frac, i}}, nil
	})
	bind("factorial", func(args []Object, kwargs map[string]Object) (Object, error) {
		n, err := requireInt("factorial", args)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, newExc("ValueError", "factorial() not defined for negative values")
		}
		return factorial(n), nil
	})
	bind("gcd", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("gcd", len(args), 2)
		}
		a, ok := intVal(args[0])
		if !ok {
			return nil, newExc("TypeError", "gcd() 需要整数参数")
		}
		b, ok2 := intVal(args[1])
		if !ok2 {
			return nil, newExc("TypeError", "gcd() 需要整数参数")
		}
		return gcd(a, b), nil
	})
	bind("sinh", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("sinh", args, math.Sinh)
	})
	bind("cosh", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("cosh", args, math.Cosh)
	})
	bind("tanh", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("tanh", args, math.Tanh)
	})
	bind("asinh", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("asinh", args, math.Asinh)
	})
	bind("acosh", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("acosh", args, math.Acosh)
	})
	bind("atanh", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("atanh", args, math.Atanh)
	})
	bind("log1p", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("log1p", args, math.Log1p)
	})
	bind("expm1", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("expm1", args, math.Expm1)
	})
	bind("erf", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("erf", args, math.Erf)
	})
	bind("erfc", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("erfc", args, math.Erfc)
	})
	bind("gamma", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("gamma", args, math.Gamma)
	})
	bind("lgamma", func(args []Object, kwargs map[string]Object) (Object, error) {
		return num1("lgamma", args, func(f float64) float64 {
			v, _ := math.Lgamma(f)
			return v
		})
	})
	bind("isqrt", func(args []Object, kwargs map[string]Object) (Object, error) {
		n, err := requireInt("isqrt", args)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, newExc("ValueError", "isqrt() argument must be non-negative")
		}
		return isqrtInt(n), nil
	})
	bind("dist", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 2 {
			return nil, argCountErr("dist", len(args), 2)
		}
		p, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		q, err := iterate(args[1])
		if err != nil {
			return nil, err
		}
		if len(p) != len(q) {
			return nil, newExc("ValueError", "dist() 两个点的维度必须一致")
		}
		sum := 0.0
		for i := range p {
			a, ok1 := toFloat(p[i])
			b, ok2 := toFloat(q[i])
			if !ok1 || !ok2 {
				return nil, newExc("TypeError", "dist() 需要数值坐标")
			}
			d := a - b
			sum += d * d
		}
		return math.Sqrt(sum), nil
	})
	bind("comb", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("comb", len(args), 2)
		}
		n, ok1 := intVal(args[0])
		k, ok2 := intVal(args[1])
		if !ok1 || !ok2 {
			return nil, newExc("TypeError", "comb() 需要整数参数")
		}
		if n < 0 || k < 0 {
			return nil, newExc("ValueError", "comb() 不接受负数")
		}
		return mathComb(n, k), nil
	})
	bind("perm", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("perm", len(args), 2)
		}
		n, ok1 := intVal(args[0])
		k, ok2 := intVal(args[1])
		if !ok1 || !ok2 {
			return nil, newExc("TypeError", "perm() 需要整数参数")
		}
		if n < 0 || k < 0 {
			return nil, newExc("ValueError", "perm() 不接受负数")
		}
		if k > n {
			return 0, nil
		}
		return mathPerm(n, k), nil
	})
	bind("lcm", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) == 0 {
			return 1, nil
		}
		res := 0
		first := true
		for _, a := range args {
			ai, ok := intVal(a)
			if !ok {
				return nil, newExc("TypeError", "lcm() 需要整数参数")
			}
			if first {
				res = ai
				first = false
				continue
			}
			res = lcmInt(res, ai)
		}
		return res, nil
	})
	bind("prod", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("prod", len(args), 1)
		}
		items, err := iterate(args[0])
		if err != nil {
			return nil, err
		}
		startObj := Object(1)
		if v, ok := kwargs["start"]; ok {
			startObj = v
		} else if len(args) >= 2 {
			startObj = args[1]
		}
		if i, ok := intVal(startObj); ok {
			res := i
			allInt := true
			for _, it := range items {
				if j, ok := intVal(it); ok {
					res *= j
					continue
				}
				allInt = false
				break
			}
			if allInt {
				return res, nil
			}
			acc := float64(res)
			for _, it := range items {
				v, ok := numVal(it)
				if !ok {
					return nil, newExc("TypeError", "prod() 需要数值")
				}
				acc *= v
			}
			return acc, nil
		}
		acc, ok := numVal(startObj)
		if !ok {
			return nil, newExc("TypeError", "prod() 的 start 必须是数值")
		}
		for _, it := range items {
			v, ok := numVal(it)
			if !ok {
				return nil, newExc("TypeError", "prod() 需要数值")
			}
			acc *= v
		}
		return acc, nil
	})
	bind("isfinite", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("isfinite", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "isfinite() 需要数值参数")
		}
		return !(math.IsInf(f, 0) || math.IsNaN(f)), nil
	})
	bind("isinf", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("isinf", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "isinf() 需要数值参数")
		}
		return math.IsInf(f, 0), nil
	})
	bind("isnan", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("isnan", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "isnan() 需要数值参数")
		}
		return math.IsNaN(f), nil
	})
	bind("remainder", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("remainder", len(args), 2)
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Remainder(a, b), nil
	})
	bind("nextafter", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("nextafter", len(args), 2)
		}
		a, _ := toFloat(args[0])
		b, _ := toFloat(args[1])
		return math.Nextafter(a, b), nil
	})
	bind("ulp", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("ulp", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "ulp() 需要数值参数")
		}
		return math.Nextafter(f, math.Inf(1)) - f, nil
	})
	bind("frexp", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) != 1 {
			return nil, argCountErr("frexp", len(args), 1)
		}
		f, ok := toFloat(args[0])
		if !ok {
			return nil, newExc("TypeError", "frexp() 需要数值参数")
		}
		m, exp := math.Frexp(f)
		return &Tuple{Items: []Object{m, exp}}, nil
	})
	m.Attrs["pi"] = math.Pi
	m.Attrs["e"] = math.E
	m.Attrs["tau"] = math.Pi * 2
	m.Attrs["inf"] = math.Inf(1)
	m.Attrs["nan"] = math.NaN()
	return m
}

func newOsModule() *Module {
	m := &Module{Name: "os", Attrs: map[string]Object{}}
	bind := func(name string, fn func(args []Object, kwargs map[string]Object) (Object, error)) {
		m.Attrs[name] = &Builtin{Name: "os." + name, Fn: fn}
	}
	bind("getcwd", func(args []Object, kwargs map[string]Object) (Object, error) {
		wd, err := os.Getwd()
		if err != nil {
			return "", nil
		}
		return wd, nil
	})
	bind("listdir", func(args []Object, kwargs map[string]Object) (Object, error) {
		path := "."
		if len(args) > 0 {
			if s, ok := args[0].(string); ok {
				path = s
			}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, newExc("FileNotFoundError", "No such file or directory: '%s'", path)
		}
		out := make([]Object, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Name())
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].(string) < out[j].(string) })
		return &List{Items: out}, nil
	})
	bind("getpid", func(args []Object, kwargs map[string]Object) (Object, error) {
		return os.Getpid(), nil
	})
	bind("getenv", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("getenv", len(args), 1)
		}
		key, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "getenv() 需要字符串参数")
		}
		if v, ok := os.LookupEnv(key); ok {
			return v, nil
		}
		return None, nil
	})
	bind("environ", func(args []Object, kwargs map[string]Object) (Object, error) {
		return envDict(), nil
	})
	m.Attrs["environ"] = envDict()
	bind("mkdir", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("mkdir", len(args), 1)
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "mkdir() 需要字符串参数")
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	bind("makedirs", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("makedirs", len(args), 1)
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "makedirs() 需要字符串参数")
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	bind("remove", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("remove", len(args), 1)
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "remove() 需要字符串参数")
		}
		if err := os.Remove(path); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	bind("unlink", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("unlink", len(args), 1)
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "unlink() 需要字符串参数")
		}
		if err := os.Remove(path); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	bind("rmdir", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("rmdir", len(args), 1)
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "rmdir() 需要字符串参数")
		}
		if err := os.Remove(path); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	bind("rename", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 2 {
			return nil, argCountErr("rename", len(args), 2)
		}
		src, ok1 := args[0].(string)
		dst, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return nil, newExc("TypeError", "rename() 需要字符串参数")
		}
		if err := os.Rename(src, dst); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	bind("chdir", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("chdir", len(args), 1)
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, newExc("TypeError", "chdir() 需要字符串参数")
		}
		if err := os.Chdir(path); err != nil {
			return nil, newExc("OSError", "%s", err.Error())
		}
		return None, nil
	})
	m.Attrs["name"] = "posix"
	m.Attrs["sep"] = "/"

	// os.path 子模块
	p := &Module{Name: "os.path", Attrs: map[string]Object{}}
	pbind := func(name string, fn func(args []Object, kwargs map[string]Object) (Object, error)) {
		p.Attrs[name] = &Builtin{Name: "os.path." + name, Fn: fn}
	}
	pathArg := func(args []Object) string {
		if len(args) > 0 {
			if s, ok := args[0].(string); ok {
				return s
			}
		}
		return ""
	}
	pbind("join", func(args []Object, kwargs map[string]Object) (Object, error) {
		parts := make([]string, 0, len(args))
		for _, a := range args {
			s, ok := a.(string)
			if !ok {
				return nil, newExc("TypeError", "os.path.join() 需要字符串参数")
			}
			parts = append(parts, s)
		}
		return filepath.Join(parts...), nil
	})
	pbind("exists", func(args []Object, kwargs map[string]Object) (Object, error) {
		_, err := os.Stat(pathArg(args))
		return err == nil, nil
	})
	pbind("isfile", func(args []Object, kwargs map[string]Object) (Object, error) {
		fi, err := os.Stat(pathArg(args))
		return err == nil && !fi.IsDir(), nil
	})
	pbind("isdir", func(args []Object, kwargs map[string]Object) (Object, error) {
		fi, err := os.Stat(pathArg(args))
		return err == nil && fi.IsDir(), nil
	})
	pbind("basename", func(args []Object, kwargs map[string]Object) (Object, error) {
		return filepath.Base(pathArg(args)), nil
	})
	pbind("dirname", func(args []Object, kwargs map[string]Object) (Object, error) {
		return filepath.Dir(pathArg(args)), nil
	})
	pbind("abspath", func(args []Object, kwargs map[string]Object) (Object, error) {
		abs, err := filepath.Abs(pathArg(args))
		if err != nil {
			return "", nil
		}
		return abs, nil
	})
	pbind("splitext", func(args []Object, kwargs map[string]Object) (Object, error) {
		ext := filepath.Ext(pathArg(args))
		base := strings.TrimSuffix(pathArg(args), ext)
		return &Tuple{Items: []Object{base, ext}}, nil
	})
	pbind("split", func(args []Object, kwargs map[string]Object) (Object, error) {
		dir, file := filepath.Split(pathArg(args))
		if len(dir) > 1 {
			dir = strings.TrimRight(dir, "/")
		}
		return &Tuple{Items: []Object{dir, file}}, nil
	})
	pbind("normpath", func(args []Object, kwargs map[string]Object) (Object, error) {
		return filepath.Clean(pathArg(args)), nil
	})
	pbind("realpath", func(args []Object, kwargs map[string]Object) (Object, error) {
		abs, err := filepath.Abs(pathArg(args))
		if err != nil {
			return "", nil
		}
		return filepath.Clean(abs), nil
	})
	pbind("isabs", func(args []Object, kwargs map[string]Object) (Object, error) {
		return filepath.IsAbs(pathArg(args)), nil
	})
	pbind("relpath", func(args []Object, kwargs map[string]Object) (Object, error) {
		path := pathArg(args)
		base := "."
		if len(args) >= 2 {
			if s, ok := args[1].(string); ok {
				base = s
			}
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return path, nil
		}
		return rel, nil
	})
	pbind("commonpath", func(args []Object, kwargs map[string]Object) (Object, error) {
		if len(args) < 1 {
			return nil, argCountErr("commonpath", len(args), 1)
		}
		raw, err := iterate(args[0])
		if err != nil {
			return nil, newExc("TypeError", "commonpath() 需要路径列表")
		}
		paths := make([]string, 0, len(raw))
		for _, a := range raw {
			s, ok := a.(string)
			if !ok {
				return nil, newExc("TypeError", "commonpath() 需要字符串路径")
			}
			paths = append(paths, s)
		}
		if len(paths) == 0 {
			return nil, newExc("ValueError", "commonpath() 需要至少一个路径")
		}
		common, err := filepath.Rel(filepath.Dir(paths[0]), paths[0])
		if err != nil {
			return "", nil
		}
		common = filepath.Clean(common)
		for _, p := range paths[1:] {
			cp, err := filepath.Rel(filepath.Dir(p), p)
			if err != nil {
				return "", nil
			}
			common = commonPrefix(common, filepath.Clean(cp))
		}
		if common == "." {
			return filepath.Dir(paths[0]), nil
		}
		return filepath.Join(filepath.Dir(paths[0]), common), nil
	})
	pbind("getsize", func(args []Object, kwargs map[string]Object) (Object, error) {
		fi, err := os.Stat(pathArg(args))
		if err != nil {
			return nil, newExc("FileNotFoundError", "No such file or directory: '%s'", pathArg(args))
		}
		return int(fi.Size()), nil
	})
	m.Attrs["path"] = p
	return m
}

// commonPrefix 返回两个以 / 分隔路径的公共前缀（按段比较）
func commonPrefix(a, b string) string {
	as := strings.Split(a, "/")
	bs := strings.Split(b, "/")
	n := 0
	for n < len(as) && n < len(bs) && as[n] == bs[n] {
		n++
	}
	return strings.Join(as[:n], "/")
}

// envDict 由进程环境变量构造一个字典
func envDict() *Dict {
	d := NewDict()
	for _, kv := range os.Environ() {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			d.Set(kv[:idx], kv[idx+1:])
		}
	}
	return d
}

func newSysModule(argv []Object) *Module {
	m := &Module{Name: "sys", Attrs: map[string]Object{}}
	m.Attrs["argv"] = &List{Items: argv}
	m.Attrs["version"] = runtime.Version()
	m.Attrs["platform"] = runtime.GOOS
	m.Attrs["exit"] = &Builtin{Name: "sys.exit", Fn: func(args []Object, kwargs map[string]Object) (Object, error) {
		code := 0
		if len(args) > 0 {
			if n, ok := intVal(args[0]); ok {
				code = n
			}
		}
		os.Exit(code)
		return None, nil
	}}
	return m
}

// newCollectionsModule 构造 collections 模块
func newCollectionsModule() *Module {
	m := &Module{Name: "collections", Attrs: map[string]Object{}}
	bind := func(name string, fn func(args []Object, kwargs map[string]Object) (Object, error)) {
		m.Attrs[name] = &Builtin{Name: "collections." + name, Fn: fn}
	}
	bind("Counter", func(args []Object, kwargs map[string]Object) (Object, error) {
		c := &PyCounter{D: NewDict()}
		if len(args) > 0 {
			if d, ok := args[0].(*Dict); ok {
				for _, k := range d.Keys {
					c.D.Set(k, d.Vals[keyOf(k)])
				}
			} else if s, ok := args[0].(string); ok {
				for _, r := range s {
					addCount(c.D, string(r), 1)
				}
			} else {
				items, err := iterate(args[0])
				if err != nil {
					return nil, err
				}
				for _, it := range items {
					addCount(c.D, it, 1)
				}
			}
		}
		for k, v := range kwargs {
			c.D.Set(k, v)
		}
		return c, nil
	})
	bind("defaultdict", func(args []Object, kwargs map[string]Object) (Object, error) {
		factory := Object(None)
		if len(args) > 0 {
			factory = args[0]
			args = args[1:]
		}
		d := &PyDefaultDict{D: NewDict(), Factory: factory}
		for _, a := range args {
			if other, ok := a.(*Dict); ok {
				for _, k := range other.Keys {
					d.D.Set(k, other.Vals[keyOf(k)])
				}
			}
		}
		for k, v := range kwargs {
			d.D.Set(k, v)
		}
		return d, nil
	})
	bind("OrderedDict", func(args []Object, kwargs map[string]Object) (Object, error) {
		od := &PyOrderedDict{D: NewDict()}
		for _, a := range args {
			items, err := iterate(a)
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				p, ok := it.(*Tuple)
				if !ok || len(p.Items) != 2 {
					return nil, newExc("ValueError", "OrderedDict 的元素必须是 (key, value) 二元组")
				}
				od.D.Set(p.Items[0], p.Items[1])
			}
		}
		for k, v := range kwargs {
			od.D.Set(k, v)
		}
		return od, nil
	})
	bind("deque", func(args []Object, kwargs map[string]Object) (Object, error) {
		d := &PyDeque{}
		if len(args) > 0 {
			items, err := iterate(args[0])
			if err != nil {
				return nil, err
			}
			d.Items = append(d.Items, items...)
		}
		return d, nil
	})
	return m
}

// exceptionTypeNames 是支持的内建异常类型
var exceptionTypeNames = []string{
	"Exception", "BaseException", "ValueError", "TypeError", "IndexError",
	"KeyError", "ZeroDivisionError", "NameError", "AttributeError",
	"RuntimeError", "NotImplementedError", "StopIteration", "AssertionError",
	"FileNotFoundError", "ImportError", "SyntaxError", "OverflowError",
	"RecursionError", "ArithmeticError", "LookupError", "OSError",
}

func isExceptionTypeName(name string) bool {
	for _, n := range exceptionTypeNames {
		if n == name {
			return true
		}
	}
	return false
}

// initGlobalEnv 初始化全局作用域
func initGlobalEnv(argv []Object) *Environment {
	env := NewEnvironment(nil)
	for name, fn := range builtinFuncs {
		env.Set(name, &Builtin{Name: name, Fn: fn})
	}
	env.Set("math", newMathModule())
	env.Set("os", newOsModule())
	env.Set("sys", newSysModule(argv))
	env.Set("collections", newCollectionsModule())
	for _, name := range exceptionTypeNames {
		env.Set(name, &PyType{Name: name})
	}
	return env
}
