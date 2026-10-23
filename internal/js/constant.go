package js

import "strings"

// Constant folding as the original's compile step does it (the constantinople package): an expression is constant
// when it is built only from literals and from operators and a fixed set of methods applied to constants. A
// constant expression is evaluated once at compile time, so an error it throws is a compile error.

// IsConstant reports whether src is a constant expression.
func IsConstant(src string) bool {
	n, err := ParseExpression(src)
	if err != nil {
		return false
	}
	_, ok, err := ToConstant(n)
	return ok || err != nil
}

// ToConstant evaluates a constant expression. ok is false when the expression is not constant; err is an error
// thrown while evaluating a constant expression.
func ToConstant(n Node) (v Value, ok bool, err error) {
	c := &folder{constant: true}
	v = c.fold(n)
	if c.err != nil {
		return nil, true, c.err
	}
	return v, c.constant, nil
}

type folder struct {
	constant bool
	err      error
}

func (c *folder) fail() Value {
	c.constant = false
	return Undefined
}

func (c *folder) check(v Value, err error) Value {
	if err != nil && c.err == nil {
		c.err = err
		c.constant = false
	}
	return v
}

func (c *folder) fold(n Node) Value {
	if !c.constant {
		return Undefined
	}
	switch x := n.(type) {
	case *ArrayLit:
		arr := NewArray()
		for _, e := range x.Elems {
			if !c.constant {
				break
			}
			arr.Elems = append(arr.Elems, c.fold(e))
		}
		return arr
	case *Binary:
		l := c.fold(x.L)
		r := c.fold(x.R)
		if !c.constant {
			return Undefined
		}
		v, err := BinaryOp(x.Op, l, r)
		return c.check(v, err)
	case *BoolLit:
		return x.Value
	case *Call:
		return c.foldCall(x)
	case *Conditional:
		if Truthy(c.fold(x.Test)) {
			return c.fold(x.Then)
		}
		return c.fold(x.Else)
	case *Logical:
		l := c.fold(x.L)
		r := c.fold(x.R)
		if !c.constant {
			return Undefined
		}
		if (x.Op == "&&") == Truthy(l) {
			return r
		}
		return l
	case *Member:
		obj := c.fold(x.Object)
		if !Truthy(obj) || !c.constant {
			return c.fail()
		}
		key, ok := c.memberName(x)
		if !c.constant {
			return Undefined
		}
		if !ok || !hasOwn(obj, key) || strings.HasPrefix(key, "_") {
			return c.fail()
		}
		v, err := GetMember(obj, key)
		return c.check(v, err)
	case *NullLit:
		return Null
	case *NumberLit:
		return x.Value
	case *StringLit:
		return x.Value
	case *ObjectLit:
		obj := NewObject()
		for _, p := range x.Props {
			if !c.constant {
				break
			}
			if p.shorthand {
				return c.fail()
			}
			key := p.Key
			if p.Computed {
				key = MustString(c.fold(p.KeyExpr))
			} else if p.numericKey {
				return c.fail()
			}
			if key == "" || strings.HasPrefix(key, "_") {
				return c.fail()
			}
			v := c.fold(p.Value)
			if !c.constant {
				return Undefined
			}
			obj.Set(key, v)
		}
		return obj
	case *Paren:
		return c.fold(x.X)
	case *Sequence:
		var v Value = Undefined
		for _, e := range x.Exprs {
			v = c.fold(e)
		}
		return v
	case *TemplateLit:
		parts := []string{x.Quasis[0]}
		for i, e := range x.Exprs {
			v := c.fold(e)
			if !c.constant {
				return Undefined
			}
			s, err := ToString(c.check(ToPrimitive(v)))
			parts = append(parts, MustString(c.check(s, err)), x.Quasis[i+1])
		}
		return Concat(parts...)
	case *Unary:
		v := c.fold(x.X)
		if !c.constant {
			return Undefined
		}
		switch x.Op {
		case "-":
			n, err := ToNumeric(v)
			return c.check(-n, err)
		case "+":
			n, err := ToNumeric(v)
			return c.check(n, err)
		case "!":
			return !Truthy(v)
		case "typeof":
			return TypeOf(v)
		case "void":
			return Undefined
		}
	}
	return c.fail()
}

func (c *folder) memberName(m *Member) (string, bool) {
	if !m.Computed {
		return m.Property.(*StringLit).Value, true
	}
	k := c.fold(m.Property)
	s, err := ToPropertyKey(k)
	return MustString(c.check(s, err)), err == nil
}

func hasOwn(obj Value, key string) bool {
	switch x := obj.(type) {
	case string:
		if key == "length" {
			return true
		}
		i, ok := ArrayIndex(key)
		return ok && int(i) < Length(x)
	case *Array:
		if key == "length" {
			return true
		}
		i, ok := ArrayIndex(key)
		return ok && int(i) < len(x.Elems)
	case *Object:
		return x.Has(key)
	}
	return false
}

// foldableMethods are the methods of primitive values that constant folding may call.
var foldableMethods = map[string]map[string]bool{
	"boolean": {"toString": true},
	"number":  {"toExponential": true, "toFixed": true, "toPrecision": true, "toString": true},
	"string": {"charAt": true, "charCodeAt": true, "codePointAt": true, "concat": true, "endsWith": true,
		"includes": true, "indexOf": true, "lastIndexOf": true, "match": true, "normalize": true, "padEnd": true,
		"padStart": true, "repeat": true, "replace": true, "search": true, "slice": true, "split": true,
		"startsWith": true, "substr": true, "substring": true, "toLowerCase": true, "toUpperCase": true, "trim": true},
}

func (c *folder) foldCall(call *Call) Value {
	var args []Value
	for _, a := range call.Args {
		if !c.constant {
			break
		}
		args = append(args, c.fold(a))
	}
	if !c.constant {
		return Undefined
	}
	m, ok := call.Callee.(*Member)
	if !ok {
		// Only the identifiers the compiler passes as constants could be called, and templates have none.
		c.fold(call.Callee)
		return c.fail()
	}
	obj := c.fold(m.Object)
	if !Truthy(obj) || !c.constant {
		return c.fail()
	}
	name, ok := c.memberName(m)
	if !c.constant || !ok {
		return c.fail()
	}
	allowed := false
	switch t := TypeOf(obj); t {
	case "boolean", "number", "string":
		allowed = foldableMethods[t][name]
	default:
		allowed = hasOwn(obj, name) && !strings.HasPrefix(name, "_")
	}
	if !allowed {
		return c.fail()
	}
	fn, err := GetMember(obj, name)
	if err != nil {
		return c.check(nil, err)
	}
	f, isFn := fn.(*Function)
	if !isFn {
		return c.check(nil, typeError("object[member].apply is not a function"))
	}
	v, err := f.Call(obj, args)
	return c.check(v, err)
}
