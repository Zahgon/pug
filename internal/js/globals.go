package js

import "math"

var globals map[string]Value

func init() {
	mathObj := NewObject()
	unaryMath := func(name string, f func(float64) float64) {
		mathObj.Set(name, method(name, func(this Value, args []Value) (Value, error) {
			n, err := ToNumeric(arg(args, 0))
			if err != nil {
				return nil, err
			}
			return f(n), nil
		}))
	}
	unaryMath("abs", math.Abs)
	unaryMath("ceil", math.Ceil)
	unaryMath("floor", math.Floor)
	unaryMath("round", round)
	unaryMath("sign", func(x float64) float64 {
		switch {
		case x > 0:
			return 1
		case x < 0:
			return -1
		}
		return x
	})
	unaryMath("sqrt", math.Sqrt)
	unaryMath("trunc", math.Trunc)
	mathObj.Set("max", method("max", func(this Value, args []Value) (Value, error) {
		return extremum(args, true)
	}))
	mathObj.Set("min", method("min", func(this Value, args []Value) (Value, error) {
		return extremum(args, false)
	}))
	mathObj.Set("PI", math.Pi)

	jsonObj := NewObject()
	jsonObj.Set("stringify", method("stringify", func(this Value, args []Value) (Value, error) {
		s, ok, err := Stringify(arg(args, 0), arg(args, 2))
		if err != nil {
			return nil, err
		}
		if !ok {
			return Undefined, nil
		}
		return s, nil
	}))
	jsonObj.Set("parse", method("parse", func(this Value, args []Value) (Value, error) {
		s, err := ToString(arg(args, 0))
		if err != nil {
			return nil, err
		}
		return ParseJSON(s)
	}))

	array := method("Array", func(this Value, args []Value) (Value, error) {
		return NewArray(args...), nil
	})
	array.Props = map[string]Value{"isArray": method("isArray", func(this Value, args []Value) (Value, error) {
		_, ok := arg(args, 0).(*Array)
		return ok, nil
	})}
	object := method("Object", func(this Value, args []Value) (Value, error) {
		if IsNullish(arg(args, 0)) {
			return NewObject(), nil
		}
		return args[0], nil
	})
	object.Props = map[string]Value{"keys": method("keys", func(this Value, args []Value) (Value, error) {
		return objectKeys(arg(args, 0))
	})}

	globals = map[string]Value{
		"Math":     mathObj,
		"JSON":     jsonObj,
		"NaN":      math.NaN(),
		"Infinity": math.Inf(1),
		"Array":    array,
		"Object":   object,
		"String": method("String", func(this Value, args []Value) (Value, error) {
			if len(args) == 0 {
				return "", nil
			}
			return ToString(args[0])
		}),
		"Number": method("Number", func(this Value, args []Value) (Value, error) {
			if len(args) == 0 {
				return 0.0, nil
			}
			return ToNumeric(args[0])
		}),
		"Boolean": method("Boolean", func(this Value, args []Value) (Value, error) {
			return Truthy(arg(args, 0)), nil
		}),
		"parseInt": method("parseInt", func(this Value, args []Value) (Value, error) {
			s, err := ToString(arg(args, 0))
			if err != nil {
				return nil, err
			}
			return ParseInt(s, arg(args, 1)), nil
		}),
		"parseFloat": method("parseFloat", func(this Value, args []Value) (Value, error) {
			s, err := ToString(arg(args, 0))
			if err != nil {
				return nil, err
			}
			return ParseFloat(s), nil
		}),
		"isNaN": method("isNaN", func(this Value, args []Value) (Value, error) {
			n, err := ToNumeric(arg(args, 0))
			return math.IsNaN(n), err
		}),
		"isFinite": method("isFinite", func(this Value, args []Value) (Value, error) {
			n, err := ToNumeric(arg(args, 0))
			return !math.IsNaN(n) && !math.IsInf(n, 0), err
		}),
	}
}

// Global returns the value of a global binding of the JavaScript environment the templates run in.
func Global(name string) (Value, bool) {
	v, ok := globals[name]
	return v, ok
}

// round implements Math.round: halves round towards +Infinity.
func round(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || x == 0 {
		return x
	}
	if x > 0 && x < 0.5 {
		return 0
	}
	if x < 0 && x >= -0.5 {
		return math.Copysign(0, -1)
	}
	f := math.Floor(x)
	if x-f >= 0.5 {
		return f + 1
	}
	return f
}

func extremum(args []Value, max bool) (Value, error) {
	result := math.Inf(-1)
	if !max {
		result = math.Inf(1)
	}
	nan := false
	for _, a := range args {
		n, err := ToNumeric(a)
		if err != nil {
			return nil, err
		}
		switch {
		case math.IsNaN(n):
			nan = true
		case max && (n > result || (n == 0 && result == 0 && !math.Signbit(n))):
			result = n
		case !max && (n < result || (n == 0 && result == 0 && math.Signbit(n))):
			result = n
		}
	}
	if nan {
		return math.NaN(), nil
	}
	return result, nil
}

func objectKeys(v Value) (Value, error) {
	out := NewArray()
	switch x := v.(type) {
	case UndefinedType, NullType:
		return nil, typeError("Cannot convert undefined or null to object")
	case string:
		for i := 0; i < Length(x); i++ {
			out.Elems = append(out.Elems, NumberToString(float64(i)))
		}
	case *Array:
		for i := range x.Elems {
			out.Elems = append(out.Elems, NumberToString(float64(i)))
		}
	case *Object:
		for _, k := range x.Keys() {
			out.Elems = append(out.Elems, k)
		}
	}
	return out, nil
}
