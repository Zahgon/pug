package js

import (
	"math"
)

// ToPrimitive converts objects to a primitive value with Object.prototype's toString and valueOf (arrays through
// join). Primitives are returned unchanged.
func ToPrimitive(v Value) (Value, error) {
	switch x := v.(type) {
	case *Array:
		return joinArray(x, ",", nil)
	case *Object:
		// An own toString or valueOf property shadows the inherited method; values that come from JSON are never
		// callable, so such an object has no usable conversion left.
		if x.Has("toString") {
			return nil, typeError("Cannot convert object to primitive value")
		}
		return "[object Object]", nil
	case *Function:
		return "function " + x.Name + "() { [native code] }", nil
	}
	return v, nil
}

// ToString implements the abstract operation of that name.
func ToString(v Value) (string, error) {
	switch x := v.(type) {
	case UndefinedType:
		return "undefined", nil
	case NullType:
		return "null", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case float64:
		return NumberToString(x), nil
	case string:
		return x, nil
	}
	p, err := ToPrimitive(v)
	if err != nil {
		return "", err
	}
	return ToString(p)
}

// MustString is ToString for values that are known not to throw (primitives).
func MustString(v Value) string {
	s, err := ToString(v)
	if err != nil {
		return ""
	}
	return s
}

// ToNumber implements the abstract operation of that name for primitives; objects give NaN after conversion to a
// primitive (see ToNumeric).
func ToNumber(v Value) float64 {
	n, _ := ToNumeric(v)
	return n
}

// ToNumeric is ToNumber with the error of the conversion of an object to a primitive.
func ToNumeric(v Value) (float64, error) {
	switch x := v.(type) {
	case UndefinedType:
		return math.NaN(), nil
	case NullType:
		return 0, nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case float64:
		return x, nil
	case string:
		return StringToNumber(x), nil
	}
	p, err := ToPrimitive(v)
	if err != nil {
		return 0, err
	}
	return ToNumeric(p)
}

// ToPropertyKey converts a value used in a[b] to the property key string.
func ToPropertyKey(v Value) (string, error) {
	return ToString(v)
}

// StrictEquals implements ===.
func StrictEquals(a, b Value) bool {
	switch x := a.(type) {
	case UndefinedType, NullType:
		return a == b
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case *Object:
		y, ok := b.(*Object)
		return ok && x == y
	case *Array:
		y, ok := b.(*Array)
		return ok && x == y
	case *Function:
		y, ok := b.(*Function)
		return ok && x == y
	}
	return false
}

// SameValueZero is the comparison Array.prototype.includes uses: === except that NaN equals NaN.
func SameValueZero(a, b Value) bool {
	x, ok1 := a.(float64)
	y, ok2 := b.(float64)
	if ok1 && ok2 && math.IsNaN(x) && math.IsNaN(y) {
		return true
	}
	return StrictEquals(a, b)
}

// specType returns the ECMAScript type of v: undefined, null, boolean, number, string or object.
func specType(v Value) string {
	switch v.(type) {
	case UndefinedType:
		return "undefined"
	case NullType:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	}
	return "object"
}

// LooseEquals implements ==.
func LooseEquals(a, b Value) (bool, error) {
	ta, tb := specType(a), specType(b)
	if ta == tb {
		return StrictEquals(a, b), nil
	}
	if IsNullish(a) && IsNullish(b) {
		return true, nil
	}
	switch {
	case ta == "number" && tb == "string":
		return a.(float64) == StringToNumber(b.(string)), nil
	case ta == "string" && tb == "number":
		return StringToNumber(a.(string)) == b.(float64), nil
	case ta == "boolean":
		return LooseEquals(ToNumber(a), b)
	case tb == "boolean":
		return LooseEquals(a, ToNumber(b))
	case ta == "object" && (tb == "string" || tb == "number"):
		p, err := ToPrimitive(a)
		if err != nil {
			return false, err
		}
		return LooseEquals(p, b)
	case tb == "object" && (ta == "string" || ta == "number"):
		p, err := ToPrimitive(b)
		if err != nil {
			return false, err
		}
		return LooseEquals(a, p)
	}
	return false, nil
}

// Compare implements the abstract relational comparison for <, <=, > and >=. It returns -1, 0 or 1, or ok false
// when the result is undefined (a NaN is involved).
func Compare(a, b Value) (int, bool, error) {
	pa, err := ToPrimitive(a)
	if err != nil {
		return 0, false, err
	}
	pb, err := ToPrimitive(b)
	if err != nil {
		return 0, false, err
	}
	sa, aStr := pa.(string)
	sb, bStr := pb.(string)
	if aStr && bStr {
		return CompareStrings(sa, sb), true, nil
	}
	x, y := ToNumber(pa), ToNumber(pb)
	if math.IsNaN(x) || math.IsNaN(y) {
		return 0, false, nil
	}
	switch {
	case x < y:
		return -1, true, nil
	case x > y:
		return 1, true, nil
	}
	return 0, true, nil
}

// Add implements the + operator.
func Add(a, b Value) (Value, error) {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x + y, nil
		}
	}
	pa, err := ToPrimitive(a)
	if err != nil {
		return nil, err
	}
	pb, err := ToPrimitive(b)
	if err != nil {
		return nil, err
	}
	sa, aStr := pa.(string)
	sb, bStr := pb.(string)
	if aStr || bStr {
		if !aStr {
			sa = MustString(pa)
		}
		if !bStr {
			sb = MustString(pb)
		}
		return Concat(sa, sb), nil
	}
	return ToNumber(pa) + ToNumber(pb), nil
}

// Arith implements the numeric binary operators -, *, / and %.
func Arith(op string, a, b Value) (Value, error) {
	x, err := ToNumeric(a)
	if err != nil {
		return nil, err
	}
	y, err := ToNumeric(b)
	if err != nil {
		return nil, err
	}
	switch op {
	case "-":
		return x - y, nil
	case "*":
		return x * y, nil
	case "/":
		return x / y, nil
	case "%":
		if math.IsInf(y, 0) && !math.IsInf(x, 0) {
			return x, nil
		}
		return math.Mod(x, y), nil
	}
	return math.NaN(), nil
}
