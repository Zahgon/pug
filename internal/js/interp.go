package js

import (
	"math"
	"strings"
)

// Binding is a variable.
type Binding struct {
	Value Value
	// Const marks a const binding (the loop variable of each ... of).
	Const bool
}

// Resolver supplies the variables that no enclosing scope declares.
type Resolver interface {
	// Free returns the binding of a free variable; ok false means the name is not defined (a ReferenceError).
	Free(name string) (b *Binding, ok bool)
	// Define creates the binding an assignment to an undefined free variable makes.
	Define(name string, v Value)
}

// Scope is a lexical environment: a function scope, or a block scope for const bindings.
type Scope struct {
	vars     map[string]*Binding
	parent   *Scope
	resolver Resolver
	function bool
	// This is the receiver of the function the scope belongs to.
	This Value
}

// NewRoot returns the outermost function scope of a template; free variables come from r.
func NewRoot(r Resolver) *Scope {
	return &Scope{vars: map[string]*Binding{}, resolver: r, function: true, This: Undefined}
}

// NewFunction returns a function scope nested in s.
func (s *Scope) NewFunction() *Scope {
	return &Scope{vars: map[string]*Binding{}, parent: s, resolver: s.resolver, function: true, This: Undefined}
}

// NewBlock returns a block scope nested in s.
func (s *Scope) NewBlock() *Scope {
	return &Scope{vars: map[string]*Binding{}, parent: s, resolver: s.resolver, This: s.This}
}

// Declare creates a var binding in the nearest function scope, initialised to undefined unless it exists.
func (s *Scope) Declare(name string) {
	f := s
	for !f.function {
		f = f.parent
	}
	if _, ok := f.vars[name]; !ok {
		f.vars[name] = &Binding{Value: Undefined}
	}
}

// Let creates a binding in this scope with the value v.
func (s *Scope) Let(name string, v Value, constant bool) {
	s.vars[name] = &Binding{Value: v, Const: constant}
}

// Set assigns to the variable name as an assignment expression does.
func (s *Scope) Set(name string, v Value) error {
	if b, ok := s.lookup(name); ok {
		if b.Const {
			return typeError("Assignment to constant variable.")
		}
		b.Value = v
		return nil
	}
	s.resolver.Define(name, v)
	return nil
}

func (s *Scope) lookup(name string) (*Binding, bool) {
	for sc := s; sc != nil; sc = sc.parent {
		if b, ok := sc.vars[name]; ok {
			return b, true
		}
	}
	return s.resolver.Free(name)
}

// Get reads the variable name.
func (s *Scope) Get(name string) (Value, error) {
	if b, ok := s.lookup(name); ok {
		return b.Value, nil
	}
	return nil, Throw("ReferenceError", "%s is not defined", name)
}

// Hoist declares, in the function scope of s, every var that the statements declare.
func (s *Scope) Hoist(stmts []Node) {
	for _, name := range VarNames(stmts) {
		s.Declare(name)
	}
}

// VarNames returns the names that var declarations in stmts declare (functions are not part of the subset, so
// every nested statement belongs to the same function).
func VarNames(stmts []Node) []string {
	var names []string
	var walk func(n Node)
	walk = func(n Node) {
		switch x := n.(type) {
		case *VarDecl:
			for _, d := range x.Decls {
				names = append(names, d.Name)
			}
		case *IfStmt:
			walk(x.Then)
			if x.Else != nil {
				walk(x.Else)
			}
		case *ForStmt:
			if x.Init != nil {
				walk(x.Init)
			}
			walk(x.Body)
		case *WhileStmt:
			walk(x.Body)
		case *BlockStmt:
			for _, b := range x.Body {
				walk(b)
			}
		}
	}
	for _, st := range stmts {
		walk(st)
	}
	return names
}

// Exec runs statements.
func (s *Scope) Exec(stmts []Node) error {
	for _, st := range stmts {
		if err := s.exec(st); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scope) exec(n Node) error {
	switch x := n.(type) {
	case *EmptyStmt:
		return nil
	case *ExprStmt:
		_, err := s.Eval(x.X)
		return err
	case *VarDecl:
		// The names were hoisted when the enclosing function was entered; a declaration only assigns.
		for _, d := range x.Decls {
			if d.Init == nil {
				continue
			}
			v, err := s.Eval(d.Init)
			if err != nil {
				return err
			}
			if err := s.Set(d.Name, v); err != nil {
				return err
			}
		}
		return nil
	case *BlockStmt:
		return s.Exec(x.Body)
	case *IfStmt:
		t, err := s.Eval(x.Test)
		if err != nil {
			return err
		}
		if Truthy(t) {
			return s.exec(x.Then)
		}
		if x.Else != nil {
			return s.exec(x.Else)
		}
		return nil
	case *WhileStmt:
		for {
			t, err := s.Eval(x.Test)
			if err != nil {
				return err
			}
			if !Truthy(t) {
				return nil
			}
			if err := s.exec(x.Body); err != nil {
				return err
			}
		}
	case *ForStmt:
		return s.For(x.Init, x.Test, x.Update, func() error { return s.exec(x.Body) })
	}
	return Throw("SyntaxError", "Unsupported statement")
}

// For runs a for loop whose body is the callback.
func (s *Scope) For(init, test, update Node, body func() error) error {
	if init != nil {
		if err := s.exec(init); err != nil {
			return err
		}
	}
	for {
		if test != nil {
			t, err := s.Eval(test)
			if err != nil {
				return err
			}
			if !Truthy(t) {
				return nil
			}
		}
		if err := body(); err != nil {
			return err
		}
		if update != nil {
			if _, err := s.Eval(update); err != nil {
				return err
			}
		}
	}
}

// Eval evaluates an expression.
func (s *Scope) Eval(n Node) (Value, error) {
	switch x := n.(type) {
	case *NumberLit:
		return x.Value, nil
	case *StringLit:
		return x.Value, nil
	case *BoolLit:
		return x.Value, nil
	case *NullLit:
		return Null, nil
	case *ThisExpr:
		return s.This, nil
	case *Paren:
		return s.Eval(x.X)
	case *TemplateLit:
		parts := []string{x.Quasis[0]}
		for i, e := range x.Exprs {
			v, err := s.Eval(e)
			if err != nil {
				return nil, err
			}
			str, err := ToString(v)
			if err != nil {
				return nil, err
			}
			parts = append(parts, str, x.Quasis[i+1])
		}
		return Concat(parts...), nil
	case *Ident:
		if x.Name == "undefined" {
			if b, ok := s.lookupLocal("undefined"); ok {
				return b.Value, nil
			}
			return Undefined, nil
		}
		return s.Get(x.Name)
	case *ArrayLit:
		arr := NewArray()
		for _, e := range x.Elems {
			v, err := s.Eval(e)
			if err != nil {
				return nil, err
			}
			arr.Elems = append(arr.Elems, v)
		}
		return arr, nil
	case *ObjectLit:
		obj := NewObject()
		for _, p := range x.Props {
			key := p.Key
			if p.Computed {
				k, err := s.Eval(p.KeyExpr)
				if err != nil {
					return nil, err
				}
				if key, err = ToPropertyKey(k); err != nil {
					return nil, err
				}
			}
			v, err := s.Eval(p.Value)
			if err != nil {
				return nil, err
			}
			if key == "__proto__" && !p.Computed {
				// {__proto__: v} sets the prototype rather than an own property.
				continue
			}
			obj.Set(key, v)
		}
		return obj, nil
	case *Member:
		obj, err := s.Eval(x.Object)
		if err != nil {
			return nil, err
		}
		key, err := s.memberKey(x)
		if err != nil {
			return nil, err
		}
		return GetMember(obj, key)
	case *Call:
		return s.call(x)
	case *Unary:
		if x.Op == "typeof" {
			if id, ok := x.X.(*Ident); ok {
				if b, ok := s.lookup(id.Name); ok {
					return TypeOf(b.Value), nil
				}
				return "undefined", nil
			}
		}
		v, err := s.Eval(x.X)
		if err != nil {
			return nil, err
		}
		switch x.Op {
		case "!":
			return !Truthy(v), nil
		case "-":
			n, err := ToNumeric(v)
			return -n, err
		case "+":
			return ToNumeric(v)
		case "typeof":
			return TypeOf(v), nil
		case "void":
			return Undefined, nil
		}
	case *Binary:
		l, err := s.Eval(x.L)
		if err != nil {
			return nil, err
		}
		r, err := s.Eval(x.R)
		if err != nil {
			return nil, err
		}
		return BinaryOp(x.Op, l, r)
	case *Logical:
		l, err := s.Eval(x.L)
		if err != nil {
			return nil, err
		}
		if (x.Op == "&&") != Truthy(l) {
			return l, nil
		}
		return s.Eval(x.R)
	case *Conditional:
		t, err := s.Eval(x.Test)
		if err != nil {
			return nil, err
		}
		if Truthy(t) {
			return s.Eval(x.Then)
		}
		return s.Eval(x.Else)
	case *Sequence:
		var v Value = Undefined
		for _, e := range x.Exprs {
			var err error
			if v, err = s.Eval(e); err != nil {
				return nil, err
			}
		}
		return v, nil
	case *Assign:
		return s.assign(x)
	case *Update:
		return s.update(x)
	}
	return nil, Throw("SyntaxError", "Unsupported expression")
}

func (s *Scope) lookupLocal(name string) (*Binding, bool) {
	for sc := s; sc != nil; sc = sc.parent {
		if b, ok := sc.vars[name]; ok {
			return b, true
		}
	}
	return nil, false
}

func (s *Scope) memberKey(m *Member) (string, error) {
	if !m.Computed {
		return m.Property.(*StringLit).Value, nil
	}
	k, err := s.Eval(m.Property)
	if err != nil {
		return "", err
	}
	return ToPropertyKey(k)
}

// BinaryOp applies a binary operator to two values.
func BinaryOp(op string, l, r Value) (Value, error) {
	switch op {
	case "+":
		return Add(l, r)
	case "-", "*", "/", "%":
		return Arith(op, l, r)
	case "===":
		return StrictEquals(l, r), nil
	case "!==":
		return !StrictEquals(l, r), nil
	case "==":
		return LooseEquals(l, r)
	case "!=":
		eq, err := LooseEquals(l, r)
		return !eq, err
	case "<", ">", "<=", ">=":
		a, b := l, r
		if op == ">" || op == "<=" {
			a, b = r, l
		}
		c, ok, err := Compare(a, b)
		if err != nil {
			return nil, err
		}
		if !ok {
			return false, nil
		}
		if op == "<" || op == ">" {
			return c < 0, nil
		}
		return c >= 0, nil
	}
	return nil, Throw("SyntaxError", "Unsupported operator %s", op)
}

func (s *Scope) call(c *Call) (Value, error) {
	var fn, this Value = nil, Undefined
	switch callee := c.Callee.(type) {
	case *Member:
		obj, err := s.Eval(callee.Object)
		if err != nil {
			return nil, err
		}
		key, err := s.memberKey(callee)
		if err != nil {
			return nil, err
		}
		if fn, err = GetMember(obj, key); err != nil {
			return nil, err
		}
		this = obj
	default:
		var err error
		if fn, err = s.Eval(c.Callee); err != nil {
			return nil, err
		}
	}
	args := make([]Value, 0, len(c.Args))
	for _, a := range c.Args {
		v, err := s.Eval(a)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	f, ok := fn.(*Function)
	if !ok {
		return nil, typeError("%s is not a function", CallSite(c.Callee))
	}
	return f.Call(this, args)
}

func (s *Scope) assign(a *Assign) (Value, error) {
	switch t := a.Target.(type) {
	case *Ident:
		v, err := s.Eval(a.Value)
		if a.Op != "=" {
			if err != nil {
				return nil, err
			}
			old, err := s.Get(t.Name)
			if err != nil {
				return nil, err
			}
			if v, err = BinaryOp(strings.TrimSuffix(a.Op, "="), old, v); err != nil {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
		return v, s.Set(t.Name, v)
	case *Member:
		obj, err := s.Eval(t.Object)
		if err != nil {
			return nil, err
		}
		key, err := s.memberKey(t)
		if err != nil {
			return nil, err
		}
		var old Value
		if a.Op != "=" {
			if old, err = GetMember(obj, key); err != nil {
				return nil, err
			}
		}
		if IsNullish(obj) && a.Op == "=" {
			// The target object is checked after the right-hand side is evaluated.
			if _, err := s.Eval(a.Value); err != nil {
				return nil, err
			}
			return nil, SetMember(obj, key, nil)
		}
		v, err := s.Eval(a.Value)
		if err != nil {
			return nil, err
		}
		if a.Op != "=" {
			if v, err = BinaryOp(strings.TrimSuffix(a.Op, "="), old, v); err != nil {
				return nil, err
			}
		}
		return v, SetMember(obj, key, v)
	}
	return nil, Throw("SyntaxError", "Invalid left-hand side in assignment")
}

func (s *Scope) update(u *Update) (Value, error) {
	delta := 1.0
	if u.Op == "--" {
		delta = -1
	}
	switch t := u.X.(type) {
	case *Ident:
		old, err := s.Get(t.Name)
		if err != nil {
			return nil, err
		}
		n, err := ToNumeric(old)
		if err != nil {
			return nil, err
		}
		if err := s.Set(t.Name, n+delta); err != nil {
			return nil, err
		}
		if u.Prefix {
			return n + delta, nil
		}
		return n, nil
	case *Member:
		obj, err := s.Eval(t.Object)
		if err != nil {
			return nil, err
		}
		key, err := s.memberKey(t)
		if err != nil {
			return nil, err
		}
		old, err := GetMember(obj, key)
		if err != nil {
			return nil, err
		}
		n, err := ToNumeric(old)
		if err != nil {
			return nil, err
		}
		if err := SetMember(obj, key, n+delta); err != nil {
			return nil, err
		}
		if u.Prefix {
			return n + delta, nil
		}
		return n, nil
	}
	return nil, Throw("SyntaxError", "Invalid left-hand side expression in postfix operation")
}

// CallSite renders an expression the way V8 names it in "is not a function" and "is not iterable" errors.
func CallSite(n Node) string {
	switch x := n.(type) {
	case *Ident:
		return x.Name
	case *Paren:
		return CallSite(x.X)
	case *NumberLit:
		return NumberToString(x.Value)
	case *StringLit:
		return `"` + x.Value + `"`
	case *BoolLit:
		if x.Value {
			return "true"
		}
		return "false"
	case *NullLit:
		return "null"
	case *ThisExpr:
		return "this"
	case *TemplateLit:
		var b strings.Builder
		for _, e := range x.Exprs {
			b.WriteString(CallSite(e))
		}
		return b.String()
	case *Member:
		obj := CallSite(x.Object)
		if !x.Computed {
			return obj + "." + x.Property.(*StringLit).Value
		}
		if k, ok := foldKey(x.Property); ok {
			if str, isStr := k.(string); isStr {
				return obj + "." + str
			}
			return obj + "[" + NumberToString(k.(float64)) + "]"
		}
		return obj + "[" + CallSite(x.Property) + "]"
	case *Call:
		return CallSite(x.Callee) + "(...)"
	case *Binary:
		if v, ok := foldNumbers(x); ok {
			return NumberToString(v)
		}
		return "(" + CallSite(x.L) + " " + x.Op + " " + CallSite(x.R) + ")"
	case *Logical:
		return "(" + CallSite(x.L) + " " + x.Op + " " + CallSite(x.R) + ")"
	case *Unary:
		if x.Op == "-" {
			if num, ok := x.X.(*NumberLit); ok {
				return NumberToString(-num.Value)
			}
		}
		if x.Op == "typeof" || x.Op == "void" {
			return "(" + x.Op + " " + CallSite(x.X) + ")"
		}
		return "(" + x.Op + CallSite(x.X) + ")"
	case *Sequence:
		parts := make([]string, len(x.Exprs))
		for i, e := range x.Exprs {
			parts[i] = CallSite(e)
		}
		return "(" + strings.Join(parts, " , ") + ")"
	case *ArrayLit:
		parts := make([]string, len(x.Elems))
		for i, e := range x.Elems {
			parts[i] = CallSite(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *ObjectLit:
		return "{" + strings.Repeat("(intermediate value)", len(x.Props)) + "}"
	case *Conditional:
		return "(intermediate value)(intermediate value)(intermediate value)"
	}
	return "(intermediate value)"
}

// foldKey returns the value of a computed member key that V8 folds at parse time: a string or number literal, a
// template literal without substitutions, or arithmetic on number literals.
func foldKey(n Node) (Value, bool) {
	switch x := n.(type) {
	case *StringLit:
		return x.Value, true
	case *NumberLit:
		return x.Value, true
	case *TemplateLit:
		if len(x.Exprs) == 0 {
			return x.Quasis[0], true
		}
	case *Unary:
		if num, ok := x.X.(*NumberLit); ok && x.Op == "-" {
			return -num.Value, true
		}
	case *Binary:
		if v, ok := foldNumbers(x); ok {
			return v, true
		}
	}
	return nil, false
}

func foldNumbers(b *Binary) (float64, bool) {
	l, ok1 := b.L.(*NumberLit)
	r, ok2 := b.R.(*NumberLit)
	if !ok1 || !ok2 {
		return 0, false
	}
	switch b.Op {
	case "+":
		return l.Value + r.Value, true
	case "-":
		return l.Value - r.Value, true
	case "*":
		return l.Value * r.Value, true
	case "/":
		return l.Value / r.Value, true
	case "%":
		if math.IsInf(r.Value, 0) {
			return l.Value, true
		}
		return math.Mod(l.Value, r.Value), true
	}
	return 0, false
}
