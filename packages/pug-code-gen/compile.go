// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-code-gen); MIT licence, see LICENSE.

// Package codegen compiles a linked pug syntax tree into a template, following pug-code-gen: the same output for
// tags, attributes, text, comments, doctypes and pretty printing, the same compile-time constant folding and
// errors, and, at run time, the same JavaScript scoping of locals, var declarations, loops and mixins.
package codegen

import (
	"regexp"
	"strings"

	pugerror "github.com/pugjs/pug/packages/pug-error"
	parser "github.com/pugjs/pug/packages/pug-parser"
	runtime "github.com/pugjs/pug/packages/pug-runtime"

	"github.com/pugjs/pug/internal/js"
)

// Options are the compile options of the original that shape the generated code.
type Options struct {
	// Pretty is the indentation unit of pretty output; empty for compact output.
	Pretty string
	// PrettySet reports that the pretty option was given (true or a string).
	PrettySet bool
	// CompileDebug keeps line information so that runtime errors carry template context.
	CompileDebug bool
	// Doctype is the doctype option; HasDoctype tells an empty value apart from none.
	Doctype    string
	HasDoctype bool
	// Self reads locals through the variable self instead of resolving free variables from them.
	Self bool
}

// Template is a compiled template.
type Template struct {
	ops     []op
	hoisted []string
	self    bool
	debug   bool
}

var doctypes = map[string]string{
	"html":         "<!DOCTYPE html>",
	"xml":          `<?xml version="1.0" encoding="utf-8" ?>`,
	"transitional": `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">`,
	"strict":       `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`,
	"frameset":     `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Frameset//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-frameset.dtd">`,
	"1.1":          `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`,
	"basic":        `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML Basic 1.1//EN" "http://www.w3.org/TR/xhtml-basic/xhtml-basic11.dtd">`,
	"mobile":       `<!DOCTYPE html PUBLIC "-//WAPFORUM//DTD XHTML Mobile 1.2//EN" "http://www.openmobilealliance.org/tech/DTD/xhtml-mobile12.dtd">`,
	"plist":        `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`,
}

// voidElements are rendered self-closing outside XML mode.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true,
}

// whitespaceSensitive tags are not pretty printed inside.
var whitespaceSensitive = map[string]bool{"pre": true, "textarea": true}

var whitespaceOnly = regexp.MustCompile(`^[\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]+$`)

type function struct {
	vars []string
}

type compiler struct {
	o                  Options
	pp                 string
	debug              bool
	indents            int
	parentIndents      int
	terse, xml         bool
	doctype            string
	hasDoctype         bool
	hasCompiledDoctype bool
	hasCompiledTag     bool
	escapePrettyMode   bool
	eachCount          int
	out                *[]op
	fns                []*function
	// syntax is the first JavaScript syntax error met. The original only finds such errors once the whole
	// template is generated (when the generated function is parsed), so every other compile error comes first.
	syntax error
}

// Compile compiles a linked syntax tree.
func Compile(ast *parser.Node, o Options) (*Template, error) {
	c := &compiler{o: o, debug: o.CompileDebug}
	if o.PrettySet && o.Pretty != "" {
		c.pp = o.Pretty
		if !whitespaceOnly.MatchString(c.pp) {
			return nil, &js.Error{Name: "Error", Message: "The pretty parameter should either be a boolean or whitespace only string"}
		}
	}
	if o.HasDoctype && strings.ContainsAny(o.Doctype, "<>") {
		return nil, &js.Error{Name: "Error", Message: `Doctype can not contain "<" or ">"`}
	}
	if o.HasDoctype && o.Doctype != "" {
		c.setDoctype(o.Doctype)
	}
	var ops []op
	c.out = &ops
	top := &function{}
	c.fns = []*function{top}
	if err := c.visit(ast, nil); err != nil {
		return nil, err
	}
	if c.syntax != nil {
		return nil, c.syntax
	}
	return &Template{ops: ops, hoisted: top.vars, self: o.Self, debug: c.debug}, nil
}

func (c *compiler) emit(o op) {
	*c.out = append(*c.out, o)
}

// collect compiles the body f into a separate list of operations.
func (c *compiler) collect(f func() error) ([]op, error) {
	saved := c.out
	var ops []op
	c.out = &ops
	err := f()
	c.out = saved
	return ops, err
}

func (c *compiler) declare(names []string) {
	f := c.fns[len(c.fns)-1]
	f.vars = append(f.vars, names...)
}

func (c *compiler) setDoctype(name string) {
	if d, ok := doctypes[js.ToLowerCase(name)]; ok {
		c.doctype = d
	} else {
		c.doctype = "<!DOCTYPE " + name + ">"
	}
	c.hasDoctype = true
	c.terse = js.ToLowerCase(c.doctype) == "<!doctype html>"
	c.xml = strings.HasPrefix(c.doctype, "<?xml")
}

func (c *compiler) buffer(s string) {
	if s == "" {
		return
	}
	if n := len(*c.out); n > 0 {
		if t, ok := (*c.out)[n-1].(opText); ok {
			(*c.out)[n-1] = opText{s: js.Concat(t.s, s)}
			return
		}
	}
	c.emit(opText{s: s})
}

func (c *compiler) prettyIndent(offset int, newline bool) {
	nl := ""
	if newline {
		nl = "\n"
	}
	c.buffer(nl + repeat(c.pp, c.indents+offset-1))
	if c.parentIndents > 0 {
		c.emit(opRuntimeIndent{})
	}
}

// repeat is Array(n + 1).join(s): n copies of s, none for n below 1.
func repeat(s string, n int) string {
	if n < 1 {
		return ""
	}
	return strings.Repeat(s, n)
}

func (c *compiler) visit(n *parser.Node, parent *parser.Node) error {
	if c.debug && !n.NoDebug && n.Type != "Block" && n.Line != 0 {
		c.emit(opDebug{line: n.Line, file: n.Filename, hasFile: n.Filename != ""})
	}
	switch n.Type {
	case "Block", "NamedBlock":
		return c.visitBlock(n)
	case "Case":
		return c.visitCase(n)
	case "Mixin":
		return c.visitMixin(n)
	case "MixinBlock":
		c.emit(opMixinBlock{indent: repeat(c.pp, c.indents), pretty: c.pp != ""})
		return nil
	case "Doctype":
		c.visitDoctype(n)
		return nil
	case "Tag", "InterpolatedTag":
		return c.visitTag(n, n.Type == "InterpolatedTag")
	case "Text":
		c.buffer(n.Val)
		return nil
	case "Comment":
		if n.Buffer {
			if c.pp != "" {
				c.prettyIndent(1, true)
			}
			c.buffer("<!--" + n.Val + "-->")
		}
		return nil
	case "BlockComment":
		if !n.Buffer {
			return nil
		}
		if c.pp != "" {
			c.prettyIndent(1, true)
		}
		c.buffer("<!--" + n.Val)
		if err := c.visit(n.Block, n); err != nil {
			return err
		}
		if c.pp != "" {
			c.prettyIndent(1, true)
		}
		c.buffer("-->")
		return nil
	case "YieldBlock":
		return nil
	case "Code":
		return c.visitCode(n)
	case "Conditional":
		return c.visitConditional(n)
	case "While":
		test, err := c.expression(n.Test)
		if err != nil {
			return err
		}
		body, err := c.collect(func() error { return c.visit(n.Block, n) })
		if err != nil {
			return err
		}
		c.emit(opWhile{test: test, body: body})
		return nil
	case "Each":
		return c.visitEach(n)
	case "EachOf":
		return c.visitEachOf(n)
	}
	where := "A top-level node"
	if parent != nil {
		where = "A child of " + parent.Type
	}
	file := n.Filename
	if file == "" {
		file = "Pug"
	}
	msg := where + " (" + file + ":" + js.NumberToString(float64(n.Line)) + ") is of type " + n.Type +
		", which is not supported by pug-code-gen."
	switch n.Type {
	case "Filter":
		msg += " Please use pug-filters to preprocess this AST."
	case "Extends", "Include", "NamedBlock", "FileReference":
		msg += " Please use pug-linker to preprocess this AST."
	}
	return &js.Error{Name: "TypeError", Message: msg}
}

func (c *compiler) visitBlock(b *parser.Node) error {
	escape := c.escapePrettyMode
	pp := c.pp != ""
	nodes := b.Nodes
	if pp && len(nodes) > 1 && !escape && nodes[0].Type == "Text" && nodes[1].Type == "Text" {
		c.prettyIndent(1, true)
	}
	for i, n := range nodes {
		if pp && i > 0 && !escape && n.Type == "Text" && nodes[i-1].Type == "Text" && strings.HasSuffix(nodes[i-1].Val, "\n") {
			c.prettyIndent(1, false)
		}
		if err := c.visit(n, b); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) visitCase(n *parser.Node) error {
	disc, err := c.expression(n.Expr)
	if err != nil {
		return err
	}
	sw := opSwitch{disc: disc}
	if n.Block != nil {
		for _, w := range n.Block.Nodes {
			if w.Type != "When" {
				return &js.Error{Name: "SyntaxError", Message: "Unexpected token"}
			}
			cl := clause{}
			if w.Expr != "default" {
				if cl.test, err = c.expression(w.Expr); err != nil {
					return err
				}
			}
			if w.Block != nil {
				cl.hasBody = true
				if cl.body, err = c.collect(func() error { return c.visit(w.Block, w) }); err != nil {
					return err
				}
			}
			sw.clauses = append(sw.clauses, cl)
		}
	}
	c.emit(sw)
	return nil
}

func (c *compiler) visitDoctype(n *parser.Node) {
	if n != nil && (n.Val != "" || !c.hasDoctype) {
		name := n.Val
		if name == "" {
			name = "html"
		}
		c.setDoctype(name)
	}
	if c.hasDoctype {
		c.buffer(c.doctype)
	}
	c.hasCompiledDoctype = true
}

// expression parses a JavaScript expression of the template; an invalid one is recorded and stands in as
// undefined until compilation ends.
func (c *compiler) expression(src string) (js.Node, error) {
	n, err := js.ParseExpression(src)
	if err != nil {
		c.syntaxError(err)
		return &js.Ident{Name: "undefined"}, nil
	}
	return n, nil
}

// syntaxError records the error the original reports for generated code that is not valid JavaScript: the with
// rewriting fails to parse it, or, in self mode, the Function constructor does.
func (c *compiler) syntaxError(err error) {
	if c.syntax != nil {
		return
	}
	if c.o.Self {
		c.syntax = &js.Error{Name: "SyntaxError", Message: err.Error()}
	} else {
		c.syntax = &js.Error{Name: "Error", Message: "Error parsing body of the with expression"}
	}
}

func (c *compiler) visitConditional(n *parser.Node) error {
	ifOp, err := c.conditional(n)
	if err != nil {
		return err
	}
	c.emit(ifOp)
	return nil
}

func (c *compiler) conditional(n *parser.Node) (*opIf, error) {
	test, err := c.expression(n.Test)
	if err != nil {
		return nil, err
	}
	then, err := c.collect(func() error { return c.visit(n.Consequent, n) })
	if err != nil {
		return nil, err
	}
	o := &opIf{test: test, then: then}
	if n.Alternate != nil {
		o.hasElse = true
		if n.Alternate.Type == "Conditional" {
			alt, err := c.conditional(n.Alternate)
			if err != nil {
				return nil, err
			}
			o.els = []op{alt}
		} else if o.els, err = c.collect(func() error { return c.visit(n.Alternate, n) }); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (c *compiler) visitCode(n *parser.Node) error {
	if n.Buffer {
		expr, err := c.expression(js.TrimSpace(n.Val))
		if err != nil {
			return err
		}
		c.emit(opValue{expr: expr, code: true, escape: n.MustEscape})
		if n.Block != nil {
			return c.visit(n.Block, n)
		}
		return nil
	}
	h, err := js.ParseHeader(n.Val)
	if err != nil {
		c.syntaxError(err)
		h = nil
	}
	if h != nil && n.Block != nil {
		return c.visitHeader(n, h)
	}
	stmts, err := js.ParseProgram(n.Val)
	if err != nil {
		c.syntaxError(err)
		stmts = nil
	}
	c.declare(js.VarNames(stmts))
	c.emit(opStatements{stmts: stmts})
	if n.Block != nil {
		body, err := c.collect(func() error { return c.visit(n.Block, n) })
		if err != nil {
			return err
		}
		c.emit(opPlainBlock{body: body})
	}
	return nil
}

// visitHeader compiles unbuffered code that opens a control statement (if, else if, else, for, while) whose body
// is the indented block.
func (c *compiler) visitHeader(n *parser.Node, h *js.Header) error {
	body, err := c.collect(func() error { return c.visit(n.Block, n) })
	if err != nil {
		return err
	}
	switch h.Kind {
	case "if":
		c.emit(&opIf{test: h.Test, then: body})
	case "else if", "else":
		last := c.pendingIf()
		if last == nil {
			c.syntaxError(&js.SyntaxError{Msg: "Unexpected token 'else'"})
			return nil
		}
		if h.Kind == "else" {
			last.els, last.hasElse = body, true
		} else {
			last.els, last.hasElse = []op{&opIf{test: h.Test, then: body}}, true
		}
	case "for":
		if h.Init != nil {
			c.declare(js.VarNames([]js.Node{h.Init}))
		}
		c.emit(opFor{init: h.Init, test: h.Test, update: h.Update, body: body})
	case "while":
		c.emit(opWhile{test: h.Test, body: body})
	}
	return nil
}

// pendingIf returns the innermost if of the chain the last operation ends with, when that chain has no else yet.
func (c *compiler) pendingIf() *opIf {
	n := len(*c.out)
	if n == 0 {
		return nil
	}
	o, ok := (*c.out)[n-1].(*opIf)
	if !ok {
		return nil
	}
	for o.hasElse {
		if len(o.els) != 1 {
			return nil
		}
		next, ok := o.els[0].(*opIf)
		if !ok {
			return nil
		}
		o = next
	}
	return o
}

func (c *compiler) visitEach(n *parser.Node) error {
	index := n.Key
	if !n.HasKey || index == "" {
		index = "pug_index" + js.NumberToString(float64(c.eachCount))
	}
	c.eachCount++
	obj, err := c.expression(n.Obj)
	if err != nil {
		return err
	}
	fn := &function{}
	c.fns = append(c.fns, fn)
	defer func() { c.fns = c.fns[:len(c.fns)-1] }()
	o := opEach{obj: obj, val: n.EachVal, key: index, hasAlt: n.Alternate != nil, indexVariable: index}
	if o.arrBody, err = c.collect(func() error { return c.visit(n.Block, n) }); err != nil {
		return err
	}
	if n.Alternate != nil {
		if o.arrAlt, err = c.collect(func() error { return c.visit(n.Alternate, n) }); err != nil {
			return err
		}
	}
	if o.objBody, err = c.collect(func() error { return c.visit(n.Block, n) }); err != nil {
		return err
	}
	if n.Alternate != nil {
		if o.objAlt, err = c.collect(func() error { return c.visit(n.Alternate, n) }); err != nil {
			return err
		}
	}
	o.hoisted = fn.vars
	c.emit(o)
	return nil
}

var pairNamesRe = regexp.MustCompile(`^\[ *([a-zA-Z_$][\w$]*) *\, *([a-zA-Z_$][\w$]*) *\]$`)

func (c *compiler) visitEachOf(n *parser.Node) error {
	obj, err := c.expression(n.Obj)
	if err != nil {
		return err
	}
	o := opEachOf{obj: obj}
	v := js.TrimSpace(n.EachVal)
	if m := pairNamesRe.FindStringSubmatch(v); m != nil {
		o.pair = []string{m[1], m[2]}
		o.conflict = m[1] == m[2]
	} else {
		o.val = v
	}
	if o.conflict {
		c.syntaxError(&js.SyntaxError{Msg: "Identifier '" + o.pair[0] + "' has already been declared"})
	}
	if o.body, err = c.collect(func() error { return c.visit(n.Block, n) }); err != nil {
		return err
	}
	c.emit(o)
	return nil
}

func (c *compiler) visitMixin(n *parser.Node) error {
	dynamic := strings.HasPrefix(n.Name, "#")
	if n.Call {
		return c.visitMixinCall(n, dynamic)
	}
	o := opMixinDef{name: n.Name}
	var pieces []string
	if n.HasArgs && n.Args != "" {
		pieces = strings.Split(n.Args, ",")
	}
	if len(pieces) > 0 && strings.HasPrefix(js.TrimSpace(pieces[len(pieces)-1]), "...") {
		o.rest = strings.TrimPrefix(js.TrimSpace(pieces[len(pieces)-1]), "...")
		o.hasRest = true
		pieces = pieces[:len(pieces)-1]
	}
	for _, piece := range pieces {
		name, def, hasDef := strings.Cut(piece, "=")
		name = js.TrimSpace(name)
		if !identifierRe.MatchString(name) {
			c.syntaxError(&js.SyntaxError{Msg: "Unexpected token"})
		}
		p := param{name: name}
		if hasDef {
			d, err := c.expression(def)
			if err != nil {
				return err
			}
			p.def = d
		}
		o.params = append(o.params, p)
	}
	fn := &function{}
	c.fns = append(c.fns, fn)
	c.parentIndents++
	body, err := c.collect(func() error { return c.visit(n.Block, n) })
	c.parentIndents--
	c.fns = c.fns[:len(c.fns)-1]
	if err != nil {
		return err
	}
	o.body = body
	o.hoisted = fn.vars
	c.emit(o)
	return nil
}

var identifierRe = regexp.MustCompile(`^[a-zA-Z_$][\w$]*$`)

func (c *compiler) visitMixinCall(n *parser.Node, dynamic bool) error {
	o := opMixinCall{name: n.Name, pretty: c.pp != ""}
	if dynamic {
		expr, err := c.expression(n.Name[2 : len(n.Name)-1])
		if err != nil {
			return err
		}
		o.dynamic = expr
	}
	if n.HasArgs && js.TrimSpace(n.Args) != "" {
		args, err := js.ParseArguments(n.Args)
		if err != nil {
			c.syntaxError(err)
		}
		o.args = args
	}
	if c.pp != "" {
		o.indent = repeat(c.pp, c.indents)
	}
	o.viaCall = n.Block != nil || len(n.Attrs) > 0 || len(n.AttrBlks) > 0
	if n.Block != nil {
		o.hasBlock = true
		fn := &function{}
		c.fns = append(c.fns, fn)
		c.parentIndents++
		saved := c.indents
		c.indents = 0
		body, err := c.collect(func() error { return c.visit(n.Block, n) })
		c.indents = saved
		c.parentIndents--
		c.fns = c.fns[:len(c.fns)-1]
		if err != nil {
			return err
		}
		o.block = body
		o.blockVar = fn.vars
	}
	if len(n.AttrBlks) > 0 || len(n.Attrs) > 0 {
		plan, err := c.objectPlan(n.Attrs, n.AttrBlks, false)
		if err != nil {
			return err
		}
		o.attrs = plan
	}
	c.emit(o)
	return nil
}

func (c *compiler) visitTag(n *parser.Node, interpolated bool) error {
	c.indents++
	defer func() { c.indents-- }()
	name := n.Name
	var nameExpr js.Node
	var nameText string
	nameIsText := !interpolated
	if interpolated {
		expr, err := c.expression(n.Expr)
		if err != nil {
			return err
		}
		v, constant, err := js.ToConstant(expr)
		if err != nil {
			return err
		}
		if constant {
			s, err := js.ToString(v)
			if err != nil {
				return err
			}
			nameText, nameIsText = s, true
		} else {
			nameExpr = expr
		}
	} else {
		nameText = name
	}
	bufferName := func() {
		if nameIsText {
			c.buffer(nameText)
		} else {
			c.emit(opValue{expr: nameExpr})
		}
	}
	sensitive := !interpolated && whitespaceSensitive[name]
	if sensitive {
		c.escapePrettyMode = true
	}
	if !c.hasCompiledTag {
		if !c.hasCompiledDoctype && !interpolated && name == "html" {
			c.visitDoctype(nil)
		}
		c.hasCompiledTag = true
	}
	pp := c.pp != ""
	if pp && !n.IsInline {
		c.prettyIndent(0, true)
	}
	if n.SelfClosing || (!c.xml && !interpolated && voidElements[name]) {
		c.buffer("<")
		bufferName()
		if err := c.visitAttributes(n.Attrs, n.AttrBlks); err != nil {
			return err
		}
		if c.terse && !n.SelfClosing {
			c.buffer(">")
		} else {
			c.buffer("/>")
		}
		if n.Block != nil && len(n.Block.Nodes) > 0 && hasContent(n.Block) {
			label := name
			if interpolated {
				label = "undefined"
			}
			return pugerror.New("SELF_CLOSING_CONTENT", label+" is a self closing element: <"+label+
				"/> but contains nested content.", pugerror.Options{Line: n.Line, Column: n.Column, Filename: n.Filename})
		}
	} else {
		c.buffer("<")
		bufferName()
		if err := c.visitAttributes(n.Attrs, n.AttrBlks); err != nil {
			return err
		}
		c.buffer(">")
		if err := c.visit(n.Block, n); err != nil {
			return err
		}
		if pp && !n.IsInline && !sensitive && !tagCanInline(n) {
			c.prettyIndent(0, true)
		}
		c.buffer("</")
		bufferName()
		c.buffer(">")
	}
	if sensitive {
		c.escapePrettyMode = false
	}
	return nil
}

var blankRe = regexp.MustCompile(`^[\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}]*$`)

// hasContent reports whether a self-closing tag's block holds anything but whitespace text.
func hasContent(b *parser.Node) bool {
	for _, n := range b.Nodes {
		if n.Type != "Text" || !blankRe.MatchString(n.Val) {
			return true
		}
	}
	return false
}

func tagCanInline(t *parser.Node) bool {
	var isInline func(n *parser.Node) bool
	isInline = func(n *parser.Node) bool {
		if n.Type == "Block" {
			for _, c := range n.Nodes {
				if !isInline(c) {
					return false
				}
			}
			return true
		}
		if n.Type == "YieldBlock" {
			return true
		}
		return (n.Type == "Text" && !strings.Contains(n.Val, "\n")) || n.IsInline
	}
	for _, n := range t.Block.Nodes {
		if !isInline(n) {
			return false
		}
	}
	return true
}

func (c *compiler) visitAttributes(attrs []parser.Attr, blocks []*parser.Node) error {
	if len(blocks) > 0 {
		plan, err := c.objectPlan(attrs, blocks, true)
		if err != nil {
			return err
		}
		if plan.constant() {
			s, err := renderAttrs(plan, nil)
			if err != nil {
				return err
			}
			c.buffer(s)
			return nil
		}
		c.emit(opAttrs{plan: plan})
		return nil
	}
	if len(attrs) == 0 {
		return nil
	}
	plan, err := c.htmlPlan(attrs)
	if err != nil {
		return err
	}
	if plan.constant() {
		for _, p := range plan.parts {
			c.buffer(p.static)
		}
		return nil
	}
	c.emit(opAttrs{plan: plan})
	return nil
}

// attrValue folds an attribute value: a bare attribute is the constant true.
func (c *compiler) attrValue(a parser.Attr) (js.Node, js.Value, bool, error) {
	if a.True {
		return nil, true, true, nil
	}
	expr, err := c.expression(a.Val)
	if err != nil {
		return nil, nil, false, err
	}
	v, constant, err := js.ToConstant(expr)
	if err != nil {
		return nil, nil, false, err
	}
	return expr, v, constant, nil
}

// classPlan compiles the class attributes of a tag into items for pug.classes.
func (c *compiler) classPlan(attrs []parser.Attr) ([]classItem, bool, error) {
	var items []classItem
	all := true
	for _, a := range attrs {
		if a.Name != "class" {
			continue
		}
		expr, v, constant, err := c.attrValue(a)
		if err != nil {
			return nil, false, err
		}
		items = append(items, classItem{value: v, expr: expr, isStatic: constant, escape: a.MustEscape})
		all = all && constant
	}
	return items, all, nil
}

func (c *compiler) htmlPlan(attrs []parser.Attr) (*attrPlan, error) {
	plan := &attrPlan{terse: c.terse}
	items, allConstant, err := c.classPlan(attrs)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		if allConstant {
			s, err := constantClasses(items)
			if err != nil {
				return nil, err
			}
			out, err := runtime.Attr("class", s, false, c.terse)
			if err != nil {
				return nil, err
			}
			plan.parts = append(plan.parts, attrPart{static: out, isStatic: true})
		} else {
			if err := preEscape(items); err != nil {
				return nil, err
			}
			plan.parts = append(plan.parts, attrPart{key: "class", class: items, isClass: true})
		}
	}
	for _, a := range attrs {
		if a.Name == "class" {
			continue
		}
		expr, v, constant, err := c.attrValue(a)
		if err != nil {
			return nil, err
		}
		if constant {
			if a.Name == "style" {
				s, err := runtime.Style(v)
				if err != nil {
					return nil, err
				}
				v = s
			}
			out, err := runtime.Attr(a.Name, v, a.MustEscape, c.terse)
			if err != nil {
				return nil, err
			}
			plan.parts = append(plan.parts, attrPart{static: out, isStatic: true})
			continue
		}
		plan.parts = append(plan.parts, attrPart{key: a.Name, expr: expr, mustEscape: a.MustEscape, style: a.Name == "style"})
	}
	return plan, nil
}

// constantClasses is pug.classes over constant items with their escaping flags.
func constantClasses(items []classItem) (js.Value, error) {
	arr := js.NewArray()
	escaping := make([]bool, len(items))
	for i, it := range items {
		arr.Elems = append(arr.Elems, it.value)
		escaping[i] = it.escape
	}
	return runtime.Classes(arr, escaping)
}

// preEscape escapes the constant items of a class list that mixes constants and expressions, as the original does
// before handing the list to pug.classes.
func preEscape(items []classItem) error {
	for i := range items {
		if items[i].isStatic {
			if items[i].escape {
				v, err := runtime.Escape(items[i].value)
				if err != nil {
					return err
				}
				items[i].value = v
			}
			items[i].escape = false
		}
	}
	return nil
}

func (c *compiler) objectPlan(attrs []parser.Attr, blocks []*parser.Node, asAttrs bool) (*attrPlan, error) {
	plan := &attrPlan{terse: c.terse, asAttrs: asAttrs, hasObj: len(attrs) > 0}
	items, allConstant, err := c.classPlan(attrs)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		if allConstant {
			s, err := constantClasses(items)
			if err != nil {
				return nil, err
			}
			plan.object = append(plan.object, attrEntry{key: "class", value: s, isStatic: true})
		} else {
			if err := preEscape(items); err != nil {
				return nil, err
			}
			plan.object = append(plan.object, attrEntry{key: "class", class: items, isClass: true})
		}
	}
	for _, a := range attrs {
		if a.Name == "class" {
			continue
		}
		expr, v, constant, err := c.attrValue(a)
		if err != nil {
			return nil, err
		}
		if constant {
			if a.Name == "style" {
				s, err := runtime.Style(v)
				if err != nil {
					return nil, err
				}
				v = s
			}
			if a.MustEscape {
				if v, err = runtime.Escape(v); err != nil {
					return nil, err
				}
			}
			plan.object = append(plan.object, attrEntry{key: a.Name, value: v, isStatic: true})
			continue
		}
		plan.object = append(plan.object, attrEntry{key: a.Name, expr: expr, mustEscape: a.MustEscape, style: a.Name == "style"})
	}
	for _, b := range blocks {
		expr, err := c.expression(b.Val)
		if err != nil {
			return nil, err
		}
		plan.blocks = append(plan.blocks, expr)
	}
	return plan, nil
}

// constant reports whether a plan needs nothing at run time; for object plans it also requires every
// &attributes expression to be constant, as the original's folding of pug.attrs(...) does.
func (p *attrPlan) constant() bool {
	for _, part := range p.parts {
		if !part.isStatic {
			return false
		}
	}
	for _, e := range p.object {
		if !e.isStatic {
			return false
		}
	}
	for _, b := range p.blocks {
		if _, ok, err := js.ToConstant(b); !ok && err == nil {
			return false
		}
	}
	return true
}
