package js

import "fmt"

type parser struct {
	toks []token
	i    int
}

func newParser(src string) (*parser, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	return &parser{toks: toks}, nil
}

func (p *parser) peek() token {
	return p.toks[p.i]
}

func (p *parser) peekAt(n int) token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) advance() token {
	t := p.toks[p.i]
	if t.kind != tEOF {
		p.i++
	}
	return t
}

func (p *parser) is(text string) bool {
	t := p.peek()
	return (t.kind == tPunct || t.kind == tKeyword) && t.text == text
}

func (p *parser) accept(text string) bool {
	if p.is(text) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) fail(t token) error {
	if t.kind == tEOF {
		return &SyntaxError{Msg: "Unexpected end of input", Pos: t.pos}
	}
	return &SyntaxError{Msg: fmt.Sprintf("Unexpected token %s", t), Pos: t.pos}
}

func (p *parser) expect(text string) error {
	if !p.accept(text) {
		return p.fail(p.peek())
	}
	return nil
}

// ParseExpression parses src as one expression.
func ParseExpression(src string) (Node, error) {
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	n, err := p.expression()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.fail(t)
	}
	return n, nil
}

// IsExpression reports whether src is one expression of the supported subset.
func IsExpression(src string) bool {
	_, err := ParseExpression(src)
	return err == nil
}

// ParseArguments parses src as the inside of an argument list: zero or more expressions separated by commas.
func ParseArguments(src string) ([]Node, error) {
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	var args []Node
	if p.peek().kind == tEOF {
		return nil, nil
	}
	for {
		a, err := p.assign()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if p.peek().kind == tEOF {
			return args, nil
		}
		if err := p.expect(","); err != nil {
			return nil, err
		}
		if p.peek().kind == tEOF {
			return args, nil
		}
	}
}

// ParseProgram parses src as a list of statements.
func ParseProgram(src string) ([]Node, error) {
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	var body []Node
	for p.peek().kind != tEOF {
		s, err := p.statement()
		if err != nil {
			return nil, err
		}
		body = append(body, s)
	}
	return body, nil
}

// Header is the head of a control statement that pug completes with an indented block: if (test), else if (test),
// else, for (init; test; update) or while (test).
type Header struct {
	Kind   string
	Init   Node
	Test   Node
	Update Node
}

// ParseHeader recognises a control statement head without its body; it returns nil when src is not one.
func ParseHeader(src string) (*Header, error) {
	p, err := newParser(src)
	if err != nil {
		return nil, err
	}
	h := &Header{}
	switch {
	case p.is("if"):
		h.Kind = "if"
	case p.is("else"):
		p.advance()
		if p.peek().kind == tEOF {
			return &Header{Kind: "else"}, nil
		}
		if !p.is("if") {
			return nil, nil
		}
		h.Kind = "else if"
	case p.is("for"):
		h.Kind = "for"
	case p.is("while"):
		h.Kind = "while"
	default:
		return nil, nil
	}
	p.advance()
	if err := p.expect("("); err != nil {
		return nil, err
	}
	if h.Kind == "for" {
		if !p.is(";") {
			if p.is("var") {
				p.advance()
				d, err := p.declarators()
				if err != nil {
					return nil, err
				}
				h.Init = d
			} else {
				x, err := p.expression()
				if err != nil {
					return nil, err
				}
				h.Init = &ExprStmt{X: x}
			}
		}
		if err := p.expect(";"); err != nil {
			return nil, err
		}
		if !p.is(";") {
			if h.Test, err = p.expression(); err != nil {
				return nil, err
			}
		}
		if err := p.expect(";"); err != nil {
			return nil, err
		}
		if !p.is(")") {
			if h.Update, err = p.expression(); err != nil {
				return nil, err
			}
		}
	} else if h.Test, err = p.expression(); err != nil {
		return nil, err
	}
	if err := p.expect(")"); err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, nil
	}
	return h, nil
}

func (p *parser) statement() (Node, error) {
	switch {
	case p.accept(";"):
		return &EmptyStmt{}, nil
	case p.is("{"):
		p.advance()
		var body []Node
		for !p.is("}") {
			if p.peek().kind == tEOF {
				return nil, p.fail(p.peek())
			}
			s, err := p.statement()
			if err != nil {
				return nil, err
			}
			body = append(body, s)
		}
		p.advance()
		return &BlockStmt{Body: body}, nil
	case p.is("var"):
		p.advance()
		d, err := p.declarators()
		if err != nil {
			return nil, err
		}
		return d, p.semicolon()
	case p.is("if"):
		p.advance()
		if err := p.expect("("); err != nil {
			return nil, err
		}
		test, err := p.expression()
		if err != nil {
			return nil, err
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		then, err := p.statement()
		if err != nil {
			return nil, err
		}
		s := &IfStmt{Test: test, Then: then}
		if p.accept("else") {
			if s.Else, err = p.statement(); err != nil {
				return nil, err
			}
		}
		return s, nil
	case p.is("while"):
		p.advance()
		if err := p.expect("("); err != nil {
			return nil, err
		}
		test, err := p.expression()
		if err != nil {
			return nil, err
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		body, err := p.statement()
		if err != nil {
			return nil, err
		}
		return &WhileStmt{Test: test, Body: body}, nil
	case p.is("for"):
		p.advance()
		if err := p.expect("("); err != nil {
			return nil, err
		}
		s := &ForStmt{}
		if !p.is(";") {
			if p.accept("var") {
				d, err := p.declarators()
				if err != nil {
					return nil, err
				}
				s.Init = d
			} else {
				x, err := p.expression()
				if err != nil {
					return nil, err
				}
				s.Init = &ExprStmt{X: x}
			}
		}
		if err := p.expect(";"); err != nil {
			return nil, err
		}
		var err error
		if !p.is(";") {
			if s.Test, err = p.expression(); err != nil {
				return nil, err
			}
		}
		if err := p.expect(";"); err != nil {
			return nil, err
		}
		if !p.is(")") {
			if s.Update, err = p.expression(); err != nil {
				return nil, err
			}
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		if s.Body, err = p.statement(); err != nil {
			return nil, err
		}
		return s, nil
	case p.peek().kind == tKeyword && !p.isExpressionKeyword():
		return nil, p.fail(p.peek())
	}
	x, err := p.expression()
	if err != nil {
		return nil, err
	}
	return &ExprStmt{X: x}, p.semicolon()
}

func (p *parser) isExpressionKeyword() bool {
	switch p.peek().text {
	case "true", "false", "null", "this", "typeof", "void":
		return true
	}
	return false
}

// semicolon ends a statement: an explicit semicolon, or automatic insertion before a line break, a closing brace
// or the end of the input.
func (p *parser) semicolon() error {
	if p.accept(";") {
		return nil
	}
	t := p.peek()
	if t.kind == tEOF || t.nl || (t.kind == tPunct && t.text == "}") {
		return nil
	}
	return p.fail(t)
}

func (p *parser) declarators() (*VarDecl, error) {
	d := &VarDecl{}
	for {
		t := p.advance()
		if t.kind != tIdent {
			return nil, p.fail(t)
		}
		decl := Declarator{Name: t.text}
		if p.accept("=") {
			init, err := p.assign()
			if err != nil {
				return nil, err
			}
			decl.Init = init
		}
		d.Decls = append(d.Decls, decl)
		if !p.accept(",") {
			return d, nil
		}
	}
}

func (p *parser) expression() (Node, error) {
	x, err := p.assign()
	if err != nil {
		return nil, err
	}
	if !p.is(",") {
		return x, nil
	}
	seq := &Sequence{Exprs: []Node{x}}
	for p.accept(",") {
		y, err := p.assign()
		if err != nil {
			return nil, err
		}
		seq.Exprs = append(seq.Exprs, y)
	}
	return seq, nil
}

var assignOps = map[string]bool{"=": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true}

func isTarget(n Node) bool {
	switch n.(type) {
	case *Ident, *Member:
		return true
	}
	return false
}

func (p *parser) assign() (Node, error) {
	x, err := p.conditional()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tPunct && assignOps[t.text] {
		if !isTarget(x) {
			return nil, &SyntaxError{Msg: "Invalid left-hand side in assignment", Pos: t.pos}
		}
		p.advance()
		v, err := p.assign()
		if err != nil {
			return nil, err
		}
		return &Assign{Op: t.text, Target: x, Value: v}, nil
	}
	return x, nil
}

func (p *parser) conditional() (Node, error) {
	test, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	if !p.accept("?") {
		return test, nil
	}
	then, err := p.assign()
	if err != nil {
		return nil, err
	}
	if err := p.expect(":"); err != nil {
		return nil, err
	}
	els, err := p.assign()
	if err != nil {
		return nil, err
	}
	return &Conditional{Test: test, Then: then, Else: els}, nil
}

var binaryPrec = map[string]int{
	"||": 1, "&&": 2,
	"==": 6, "!=": 6, "===": 6, "!==": 6,
	"<": 7, ">": 7, "<=": 7, ">=": 7,
	"+": 9, "-": 9,
	"*": 10, "/": 10, "%": 10,
}

func (p *parser) binary(min int) (Node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		prec, ok := binaryPrec[t.text]
		if t.kind != tPunct || !ok || prec <= min {
			return left, nil
		}
		p.advance()
		right, err := p.binary(prec)
		if err != nil {
			return nil, err
		}
		if t.text == "&&" || t.text == "||" {
			left = &Logical{Op: t.text, L: left, R: right}
		} else {
			left = &Binary{Op: t.text, L: left, R: right}
		}
	}
}

func (p *parser) unary() (Node, error) {
	t := p.peek()
	if (t.kind == tPunct && (t.text == "!" || t.text == "-" || t.text == "+")) ||
		(t.kind == tKeyword && (t.text == "typeof" || t.text == "void")) {
		p.advance()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &Unary{Op: t.text, X: x}, nil
	}
	if t.kind == tPunct && (t.text == "++" || t.text == "--") {
		p.advance()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		if !isTarget(x) {
			return nil, &SyntaxError{Msg: "Invalid left-hand side expression in prefix operation", Pos: t.pos}
		}
		return &Update{Op: t.text, Prefix: true, X: x}, nil
	}
	x, err := p.postfix()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tPunct && (t.text == "++" || t.text == "--") && !t.nl {
		if !isTarget(x) {
			return nil, &SyntaxError{Msg: "Invalid left-hand side expression in postfix operation", Pos: t.pos}
		}
		p.advance()
		return &Update{Op: t.text, X: x}, nil
	}
	return x, nil
}

func (p *parser) postfix() (Node, error) {
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.is("."):
			p.advance()
			t := p.advance()
			if t.kind != tIdent && t.kind != tKeyword {
				return nil, p.fail(t)
			}
			x = &Member{Object: x, Property: &StringLit{Value: t.text}}
		case p.is("["):
			p.advance()
			key, err := p.expression()
			if err != nil {
				return nil, err
			}
			if err := p.expect("]"); err != nil {
				return nil, err
			}
			x = &Member{Object: x, Property: key, Computed: true}
		case p.is("("):
			p.advance()
			var args []Node
			for !p.is(")") {
				a, err := p.assign()
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if !p.accept(",") {
					break
				}
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			x = &Call{Callee: x, Args: args}
		default:
			return x, nil
		}
	}
}

func (p *parser) primary() (Node, error) {
	t := p.advance()
	switch t.kind {
	case tNumber:
		return &NumberLit{Value: t.num}, nil
	case tString:
		return &StringLit{Value: t.str}, nil
	case tTemplate:
		return parseTemplate(t.text)
	case tIdent:
		return &Ident{Name: t.text}, nil
	case tKeyword:
		switch t.text {
		case "true":
			return &BoolLit{Value: true}, nil
		case "false":
			return &BoolLit{Value: false}, nil
		case "null":
			return &NullLit{}, nil
		case "this":
			return &ThisExpr{}, nil
		}
	case tPunct:
		switch t.text {
		case "(":
			x, err := p.expression()
			if err != nil {
				return nil, err
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			return &Paren{X: x}, nil
		case "[":
			arr := &ArrayLit{}
			for !p.is("]") {
				if p.is(",") {
					return nil, p.fail(p.peek())
				}
				e, err := p.assign()
				if err != nil {
					return nil, err
				}
				arr.Elems = append(arr.Elems, e)
				if !p.accept(",") {
					break
				}
			}
			if err := p.expect("]"); err != nil {
				return nil, err
			}
			return arr, nil
		case "{":
			return p.object()
		}
	}
	return nil, p.fail(t)
}

func (p *parser) object() (Node, error) {
	obj := &ObjectLit{}
	for !p.is("}") {
		prop, err := p.property()
		if err != nil {
			return nil, err
		}
		obj.Props = append(obj.Props, prop)
		if !p.accept(",") {
			break
		}
	}
	if err := p.expect("}"); err != nil {
		return nil, err
	}
	return obj, nil
}

func (p *parser) property() (Property, error) {
	var prop Property
	t := p.advance()
	switch t.kind {
	case tIdent, tKeyword:
		prop.Key = t.text
		if t.kind == tIdent && (p.is(",") || p.is("}")) {
			prop.Value = &Ident{Name: t.text}
			prop.shorthand = true
			return prop, nil
		}
	case tString:
		prop.Key = t.str
	case tNumber:
		prop.Key = NumberToString(t.num)
		prop.numericKey = true
	case tPunct:
		if t.text != "[" {
			return prop, p.fail(t)
		}
		k, err := p.assign()
		if err != nil {
			return prop, err
		}
		if err := p.expect("]"); err != nil {
			return prop, err
		}
		prop.KeyExpr, prop.Computed = k, true
	default:
		return prop, p.fail(t)
	}
	if err := p.expect(":"); err != nil {
		return prop, err
	}
	v, err := p.assign()
	if err != nil {
		return prop, err
	}
	prop.Value = v
	return prop, nil
}

func parseTemplate(src string) (Node, error) {
	quasis, exprs, err := cookTemplate(src)
	if err != nil {
		return nil, err
	}
	tl := &TemplateLit{Quasis: quasis}
	for _, e := range exprs {
		x, err := ParseExpression(e)
		if err != nil {
			return nil, err
		}
		tl.Exprs = append(tl.Exprs, x)
	}
	return tl, nil
}
