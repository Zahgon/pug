// Package js implements the subset of JavaScript that pug templates evaluate: the value model, the operators with
// their coercions, the built-in methods templates call, JSON, and an interpreter for expressions and the statements
// of unbuffered code. Behaviour follows ECMAScript as V8 implements it, including the texts of the errors V8 throws.
package js

import (
	"math"
	"sort"
	"strconv"
)

// Value is a JavaScript value: Undefined, Null, bool, float64, string (a JavaScript string held as WTF-8, see
// FromUTF16), *Object, *Array or *Function.
type Value any

// UndefinedType is the type of Undefined.
type UndefinedType struct{}

// NullType is the type of Null.
type NullType struct{}

var (
	// Undefined is the JavaScript undefined value.
	Undefined Value = UndefinedType{}
	// Null is the JavaScript null value.
	Null Value = NullType{}
)

// Object is a plain JavaScript object. Its own properties keep JavaScript's order: integer index keys in ascending
// numeric order first, then the other keys in insertion order.
type Object struct {
	keys  []string
	props map[string]Value
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{props: map[string]Value{}}
}

// Get returns the own property key and whether it exists.
func (o *Object) Get(key string) (Value, bool) {
	v, ok := o.props[key]
	return v, ok
}

// Set creates or updates the own property key; a new key is added at the end of the insertion order.
func (o *Object) Set(key string, v Value) {
	if _, ok := o.props[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.props[key] = v
}

// Has reports whether key is an own property.
func (o *Object) Has(key string) bool {
	_, ok := o.props[key]
	return ok
}

// Keys returns the own property keys in JavaScript order.
func (o *Object) Keys() []string {
	var index []string
	var other []string
	for _, k := range o.keys {
		if _, ok := ArrayIndex(k); ok {
			index = append(index, k)
		} else {
			other = append(other, k)
		}
	}
	if len(index) == 0 {
		return other
	}
	sort.SliceStable(index, func(i, j int) bool {
		a, _ := ArrayIndex(index[i])
		b, _ := ArrayIndex(index[j])
		return a < b
	})
	return append(index, other...)
}

// Len returns the number of own properties.
func (o *Object) Len() int {
	return len(o.keys)
}

// ArrayIndex reports whether key is the canonical form of an array index (0 to 2^32-2) and returns it.
func ArrayIndex(key string) (uint32, bool) {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(key, 10, 64)
	if err != nil || n >= math.MaxUint32 {
		return 0, false
	}
	return uint32(n), true
}

// Array is a JavaScript array. Holes are not modelled: a missing element reads as undefined.
type Array struct {
	Elems []Value
}

// NewArray returns an array holding elems.
func NewArray(elems ...Value) *Array {
	return &Array{Elems: elems}
}

// Function is a callable value: a built-in function or method.
type Function struct {
	Name string
	// Call runs the function with the receiver this and the arguments.
	Call func(this Value, args []Value) (Value, error)
	// Props holds the function's own properties (for example Array.isArray on Array).
	Props map[string]Value
}

// TypeOf returns the result of the typeof operator.
func TypeOf(v Value) string {
	switch v.(type) {
	case UndefinedType:
		return "undefined"
	case NullType:
		return "object"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case *Function:
		return "function"
	default:
		return "object"
	}
}

// IsNullish reports whether v is undefined or null.
func IsNullish(v Value) bool {
	switch v.(type) {
	case UndefinedType, NullType:
		return true
	}
	return false
}

// Truthy implements ToBoolean.
func Truthy(v Value) bool {
	switch x := v.(type) {
	case UndefinedType, NullType:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	default:
		return true
	}
}
