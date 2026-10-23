package js

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// arg returns args[i], or undefined when it was not passed.
func arg(args []Value, i int) Value {
	if i < len(args) {
		return args[i]
	}
	return Undefined
}

func method(name string, call func(this Value, args []Value) (Value, error)) *Function {
	return &Function{Name: name, Call: call}
}

// GetMember implements property access v[key] for the values of this package.
func GetMember(v Value, key string) (Value, error) {
	switch x := v.(type) {
	case UndefinedType:
		return nil, typeError("Cannot read properties of undefined (reading '%s')", key)
	case NullType:
		return nil, typeError("Cannot read properties of null (reading '%s')", key)
	case string:
		if key == "length" {
			return float64(Length(x)), nil
		}
		if i, ok := ArrayIndex(key); ok {
			u := ToUTF16(x)
			if int(i) < len(u) {
				return FromUTF16(u[i : i+1]), nil
			}
			return Undefined, nil
		}
		if f, ok := stringMethods[key]; ok {
			return f, nil
		}
		return objectProtoMember(key), nil
	case float64:
		if f, ok := numberMethods[key]; ok {
			return f, nil
		}
		return objectProtoMember(key), nil
	case bool:
		if key == "toString" {
			return booleanToString, nil
		}
		return objectProtoMember(key), nil
	case *Array:
		if key == "length" {
			return float64(len(x.Elems)), nil
		}
		if i, ok := ArrayIndex(key); ok {
			if int(i) < len(x.Elems) {
				return x.Elems[i], nil
			}
			return Undefined, nil
		}
		if f, ok := arrayMethods[key]; ok {
			return f, nil
		}
		return objectProtoMember(key), nil
	case *Object:
		if val, ok := x.Get(key); ok {
			return val, nil
		}
		return objectProtoMember(key), nil
	case *Function:
		if val, ok := x.Props[key]; ok {
			return val, nil
		}
		if key == "name" {
			return x.Name, nil
		}
		return objectProtoMember(key), nil
	}
	return Undefined, nil
}

func objectProtoMember(key string) Value {
	if key == "toString" {
		return objectToString
	}
	return Undefined
}

// SetMember implements the assignment target[key] = val (sloppy mode: writes to primitives are ignored).
func SetMember(target Value, key string, val Value) error {
	switch x := target.(type) {
	case UndefinedType:
		return typeError("Cannot set properties of undefined (setting '%s')", key)
	case NullType:
		return typeError("Cannot set properties of null (setting '%s')", key)
	case *Object:
		x.Set(key, val)
	case *Array:
		if key == "length" {
			n := ToNumber(val)
			if n < 0 || n != math.Trunc(n) || n > math.MaxUint32-1 {
				return rangeError("Invalid array length")
			}
			for len(x.Elems) < int(n) {
				x.Elems = append(x.Elems, Undefined)
			}
			x.Elems = x.Elems[:int(n)]
			return nil
		}
		if i, ok := ArrayIndex(key); ok {
			for len(x.Elems) <= int(i) {
				x.Elems = append(x.Elems, Undefined)
			}
			x.Elems[i] = val
		}
	case *Function:
		if x.Props == nil {
			x.Props = map[string]Value{}
		}
		x.Props[key] = val
	}
	return nil
}

var objectToString = method("toString", func(this Value, args []Value) (Value, error) {
	switch this.(type) {
	case UndefinedType:
		return "[object Undefined]", nil
	case NullType:
		return "[object Null]", nil
	case *Array:
		return "[object Array]", nil
	case *Function:
		return "[object Function]", nil
	case string:
		return "[object String]", nil
	case float64:
		return "[object Number]", nil
	case bool:
		return "[object Boolean]", nil
	}
	return "[object Object]", nil
})

var booleanToString = method("toString", func(this Value, args []Value) (Value, error) {
	return MustString(this), nil
})

// relativeIndex clamps ToIntegerOrInfinity(v) counted from the end when negative, as slice does.
func relativeIndex(v Value, length int, def int) int {
	if _, ok := v.(UndefinedType); ok {
		return def
	}
	n := ToIntegerOrInfinity(v)
	if n < 0 {
		n = math.Max(0, float64(length)+n)
	} else {
		n = math.Min(n, float64(length))
	}
	return int(n)
}

// clampIndex clamps ToIntegerOrInfinity(v) to 0..length.
func clampIndex(v Value, length int) int {
	n := ToIntegerOrInfinity(v)
	return int(math.Max(0, math.Min(n, float64(length))))
}

func thisUnits(this Value) []uint16 {
	return ToUTF16(MustString(this))
}

func stringArg(args []Value, i int) (string, error) {
	return ToString(arg(args, i))
}

var stringMethods map[string]*Function

func init() {
	stringMethods = map[string]*Function{
		"at": method("at", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			k := ToIntegerOrInfinity(arg(args, 0))
			if k < 0 {
				k += float64(len(u))
			}
			if k < 0 || k >= float64(len(u)) {
				return Undefined, nil
			}
			return FromUTF16(u[int(k) : int(k)+1]), nil
		}),
		"charAt": method("charAt", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			k := ToIntegerOrInfinity(arg(args, 0))
			if k < 0 || k >= float64(len(u)) {
				return "", nil
			}
			return FromUTF16(u[int(k) : int(k)+1]), nil
		}),
		"charCodeAt": method("charCodeAt", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			k := ToIntegerOrInfinity(arg(args, 0))
			if k < 0 || k >= float64(len(u)) {
				return math.NaN(), nil
			}
			return float64(u[int(k)]), nil
		}),
		"concat": method("concat", func(this Value, args []Value) (Value, error) {
			parts := []string{MustString(this)}
			for _, a := range args {
				s, err := ToString(a)
				if err != nil {
					return nil, err
				}
				parts = append(parts, s)
			}
			return Concat(parts...), nil
		}),
		"endsWith": method("endsWith", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			s, err := stringArg(args, 0)
			if err != nil {
				return nil, err
			}
			end := len(u)
			if _, ok := arg(args, 1).(UndefinedType); !ok {
				end = clampIndex(args[1], len(u))
			}
			sub := ToUTF16(s)
			start := end - len(sub)
			return start >= 0 && unitsHasPrefix(u[start:end], sub), nil
		}),
		"includes": method("includes", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			s, err := stringArg(args, 0)
			if err != nil {
				return nil, err
			}
			return unitsIndex(u, ToUTF16(s), clampIndex(arg(args, 1), len(u))) >= 0, nil
		}),
		"indexOf": method("indexOf", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			s, err := stringArg(args, 0)
			if err != nil {
				return nil, err
			}
			return float64(unitsIndex(u, ToUTF16(s), clampIndex(arg(args, 1), len(u)))), nil
		}),
		"lastIndexOf": method("lastIndexOf", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			s, err := stringArg(args, 0)
			if err != nil {
				return nil, err
			}
			sub := ToUTF16(s)
			pos := math.Inf(1)
			if n := ToNumber(arg(args, 1)); !math.IsNaN(n) {
				pos = ToIntegerOrInfinity(n)
			}
			start := int(math.Max(0, math.Min(pos, float64(len(u)-len(sub)))))
			for i := start; i >= 0; i-- {
				if i+len(sub) <= len(u) && unitsHasPrefix(u[i:], sub) {
					return float64(i), nil
				}
			}
			return -1.0, nil
		}),
		"padEnd": method("padEnd", func(this Value, args []Value) (Value, error) {
			return pad(this, args, false)
		}),
		"padStart": method("padStart", func(this Value, args []Value) (Value, error) {
			return pad(this, args, true)
		}),
		"repeat": method("repeat", func(this Value, args []Value) (Value, error) {
			s := MustString(this)
			n := ToIntegerOrInfinity(arg(args, 0))
			if n < 0 || math.IsInf(n, 0) {
				return nil, rangeError("Invalid count value: %s", NumberToString(ToNumber(arg(args, 0))))
			}
			if n == 0 || s == "" {
				return "", nil
			}
			if float64(Length(s))*n > (1<<29)-24 {
				return nil, rangeError("Invalid string length")
			}
			return strings.Repeat(s, int(n)), nil
		}),
		"replace": method("replace", func(this Value, args []Value) (Value, error) {
			return replace(this, args, false)
		}),
		"replaceAll": method("replaceAll", func(this Value, args []Value) (Value, error) {
			return replace(this, args, true)
		}),
		"slice": method("slice", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			from := relativeIndex(arg(args, 0), len(u), 0)
			to := relativeIndex(arg(args, 1), len(u), len(u))
			if from >= to {
				return "", nil
			}
			return FromUTF16(u[from:to]), nil
		}),
		"split": method("split", func(this Value, args []Value) (Value, error) {
			s := MustString(this)
			lim := uint32(math.MaxUint32)
			if _, ok := arg(args, 1).(UndefinedType); !ok {
				lim = uint32(ToInt32(ToNumber(args[1])))
			}
			if _, ok := arg(args, 0).(UndefinedType); ok {
				if lim == 0 {
					return NewArray(), nil
				}
				return NewArray(s), nil
			}
			sep, err := ToString(args[0])
			if err != nil {
				return nil, err
			}
			out := NewArray()
			if lim == 0 {
				return out, nil
			}
			u, r := ToUTF16(s), ToUTF16(sep)
			if len(r) == 0 {
				for i := 0; i < len(u) && uint32(len(out.Elems)) < lim; i++ {
					out.Elems = append(out.Elems, FromUTF16(u[i:i+1]))
				}
				return out, nil
			}
			if len(u) == 0 {
				out.Elems = append(out.Elems, s)
				return out, nil
			}
			p := 0
			for {
				q := unitsIndex(u, r, p)
				if q < 0 {
					break
				}
				out.Elems = append(out.Elems, FromUTF16(u[p:q]))
				if uint32(len(out.Elems)) == lim {
					return out, nil
				}
				p = q + len(r)
			}
			out.Elems = append(out.Elems, FromUTF16(u[p:]))
			return out, nil
		}),
		"startsWith": method("startsWith", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			s, err := stringArg(args, 0)
			if err != nil {
				return nil, err
			}
			start := clampIndex(arg(args, 1), len(u))
			return unitsHasPrefix(u[start:], ToUTF16(s)), nil
		}),
		"substring": method("substring", func(this Value, args []Value) (Value, error) {
			u := thisUnits(this)
			a := clampIndex(arg(args, 0), len(u))
			b := len(u)
			if _, ok := arg(args, 1).(UndefinedType); !ok {
				b = clampIndex(args[1], len(u))
			}
			if a > b {
				a, b = b, a
			}
			return FromUTF16(u[a:b]), nil
		}),
		"toLowerCase": method("toLowerCase", func(this Value, args []Value) (Value, error) {
			return ToLowerCase(MustString(this)), nil
		}),
		"toUpperCase": method("toUpperCase", func(this Value, args []Value) (Value, error) {
			return ToUpperCase(MustString(this)), nil
		}),
		"toString": method("toString", func(this Value, args []Value) (Value, error) {
			return MustString(this), nil
		}),
		"trim": method("trim", func(this Value, args []Value) (Value, error) {
			return FromUTF16(trimUnits(thisUnits(this), true, true)), nil
		}),
		"trimEnd": method("trimEnd", func(this Value, args []Value) (Value, error) {
			return FromUTF16(trimUnits(thisUnits(this), false, true)), nil
		}),
		"trimStart": method("trimStart", func(this Value, args []Value) (Value, error) {
			return FromUTF16(trimUnits(thisUnits(this), true, false)), nil
		}),
	}
}

func pad(this Value, args []Value, atStart bool) (Value, error) {
	s := MustString(this)
	u := ToUTF16(s)
	max := ToIntegerOrInfinity(arg(args, 0))
	if max <= float64(len(u)) {
		return s, nil
	}
	fill := " "
	if _, ok := arg(args, 1).(UndefinedType); !ok {
		f, err := ToString(args[1])
		if err != nil {
			return nil, err
		}
		fill = f
	}
	f := ToUTF16(fill)
	if len(f) == 0 {
		return s, nil
	}
	if max > (1<<29)-24 {
		return nil, rangeError("Invalid string length")
	}
	n := int(max) - len(u)
	padding := make([]uint16, 0, n)
	for len(padding) < n {
		padding = append(padding, f...)
	}
	padding = padding[:n]
	if atStart {
		return FromUTF16(append(padding, u...)), nil
	}
	return FromUTF16(append(u, padding...)), nil
}

func replace(this Value, args []Value, all bool) (Value, error) {
	s := MustString(this)
	pattern, err := stringArg(args, 0)
	if err != nil {
		return nil, err
	}
	replacement, err := stringArg(args, 1)
	if err != nil {
		return nil, err
	}
	u, p := ToUTF16(s), ToUTF16(pattern)
	var positions []int
	for pos := unitsIndex(u, p, 0); pos >= 0; {
		positions = append(positions, pos)
		if !all {
			break
		}
		step := len(p)
		if step == 0 {
			step = 1
		}
		if pos+step > len(u) {
			break
		}
		pos = unitsIndex(u, p, pos+step)
	}
	if len(positions) == 0 {
		return s, nil
	}
	r := ToUTF16(replacement)
	var out []uint16
	last := 0
	for _, pos := range positions {
		out = append(out, u[last:pos]...)
		out = append(out, substitution(u, p, pos, r)...)
		last = pos + len(p)
	}
	out = append(out, u[last:]...)
	return FromUTF16(out), nil
}

// substitution implements GetSubstitution for a string pattern (no capture groups).
func substitution(u, matched []uint16, pos int, r []uint16) []uint16 {
	var out []uint16
	for i := 0; i < len(r); i++ {
		if r[i] != '$' || i+1 >= len(r) {
			out = append(out, r[i])
			continue
		}
		switch r[i+1] {
		case '$':
			out = append(out, '$')
			i++
		case '&':
			out = append(out, matched...)
			i++
		case '`':
			out = append(out, u[:pos]...)
			i++
		case '\'':
			if end := pos + len(matched); end < len(u) {
				out = append(out, u[end:]...)
			}
			i++
		default:
			out = append(out, '$')
		}
	}
	return out
}

func thisArray(this Value) *Array {
	if a, ok := this.(*Array); ok {
		return a
	}
	return NewArray()
}

var arrayMethods map[string]*Function

func init() {
	arrayMethods = map[string]*Function{
		"at": method("at", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			k := ToIntegerOrInfinity(arg(args, 0))
			if k < 0 {
				k += float64(len(a.Elems))
			}
			if k < 0 || k >= float64(len(a.Elems)) {
				return Undefined, nil
			}
			return a.Elems[int(k)], nil
		}),
		"concat": method("concat", func(this Value, args []Value) (Value, error) {
			out := NewArray(append([]Value(nil), thisArray(this).Elems...)...)
			for _, x := range args {
				if a, ok := x.(*Array); ok {
					out.Elems = append(out.Elems, a.Elems...)
				} else {
					out.Elems = append(out.Elems, x)
				}
			}
			return out, nil
		}),
		"includes": method("includes", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			for i := relativeIndex(arg(args, 1), len(a.Elems), 0); i < len(a.Elems); i++ {
				if SameValueZero(a.Elems[i], arg(args, 0)) {
					return true, nil
				}
			}
			return false, nil
		}),
		"indexOf": method("indexOf", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			for i := relativeIndex(arg(args, 1), len(a.Elems), 0); i < len(a.Elems); i++ {
				if StrictEquals(a.Elems[i], arg(args, 0)) {
					return float64(i), nil
				}
			}
			return -1.0, nil
		}),
		"join": method("join", func(this Value, args []Value) (Value, error) {
			sep := ","
			if _, ok := arg(args, 0).(UndefinedType); !ok {
				s, err := ToString(args[0])
				if err != nil {
					return nil, err
				}
				sep = s
			}
			return joinArray(thisArray(this), sep, nil)
		}),
		"lastIndexOf": method("lastIndexOf", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			start := len(a.Elems) - 1
			if len(args) > 1 {
				n := ToIntegerOrInfinity(args[1])
				if n < 0 {
					n += float64(len(a.Elems))
				}
				start = int(math.Min(n, float64(len(a.Elems)-1)))
			}
			for i := start; i >= 0; i-- {
				if StrictEquals(a.Elems[i], arg(args, 0)) {
					return float64(i), nil
				}
			}
			return -1.0, nil
		}),
		"pop": method("pop", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			if len(a.Elems) == 0 {
				return Undefined, nil
			}
			v := a.Elems[len(a.Elems)-1]
			a.Elems = a.Elems[:len(a.Elems)-1]
			return v, nil
		}),
		"push": method("push", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			a.Elems = append(a.Elems, args...)
			return float64(len(a.Elems)), nil
		}),
		"reverse": method("reverse", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			for i, j := 0, len(a.Elems)-1; i < j; i, j = i+1, j-1 {
				a.Elems[i], a.Elems[j] = a.Elems[j], a.Elems[i]
			}
			return a, nil
		}),
		"shift": method("shift", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			if len(a.Elems) == 0 {
				return Undefined, nil
			}
			v := a.Elems[0]
			a.Elems = append([]Value(nil), a.Elems[1:]...)
			return v, nil
		}),
		"slice": method("slice", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			from := relativeIndex(arg(args, 0), len(a.Elems), 0)
			to := relativeIndex(arg(args, 1), len(a.Elems), len(a.Elems))
			out := NewArray()
			if from < to {
				out.Elems = append(out.Elems, a.Elems[from:to]...)
			}
			return out, nil
		}),
		"sort": method("sort", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			if _, ok := arg(args, 0).(UndefinedType); !ok {
				return nil, typeError("The comparison function must be either a function or undefined")
			}
			var defined []Value
			var keys []string
			undefined := 0
			for _, e := range a.Elems {
				if _, ok := e.(UndefinedType); ok {
					undefined++
					continue
				}
				s, err := ToString(e)
				if err != nil {
					return nil, err
				}
				defined = append(defined, e)
				keys = append(keys, s)
			}
			idx := make([]int, len(defined))
			for i := range idx {
				idx[i] = i
			}
			sort.SliceStable(idx, func(i, j int) bool { return CompareStrings(keys[idx[i]], keys[idx[j]]) < 0 })
			elems := make([]Value, 0, len(a.Elems))
			for _, i := range idx {
				elems = append(elems, defined[i])
			}
			for ; undefined > 0; undefined-- {
				elems = append(elems, Undefined)
			}
			a.Elems = elems
			return a, nil
		}),
		"toString": method("toString", func(this Value, args []Value) (Value, error) {
			return joinArray(thisArray(this), ",", nil)
		}),
		"unshift": method("unshift", func(this Value, args []Value) (Value, error) {
			a := thisArray(this)
			a.Elems = append(append([]Value(nil), args...), a.Elems...)
			return float64(len(a.Elems)), nil
		}),
	}
}

// joinArray implements Array.prototype.join; arrays already being joined (cycles) give the empty string.
func joinArray(a *Array, sep string, seen []*Array) (Value, error) {
	for _, s := range seen {
		if s == a {
			return "", nil
		}
	}
	seen = append(seen, a)
	parts := make([]string, len(a.Elems))
	for i, e := range a.Elems {
		switch x := e.(type) {
		case UndefinedType, NullType:
		case *Array:
			s, err := joinArray(x, ",", seen)
			if err != nil {
				return nil, err
			}
			parts[i] = s.(string)
		default:
			s, err := ToString(x)
			if err != nil {
				return nil, err
			}
			parts[i] = s
		}
	}
	return Concat(strings.Join(parts, sep)), nil
}

var numberMethods map[string]*Function

func init() {
	numberMethods = map[string]*Function{
		"toFixed": method("toFixed", func(this Value, args []Value) (Value, error) {
			x := this.(float64)
			d := ToIntegerOrInfinity(arg(args, 0))
			if d < 0 || d > 100 {
				return nil, rangeError("toFixed() digits argument must be between 0 and 100")
			}
			return ToFixed(x, int(d)), nil
		}),
		"toString": method("toString", func(this Value, args []Value) (Value, error) {
			x := this.(float64)
			radix := 10.0
			if _, ok := arg(args, 0).(UndefinedType); !ok {
				radix = ToIntegerOrInfinity(args[0])
			}
			if radix < 2 || radix > 36 {
				return nil, rangeError("toString() radix argument must be between 2 and 36")
			}
			if radix == 10 || math.IsNaN(x) || math.IsInf(x, 0) {
				return NumberToString(x), nil
			}
			if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
				return strconv.FormatInt(int64(x), int(radix)), nil
			}
			return NumberToString(x), nil
		}),
	}
}
