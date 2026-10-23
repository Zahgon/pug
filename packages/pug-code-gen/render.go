// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-code-gen); MIT licence, see LICENSE.

package codegen

import (
	"errors"
	"strings"

	runtime "github.com/pugjs/pug/packages/pug-runtime"

	"github.com/pugjs/pug/internal/js"
)

// output accumulates the rendered JavaScript string.
type output struct {
	parts []string
}

func (o *output) add(s string) {
	if s != "" {
		o.parts = append(o.parts, s)
	}
}

// String returns the rendered text.
func (o *output) String() string {
	return js.Concat(o.parts...)
}

type mixin struct {
	def     *opMixinDef
	closure *js.Scope
}

type renderer struct {
	t       *Template
	out     output
	indent  []string
	mixins  map[string]*mixin
	line    int
	hasLine bool
	file    string
	hasFile bool
}

// resolver gives the free variables of a template their values: from the locals, else from the JavaScript
// globals, else undefined, as the original's with rewriting does. In self mode free variables are globals only.
type resolver struct {
	locals   *js.Object
	self     bool
	bindings map[string]*js.Binding
}

// Free resolves a free variable.
func (r *resolver) Free(name string) (*js.Binding, bool) {
	if b, ok := r.bindings[name]; ok {
		return b, true
	}
	if r.self {
		v, ok := js.Global(name)
		if !ok {
			return nil, false
		}
		b := &js.Binding{Value: v}
		r.bindings[name] = b
		return b, true
	}
	var v js.Value = js.Undefined
	if val, ok := r.locals.Get(name); ok {
		v = val
	} else if name == "locals" {
		v = r.locals
	} else if g, ok := js.Global(name); ok {
		v = g
	}
	b := &js.Binding{Value: v}
	r.bindings[name] = b
	return b, true
}

// Define creates a variable assigned without a declaration.
func (r *resolver) Define(name string, v js.Value) {
	r.bindings[name] = &js.Binding{Value: v}
}

// Render renders the template with the locals.
func (t *Template) Render(locals *js.Object) (string, error) {
	r := &renderer{t: t, mixins: map[string]*mixin{}}
	res := &resolver{locals: locals, self: t.self, bindings: map[string]*js.Binding{}}
	root := js.NewRoot(res)
	if t.self {
		root.Let("locals", locals, false)
		root.Let("self", locals, false)
	}
	if t.self {
		for _, name := range t.hoisted {
			root.Declare(name)
		}
	}
	// Otherwise the template's top-level var declarations are not function-local in the generated code: the with
	// rewriting turns them into parameters initialised from the locals, which the resolver provides.
	if err := r.exec(t.ops, root); err != nil {
		var je *js.Error
		if t.debug && errors.As(err, &je) {
			return "", runtime.Rethrow(je, r.file, r.hasFile, r.line, r.hasLine)
		}
		return "", err
	}
	return r.out.String(), nil
}

func (r *renderer) exec(ops []op, sc *js.Scope) error {
	for _, o := range ops {
		if err := r.step(o, sc); err != nil {
			return err
		}
	}
	return nil
}

// str converts a value appended to the output (pug_html + value).
func str(v js.Value) (string, error) {
	s, err := js.Add("", v)
	if err != nil {
		return "", err
	}
	return s.(string), nil
}

func (r *renderer) step(o op, sc *js.Scope) error {
	switch x := o.(type) {
	case opText:
		r.out.add(x.s)
	case opRuntimeIndent:
		r.out.add(strings.Join(r.indent, ""))
	case opDebug:
		r.line, r.hasLine = x.line, true
		if x.hasFile {
			r.file, r.hasFile = x.file, true
		}
	case opValue:
		v, err := sc.Eval(x.expr)
		if err != nil {
			return err
		}
		if x.code {
			if js.IsNullish(v) {
				v = ""
			}
			if x.escape {
				if v, err = runtime.Escape(v); err != nil {
					return err
				}
			}
		}
		s, err := str(v)
		if err != nil {
			return err
		}
		r.out.add(s)
	case opAttrs:
		s, err := renderAttrs(x.plan, sc)
		if err != nil {
			return err
		}
		r.out.add(s)
	case opStatements:
		return sc.Exec(x.stmts)
	case opPlainBlock:
		return r.exec(x.body, sc)
	case *opIf:
		t, err := sc.Eval(x.test)
		if err != nil {
			return err
		}
		if js.Truthy(t) {
			return r.exec(x.then, sc)
		}
		return r.exec(x.els, sc)
	case opFor:
		return sc.For(x.init, x.test, x.update, func() error { return r.exec(x.body, sc) })
	case opWhile:
		for {
			t, err := sc.Eval(x.test)
			if err != nil {
				return err
			}
			if !js.Truthy(t) {
				return nil
			}
			if err := r.exec(x.body, sc); err != nil {
				return err
			}
		}
	case opSwitch:
		return r.execSwitch(x, sc)
	case opEach:
		return r.execEach(x, sc)
	case opEachOf:
		return r.execEachOf(x, sc)
	case opMixinDef:
		def := x
		r.mixins[x.name] = &mixin{def: &def, closure: sc}
	case opMixinCall:
		return r.execMixinCall(x, sc)
	case opMixinBlock:
		return r.execMixinBlock(x, sc)
	}
	return nil
}

func (r *renderer) execSwitch(x opSwitch, sc *js.Scope) error {
	d, err := sc.Eval(x.disc)
	if err != nil {
		return err
	}
	start := -1
	for i, cl := range x.clauses {
		if cl.test == nil {
			continue
		}
		v, err := sc.Eval(cl.test)
		if err != nil {
			return err
		}
		if js.StrictEquals(d, v) {
			start = i
			break
		}
	}
	if start < 0 {
		for i, cl := range x.clauses {
			if cl.test == nil {
				start = i
				break
			}
		}
	}
	if start < 0 {
		return nil
	}
	for _, cl := range x.clauses[start:] {
		if err := r.exec(cl.body, sc); err != nil {
			return err
		}
		if cl.hasBody {
			return nil
		}
	}
	return nil
}

func (r *renderer) execEach(x opEach, outer *js.Scope) error {
	sc := outer.NewFunction()
	sc.This = outer.This
	sc.Declare(x.val)
	sc.Declare(x.key)
	for _, name := range x.hoisted {
		sc.Declare(name)
	}
	obj, err := sc.Eval(x.obj)
	if err != nil {
		return err
	}
	length, err := js.GetMember(obj, "length")
	if err != nil {
		return err
	}
	if n, ok := length.(float64); ok {
		if x.hasAlt && !js.Truthy(n) {
			return r.exec(x.arrAlt, sc)
		}
		// for (var key = 0, $$l = $$obj.length; key < $$l; key++): the index is a variable of the loop's function,
		// so code in the body that assigns it changes the iteration.
		if err := sc.Set(x.key, 0.0); err != nil {
			return err
		}
		for {
			cur, err := sc.Get(x.key)
			if err != nil {
				return err
			}
			c, ok, err := js.Compare(cur, n)
			if err != nil {
				return err
			}
			if !ok || c >= 0 {
				return nil
			}
			key, err := js.ToPropertyKey(cur)
			if err != nil {
				return err
			}
			v, err := js.GetMember(obj, key)
			if err != nil {
				return err
			}
			if err := sc.Set(x.val, v); err != nil {
				return err
			}
			if err := r.exec(x.arrBody, sc); err != nil {
				return err
			}
			if cur, err = sc.Get(x.key); err != nil {
				return err
			}
			num, err := js.ToNumeric(cur)
			if err != nil {
				return err
			}
			if err := sc.Set(x.key, num+1); err != nil {
				return err
			}
		}
	}
	count := 0
	for _, key := range runtime.ForIn(obj) {
		if !stillHas(obj, key) {
			continue
		}
		count++
		if err := sc.Set(x.key, key); err != nil {
			return err
		}
		v, err := js.GetMember(obj, key)
		if err != nil {
			return err
		}
		if err := sc.Set(x.val, v); err != nil {
			return err
		}
		if err := r.exec(x.objBody, sc); err != nil {
			return err
		}
	}
	if x.hasAlt && count == 0 {
		return r.exec(x.objAlt, sc)
	}
	return nil
}

// stillHas reports whether a key enumerated at the start of a for-in loop is still present (deleted keys are
// skipped).
func stillHas(obj js.Value, key string) bool {
	if o, ok := obj.(*js.Object); ok {
		return o.Has(key)
	}
	return true
}

func (r *renderer) execEachOf(x opEachOf, sc *js.Scope) error {
	obj, err := sc.Eval(x.obj)
	if err != nil {
		return err
	}
	next, err := iterate(obj, x.obj)
	if err != nil {
		return err
	}
	for {
		v, ok := next()
		if !ok {
			return nil
		}
		block := sc.NewBlock()
		if x.pair != nil {
			pairNext, err := iterate(v, nil)
			if err != nil {
				return err
			}
			a, ok := pairNext()
			if !ok {
				a = js.Undefined
			}
			b, ok := pairNext()
			if !ok {
				b = js.Undefined
			}
			block.Let(x.pair[0], a, true)
			block.Let(x.pair[1], b, true)
		} else {
			block.Let(x.val, v, true)
		}
		if err := r.exec(x.body, block); err != nil {
			return err
		}
	}
}

// iterate returns an iterator over an array (live, by index) or a string (by code point); other values are not
// iterable. src is the iterated expression, for the error message (nil for destructuring).
func iterate(v js.Value, src js.Node) (func() (js.Value, bool), error) {
	switch x := v.(type) {
	case *js.Array:
		i := 0
		return func() (js.Value, bool) {
			if i >= len(x.Elems) {
				return nil, false
			}
			i++
			return x.Elems[i-1], true
		}, nil
	case string:
		u := js.ToUTF16(x)
		i := 0
		return func() (js.Value, bool) {
			if i >= len(u) {
				return nil, false
			}
			n := 1
			if u[i] >= 0xD800 && u[i] <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF {
				n = 2
			}
			i += n
			return js.FromUTF16(u[i-n : i]), true
		}, nil
	}
	if src == nil {
		return nil, &js.Error{Name: "TypeError", Message: ".for is not iterable"}
	}
	if _, ok := src.(*js.Conditional); ok {
		return nil, &js.Error{Name: "TypeError", Message: js.TypeOf(v) + " is not iterable (cannot read property Symbol(Symbol.iterator))"}
	}
	return nil, &js.Error{Name: "TypeError", Message: js.CallSite(src) + " is not iterable"}
}

func (r *renderer) mixinCallee(x opMixinCall, sc *js.Scope) (*mixin, string, error) {
	if x.dynamic == nil {
		return r.mixins[x.name], "pug_mixins." + x.name, nil
	}
	k, err := sc.Eval(x.dynamic)
	if err != nil {
		return nil, "", err
	}
	key, err := js.ToPropertyKey(k)
	if err != nil {
		return nil, "", err
	}
	site := "pug_mixins[" + js.CallSite(x.dynamic) + "]"
	if lit, ok := x.dynamic.(*js.StringLit); ok {
		site = "pug_mixins." + lit.Value
	}
	return r.mixins[key], site, nil
}

func (r *renderer) execMixinCall(x opMixinCall, sc *js.Scope) error {
	if x.pretty {
		r.indent = append(r.indent, x.indent)
	}
	m, site, err := r.mixinCallee(x, sc)
	if err != nil {
		return err
	}
	var this js.Value = js.Undefined
	if x.viaCall {
		if m == nil {
			return &js.Error{Name: "TypeError", Message: "Cannot read properties of undefined (reading 'call')"}
		}
		obj := js.NewObject()
		if x.hasBlock {
			caller := sc
			obj.Set("block", &js.Function{Name: "block", Call: func(js.Value, []js.Value) (js.Value, error) {
				bs := caller.NewFunction()
				for _, name := range x.blockVar {
					bs.Declare(name)
				}
				return js.Undefined, r.exec(x.block, bs)
			}})
		}
		if x.attrs != nil {
			v, err := attrsValue(x.attrs, sc)
			if err != nil {
				return err
			}
			obj.Set("attributes", v)
		}
		this = obj
	}
	args := make([]js.Value, 0, len(x.args))
	for _, a := range x.args {
		v, err := sc.Eval(a)
		if err != nil {
			return err
		}
		args = append(args, v)
	}
	if m == nil {
		return &js.Error{Name: "TypeError", Message: site + " is not a function"}
	}
	if err := r.callMixin(m, this, args); err != nil {
		return err
	}
	if x.pretty {
		r.indent = r.indent[:len(r.indent)-1]
	}
	return nil
}

func (r *renderer) callMixin(m *mixin, this js.Value, args []js.Value) error {
	d := m.def
	sc := m.closure.NewFunction()
	sc.This = this
	for i, p := range d.params {
		var v js.Value = js.Undefined
		if i < len(args) {
			v = args[i]
		}
		if _, undef := v.(js.UndefinedType); undef && p.def != nil {
			var err error
			sc.Let(p.name, js.Undefined, false)
			if v, err = sc.Eval(p.def); err != nil {
				return err
			}
		}
		sc.Let(p.name, v, false)
	}
	var block, attributes js.Value = js.Undefined, js.Undefined
	if js.Truthy(this) {
		block, _ = js.GetMember(this, "block")
		attributes, _ = js.GetMember(this, "attributes")
	}
	if !js.Truthy(attributes) {
		attributes = js.NewObject()
	}
	sc.Declare("block")
	sc.Declare("attributes")
	sc.Set("block", block)
	sc.Set("attributes", attributes)
	if d.hasRest {
		rest := js.NewArray()
		for i := len(d.params); i < len(args); i++ {
			rest.Elems = append(rest.Elems, args[i])
		}
		sc.Declare(d.rest)
		sc.Set(d.rest, rest)
	}
	for _, name := range d.hoisted {
		sc.Declare(name)
	}
	return r.exec(d.body, sc)
}

func (r *renderer) execMixinBlock(x opMixinBlock, sc *js.Scope) error {
	if x.pretty {
		r.indent = append(r.indent, x.indent)
	}
	b, err := sc.Get("block")
	if err != nil {
		return err
	}
	if js.Truthy(b) {
		f, ok := b.(*js.Function)
		if !ok {
			return &js.Error{Name: "TypeError", Message: "block is not a function"}
		}
		if _, err := f.Call(js.Undefined, nil); err != nil {
			return err
		}
	}
	if x.pretty {
		r.indent = r.indent[:len(r.indent)-1]
	}
	return nil
}

// renderAttrs renders an attribute plan; sc is nil for plans folded at compile time.
func renderAttrs(p *attrPlan, sc *js.Scope) (string, error) {
	if p.parts == nil && (p.object != nil || p.blocks != nil) {
		v, err := attrsValue(p, sc)
		if err != nil {
			return "", err
		}
		return runtime.Attrs(v, p.terse)
	}
	var b []string
	for _, part := range p.parts {
		if part.isStatic {
			b = append(b, part.static)
			continue
		}
		var v js.Value
		var err error
		switch {
		case part.isClass:
			if v, err = classValue(part.class, sc); err != nil {
				return "", err
			}
			s, err := runtime.Attr("class", v, false, p.terse)
			if err != nil {
				return "", err
			}
			b = append(b, s)
			continue
		default:
			if v, err = sc.Eval(part.expr); err != nil {
				return "", err
			}
			if part.style {
				s, err := runtime.Style(v)
				if err != nil {
					return "", err
				}
				v = s
			}
		}
		s, err := runtime.Attr(part.key, v, part.mustEscape, p.terse)
		if err != nil {
			return "", err
		}
		b = append(b, s)
	}
	return js.Concat(b...), nil
}

func eval(sc *js.Scope, n js.Node) (js.Value, error) {
	if sc == nil {
		v, _, err := js.ToConstant(n)
		return v, err
	}
	return sc.Eval(n)
}

func classValue(items []classItem, sc *js.Scope) (js.Value, error) {
	arr := js.NewArray()
	escaping := make([]bool, len(items))
	for i, it := range items {
		v := it.value
		if !it.isStatic {
			var err error
			if v, err = eval(sc, it.expr); err != nil {
				return nil, err
			}
		}
		arr.Elems = append(arr.Elems, v)
		escaping[i] = it.escape
	}
	return runtime.Classes(arr, escaping)
}

// attrsValue builds the attributes object of a plan in object form and merges the &attributes objects into it.
func attrsValue(p *attrPlan, sc *js.Scope) (js.Value, error) {
	var list []js.Value
	if p.hasObj {
		obj := js.NewObject()
		for _, e := range p.object {
			var v js.Value
			var err error
			switch {
			case e.isStatic:
				v = e.value
			case e.isClass:
				v, err = classValue(e.class, sc)
			default:
				if v, err = eval(sc, e.expr); err == nil {
					if e.style {
						var s string
						s, err = runtime.Style(v)
						v = s
					}
					if err == nil && e.mustEscape {
						v, err = runtime.Escape(v)
					}
				}
			}
			if err != nil {
				return nil, err
			}
			obj.Set(e.key, v)
		}
		list = append(list, obj)
	}
	for _, b := range p.blocks {
		v, err := eval(sc, b)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	if len(list) == 1 {
		return list[0], nil
	}
	return runtime.Merge(list)
}
