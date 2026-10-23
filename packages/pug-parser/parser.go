// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-parser); MIT licence, see LICENSE.

// Package parser builds the pug syntax tree from the lexer's tokens, following the original pug-parser,
// including its error codes, messages and positions and the warnings it prints.
package parser

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	pugerror "github.com/pugjs/pug/packages/pug-error"
	lexer "github.com/pugjs/pug/packages/pug-lexer"

	"github.com/pugjs/pug/internal/js"
)

// Options configure a parse.
type Options struct {
	Filename string
	// Src is the template source, for the context of error messages.
	Src string
	// Warnings receives the warnings the original prints with console.warn; nil discards them.
	Warnings io.Writer
}

var inlineTags = map[string]bool{
	"a": true, "abbr": true, "acronym": true, "b": true, "br": true, "code": true, "em": true, "font": true,
	"i": true, "img": true, "ins": true, "kbd": true, "map": true, "samp": true, "small": true, "span": true,
	"strong": true, "sub": true, "sup": true,
}

// Parser holds the token stream of one parse.
type Parser struct {
	tokens   []lexer.Token
	filename string
	src      string
	inMixin  int
	warnings io.Writer
}

// Parse returns the syntax tree of tokens.
func Parse(tokens []lexer.Token, o Options) (*Node, error) {
	p := &Parser{tokens: append([]lexer.Token(nil), tokens...), filename: o.Filename, src: o.Src, warnings: o.Warnings}
	return p.parse()
}

func (p *Parser) error(code, message string, t lexer.Token) error {
	return pugerror.New(code, message, pugerror.Options{Line: t.Loc.Start.Line, Column: t.Loc.Start.Column,
		Filename: p.filename, Src: p.src, HasSrc: true})
}

func (p *Parser) warn(format string, args ...any) {
	if p.warnings != nil {
		fmt.Fprintf(p.warnings, format, args...)
	}
}

func (p *Parser) peek() lexer.Token {
	return p.tokens[0]
}

func (p *Parser) advance() lexer.Token {
	t := p.tokens[0]
	if len(p.tokens) > 1 {
		p.tokens = p.tokens[1:]
	}
	return t
}

func (p *Parser) deferToken(t lexer.Token) {
	p.tokens = append([]lexer.Token{t}, p.tokens...)
}

func (p *Parser) expect(typ string) (lexer.Token, error) {
	if p.peek().Type == typ {
		return p.advance(), nil
	}
	return lexer.Token{}, p.error("INVALID_TOKEN", `expected "`+typ+`", but got "`+p.peek().Type+`"`, p.peek())
}

func (p *Parser) accept(typ string) (lexer.Token, bool) {
	if p.peek().Type == typ {
		return p.advance(), true
	}
	return lexer.Token{}, false
}

func (p *Parser) initBlock(line int, nodes []*Node) *Node {
	if nodes == nil {
		nodes = []*Node{}
	}
	return &Node{Type: "Block", Nodes: nodes, Line: line, Filename: p.filename}
}

func (p *Parser) emptyBlock(line int) *Node {
	return p.initBlock(line, nil)
}

func (p *Parser) node(typ string, t lexer.Token) *Node {
	return &Node{Type: typ, Line: t.Loc.Start.Line, Column: t.Loc.Start.Column, Filename: p.filename}
}

func (p *Parser) parse() (*Node, error) {
	block := p.emptyBlock(0)
	for p.peek().Type != "eos" {
		switch p.peek().Type {
		case "newline":
			p.advance()
		case "text-html":
			nodes, err := p.parseTextHTML()
			if err != nil {
				return nil, err
			}
			block.Nodes = append(block.Nodes, nodes...)
		default:
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if expr != nil {
				if expr.Type == "Block" {
					block.Nodes = append(block.Nodes, expr.Nodes...)
				} else {
					block.Nodes = append(block.Nodes, expr)
				}
			}
		}
	}
	return block, nil
}

func (p *Parser) parseExpr() (*Node, error) {
	switch p.peek().Type {
	case "tag":
		return p.parseTag()
	case "mixin":
		return p.parseMixin()
	case "block":
		return p.parseBlock()
	case "mixin-block":
		return p.parseMixinBlock()
	case "case":
		return p.parseCase()
	case "extends":
		return p.parseExtends()
	case "include":
		return p.parseInclude()
	case "doctype":
		t := p.advance()
		n := p.node("Doctype", t)
		n.Val, n.HasVal = t.Val, t.HasVal
		return n, nil
	case "filter":
		return p.parseFilter()
	case "comment":
		return p.parseComment()
	case "text", "interpolated-code", "start-pug-interpolation":
		return p.parseText(true)
	case "text-html":
		line := p.peek().Loc.Start.Line
		nodes, err := p.parseTextHTML()
		if err != nil {
			return nil, err
		}
		return p.initBlock(line, nodes), nil
	case "dot":
		p.advance()
		b, err := p.parseTextBlock()
		if err != nil || b == nil {
			return nil, err
		}
		return b, nil
	case "each":
		return p.parseEach()
	case "eachOf":
		return p.parseEachOf()
	case "code":
		return p.parseCode(false)
	case "blockcode":
		return p.parseBlockCode()
	case "if":
		return p.parseConditional()
	case "while":
		return p.parseWhile()
	case "call":
		return p.parseCall()
	case "interpolation":
		return p.parseInterpolation()
	case "yield":
		t := p.advance()
		return p.node("YieldBlock", t), nil
	case "id", "class":
		t := p.peek()
		p.deferToken(lexer.Token{Type: "tag", Val: "div", HasVal: true, Loc: t.Loc})
		return p.parseExpr()
	}
	return nil, p.error("INVALID_TOKEN", `unexpected token "`+p.peek().Type+`"`, p.peek())
}

// parseText parses text and inline code; with block set it continues across newlines.
func (p *Parser) parseText(block bool) (*Node, error) {
	var tags []*Node
	lineno := p.peek().Loc.Start.Line
loop:
	for {
		next := p.peek()
		switch next.Type {
		case "text":
			t := p.advance()
			n := p.node("Text", t)
			n.Val, n.HasVal = t.Val, true
			tags = append(tags, n)
		case "interpolated-code":
			t := p.advance()
			tags = append(tags, p.inlineCode(t))
		case "newline":
			if !block {
				break loop
			}
			t := p.advance()
			if nt := p.peek().Type; nt == "text" || nt == "interpolated-code" {
				n := p.node("Text", t)
				n.Val, n.HasVal = "\n", true
				tags = append(tags, n)
			}
		case "start-pug-interpolation":
			p.advance()
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			tags = append(tags, expr)
			if _, err := p.expect("end-pug-interpolation"); err != nil {
				return nil, err
			}
		default:
			break loop
		}
	}
	if len(tags) == 1 {
		return tags[0], nil
	}
	return p.initBlock(lineno, tags), nil
}

func (p *Parser) inlineCode(t lexer.Token) *Node {
	n := p.node("Code", t)
	n.Val, n.HasVal = t.Val, true
	n.Buffer = t.Buffer
	n.MustEscape = t.MustEscape
	n.IsInline = true
	return n
}

func (p *Parser) parseTextHTML() ([]*Node, error) {
	var nodes []*Node
	var current *Node
	for {
		switch p.peek().Type {
		case "text-html":
			t := p.advance()
			if current == nil {
				current = p.node("Text", t)
				current.Val, current.HasVal = t.Val, true
				current.IsHTML = true
				nodes = append(nodes, current)
			} else {
				current.Val += "\n" + t.Val
			}
		case "indent":
			block, err := p.block()
			if err != nil {
				return nil, err
			}
			for _, n := range block.Nodes {
				if n.IsHTML {
					if current == nil {
						current = n
						nodes = append(nodes, current)
					} else {
						current.Val += "\n" + n.Val
					}
				} else {
					current = nil
					nodes = append(nodes, n)
				}
			}
		case "code":
			current = nil
			n, err := p.parseCode(true)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, n)
		case "newline":
			p.advance()
		default:
			return nodes, nil
		}
	}
}

// requiredExpr parses an expression whose type the original reads right away.
func (p *Parser) requiredExpr() (*Node, error) {
	expr, err := p.parseExpr()
	if err == nil && expr == nil {
		err = &js.Error{Name: "TypeError", Message: "Cannot read properties of undefined (reading 'type')"}
	}
	return expr, err
}

func (p *Parser) parseBlockExpansion() (*Node, error) {
	if t, ok := p.accept(":"); ok {
		expr, err := p.requiredExpr()
		if err != nil {
			return nil, err
		}
		if expr.Type == "Block" {
			return expr, nil
		}
		return p.initBlock(t.Loc.Start.Line, []*Node{expr}), nil
	}
	return p.block()
}

func (p *Parser) parseCase() (*Node, error) {
	t, _ := p.expect("case")
	n := p.node("Case", t)
	n.Expr = t.Val
	block := p.emptyBlock(t.Loc.Start.Line + 1)
	if _, err := p.expect("indent"); err != nil {
		return nil, err
	}
	for p.peek().Type != "outdent" {
		switch p.peek().Type {
		case "comment", "newline":
			p.advance()
		case "when":
			w, err := p.parseWhen()
			if err != nil {
				return nil, err
			}
			block.Nodes = append(block.Nodes, w)
		case "default":
			t := p.advance()
			w := p.node("When", t)
			w.Expr = "default"
			w.NoDebug = true
			b, err := p.parseBlockExpansion()
			if err != nil {
				return nil, err
			}
			w.Block = b
			block.Nodes = append(block.Nodes, w)
		default:
			return nil, p.error("INVALID_TOKEN", `Unexpected token "`+p.peek().Type+
				`", expected "when", "default" or "newline"`, p.peek())
		}
	}
	if _, err := p.expect("outdent"); err != nil {
		return nil, err
	}
	n.Block = block
	return n, nil
}

func (p *Parser) parseWhen() (*Node, error) {
	t, _ := p.expect("when")
	n := p.node("When", t)
	n.Expr = t.Val
	n.NoDebug = true
	if p.peek().Type != "newline" {
		b, err := p.parseBlockExpansion()
		if err != nil {
			return nil, err
		}
		n.Block = b
	}
	return n, nil
}

var elseCodeRe = regexp.MustCompile(`^ *else`)

func (p *Parser) parseCode(noBlock bool) (*Node, error) {
	t, err := p.expect("code")
	if err != nil {
		return nil, err
	}
	n := p.node("Code", t)
	n.Val, n.HasVal = t.Val, true
	n.Buffer = t.Buffer
	n.MustEscape = t.MustEscape
	n.IsInline = noBlock
	if elseCodeRe.MatchString(n.Val) {
		n.NoDebug = true
	}
	if noBlock {
		return n, nil
	}
	if p.peek().Type == "indent" {
		if t.Buffer {
			return nil, p.error("BLOCK_IN_BUFFERED_CODE", "Buffered code cannot have a block attached to it", p.peek())
		}
		b, err := p.block()
		if err != nil {
			return nil, err
		}
		n.Block = b
	}
	return n, nil
}

func (p *Parser) parseConditional() (*Node, error) {
	t, _ := p.expect("if")
	n := p.node("Conditional", t)
	n.Test = t.Val
	n.Consequent = p.emptyBlock(t.Loc.Start.Line)
	if p.peek().Type == "indent" {
		b, err := p.block()
		if err != nil {
			return nil, err
		}
		n.Consequent = b
	}
	current := n
	for {
		switch p.peek().Type {
		case "newline":
			p.advance()
			continue
		case "else-if":
			t := p.advance()
			alt := p.node("Conditional", t)
			alt.Test = t.Val
			alt.Consequent = p.emptyBlock(t.Loc.Start.Line)
			current.Alternate = alt
			current = alt
			if p.peek().Type == "indent" {
				b, err := p.block()
				if err != nil {
					return nil, err
				}
				current.Consequent = b
			}
			continue
		case "else":
			p.advance()
			if p.peek().Type == "indent" {
				b, err := p.block()
				if err != nil {
					return nil, err
				}
				current.Alternate = b
			}
		}
		return n, nil
	}
}

func (p *Parser) parseWhile() (*Node, error) {
	t, _ := p.expect("while")
	n := p.node("While", t)
	n.Test = t.Val
	if p.peek().Type == "indent" {
		b, err := p.block()
		if err != nil {
			return nil, err
		}
		n.Block = b
	} else {
		n.Block = p.emptyBlock(t.Loc.Start.Line)
	}
	return n, nil
}

func (p *Parser) parseBlockCode() (*Node, error) {
	t, _ := p.expect("blockcode")
	n := p.node("Code", t)
	var text strings.Builder
	if p.peek().Type == "start-pipeless-text" {
		p.advance()
		for p.peek().Type != "end-pipeless-text" {
			t := p.advance()
			switch t.Type {
			case "text":
				text.WriteString(t.Val)
			case "newline":
				text.WriteString("\n")
			default:
				return nil, p.error("INVALID_TOKEN", "Unexpected token type: "+t.Type, t)
			}
		}
		p.advance()
	}
	n.Val, n.HasVal = text.String(), true
	return n, nil
}

func (p *Parser) parseComment() (*Node, error) {
	t, _ := p.expect("comment")
	block, err := p.parseTextBlock()
	if err != nil {
		return nil, err
	}
	typ := "Comment"
	if block != nil {
		typ = "BlockComment"
	}
	n := p.node(typ, t)
	n.Val, n.HasVal = t.Val, true
	n.Block = block
	n.Buffer = t.Buffer
	return n, nil
}

func (p *Parser) parseIncludeFilter() (*Node, error) {
	t, _ := p.expect("filter")
	n := p.node("IncludeFilter", t)
	n.Name = t.Val
	if p.peek().Type == "start-attributes" {
		attrs, err := p.attrs(nil)
		if err != nil {
			return nil, err
		}
		n.Attrs = attrs
	}
	return n, nil
}

func (p *Parser) parseFilter() (*Node, error) {
	t, _ := p.expect("filter")
	n := p.node("Filter", t)
	n.Name = t.Val
	if p.peek().Type == "start-attributes" {
		attrs, err := p.attrs(nil)
		if err != nil {
			return nil, err
		}
		n.Attrs = attrs
	}
	switch p.peek().Type {
	case "text":
		tt := p.advance()
		text := p.node("Text", tt)
		text.Val, text.HasVal = tt.Val, true
		n.Block = p.initBlock(tt.Loc.Start.Line, []*Node{text})
	case "filter":
		inner, err := p.parseFilter()
		if err != nil {
			return nil, err
		}
		n.Block = p.initBlock(t.Loc.Start.Line, []*Node{inner})
	default:
		b, err := p.parseTextBlock()
		if err != nil {
			return nil, err
		}
		if b == nil {
			b = p.emptyBlock(t.Loc.Start.Line)
		}
		n.Block = b
	}
	return n, nil
}

func (p *Parser) parseEach() (*Node, error) {
	t, _ := p.expect("each")
	n := p.node("Each", t)
	n.Obj = t.Code
	n.EachVal = t.Val
	n.Key, n.HasKey = t.Key, t.HasKey
	b, err := p.block()
	if err != nil {
		return nil, err
	}
	n.Block = b
	if p.peek().Type == "else" {
		p.advance()
		alt, err := p.block()
		if err != nil {
			return nil, err
		}
		n.Alternate = alt
	}
	return n, nil
}

func (p *Parser) parseEachOf() (*Node, error) {
	t, _ := p.expect("eachOf")
	n := p.node("EachOf", t)
	n.Obj = t.Code
	n.EachVal = t.Val
	b, err := p.block()
	if err != nil {
		return nil, err
	}
	n.Block = b
	return n, nil
}

func (p *Parser) parseExtends() (*Node, error) {
	t, _ := p.expect("extends")
	path, err := p.expect("path")
	if err != nil {
		return nil, err
	}
	n := p.node("Extends", t)
	n.File = &FileReference{Path: js.TrimSpace(path.Val), Line: path.Loc.Start.Line, Column: path.Loc.Start.Column,
		Filename: p.filename}
	return n, nil
}

func (p *Parser) parseBlock() (*Node, error) {
	t, _ := p.expect("block")
	var n *Node
	if p.peek().Type == "indent" {
		b, err := p.block()
		if err != nil {
			return nil, err
		}
		n = b
	} else {
		n = p.emptyBlock(t.Loc.Start.Line)
	}
	n.Type = "NamedBlock"
	n.Name = js.TrimSpace(t.Val)
	n.Mode = t.Mode
	n.Line = t.Loc.Start.Line
	n.Column = t.Loc.Start.Column
	return n, nil
}

func (p *Parser) parseMixinBlock() (*Node, error) {
	t, _ := p.expect("mixin-block")
	if p.inMixin == 0 {
		return nil, p.error("BLOCK_OUTISDE_MIXIN", "Anonymous blocks are not allowed unless they are part of a mixin.", t)
	}
	return p.node("MixinBlock", t), nil
}

var (
	pugExtRe  = regexp.MustCompile(`\.pug$`)
	jadeExtRe = regexp.MustCompile(`\.jade$`)
)

func (p *Parser) parseInclude() (*Node, error) {
	t, _ := p.expect("include")
	n := p.node("Include", t)
	n.File = &FileReference{Filename: p.filename}
	var filters []*Node
	for p.peek().Type == "filter" {
		f, err := p.parseIncludeFilter()
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}
	path, err := p.expect("path")
	if err != nil {
		return nil, err
	}
	n.File.Path = js.TrimSpace(path.Val)
	n.File.Line = path.Loc.Start.Line
	n.File.Column = path.Loc.Start.Column
	if (jadeExtRe.MatchString(n.File.Path) || pugExtRe.MatchString(n.File.Path)) && len(filters) == 0 {
		if p.peek().Type == "indent" {
			b, err := p.block()
			if err != nil {
				return nil, err
			}
			n.Block = b
		} else {
			n.Block = p.emptyBlock(t.Loc.Start.Line)
		}
		if jadeExtRe.MatchString(n.File.Path) {
			p.warn("%s, line %d:\nThe .jade extension is deprecated, use .pug for \"%s\".\n", undefinedName(p.filename),
				t.Loc.Start.Line, n.File.Path)
		}
		return n, nil
	}
	n.Type = "RawInclude"
	n.Filters = filters
	if p.peek().Type == "indent" {
		return nil, p.error("RAW_INCLUDE_BLOCK", "Raw inclusion cannot contain a block", p.peek())
	}
	return n, nil
}

// undefinedName renders a filename the way string concatenation renders an undefined one.
func undefinedName(name string) string {
	if name == "" {
		return "undefined"
	}
	return name
}

func (p *Parser) parseCall() (*Node, error) {
	t, _ := p.expect("call")
	n := p.node("Mixin", t)
	n.Name = t.Val
	n.Args, n.HasArgs = t.Args, t.HasArgs
	n.Block = p.emptyBlock(t.Loc.Start.Line)
	n.Call = true
	n.Attrs = []Attr{}
	if err := p.tag(n, false); err != nil {
		return nil, err
	}
	if len(n.Block.Nodes) == 0 {
		n.Block = nil
	}
	return n, nil
}

func (p *Parser) parseMixin() (*Node, error) {
	t, _ := p.expect("mixin")
	if p.peek().Type != "indent" {
		return nil, p.error("MIXIN_WITHOUT_BODY", "Mixin "+t.Val+" declared without body", t)
	}
	p.inMixin++
	b, err := p.block()
	p.inMixin--
	if err != nil {
		return nil, err
	}
	n := p.node("Mixin", t)
	n.Name = t.Val
	n.Args, n.HasArgs = t.Args, t.HasArgs
	n.Block = b
	return n, nil
}

// parseTextBlock parses the text of a pipeless block; it returns nil when no block follows.
func (p *Parser) parseTextBlock() (*Node, error) {
	t, ok := p.accept("start-pipeless-text")
	if !ok {
		return nil, nil
	}
	block := p.emptyBlock(t.Loc.Start.Line)
	for p.peek().Type != "end-pipeless-text" {
		t := p.advance()
		switch t.Type {
		case "text":
			n := p.node("Text", t)
			n.Val, n.HasVal = t.Val, true
			block.Nodes = append(block.Nodes, n)
		case "newline":
			n := p.node("Text", t)
			n.Val, n.HasVal = "\n", true
			block.Nodes = append(block.Nodes, n)
		case "start-pug-interpolation":
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			block.Nodes = append(block.Nodes, expr)
			if _, err := p.expect("end-pug-interpolation"); err != nil {
				return nil, err
			}
		case "interpolated-code":
			block.Nodes = append(block.Nodes, p.inlineCode(t))
		default:
			return nil, p.error("INVALID_TOKEN", "Unexpected token type: "+t.Type, t)
		}
	}
	p.advance()
	return block, nil
}

func (p *Parser) block() (*Node, error) {
	t, err := p.expect("indent")
	if err != nil {
		return nil, err
	}
	block := p.emptyBlock(t.Loc.Start.Line)
	for p.peek().Type != "outdent" {
		switch p.peek().Type {
		case "newline":
			p.advance()
		case "text-html":
			nodes, err := p.parseTextHTML()
			if err != nil {
				return nil, err
			}
			block.Nodes = append(block.Nodes, nodes...)
		default:
			expr, err := p.requiredExpr()
			if err != nil {
				return nil, err
			}
			if expr.Type == "Block" {
				block.Nodes = append(block.Nodes, expr.Nodes...)
			} else {
				block.Nodes = append(block.Nodes, expr)
			}
		}
	}
	if _, err := p.expect("outdent"); err != nil {
		return nil, err
	}
	return block, nil
}

func (p *Parser) parseInterpolation() (*Node, error) {
	t := p.advance()
	n := p.node("InterpolatedTag", t)
	n.Expr = t.Val
	n.Block = p.emptyBlock(t.Loc.Start.Line)
	n.Attrs = []Attr{}
	if err := p.tag(n, true); err != nil {
		return nil, err
	}
	return n, nil
}

func (p *Parser) parseTag() (*Node, error) {
	t := p.advance()
	n := p.node("Tag", t)
	n.Name = t.Val
	n.Block = p.emptyBlock(t.Loc.Start.Line)
	n.Attrs = []Attr{}
	n.IsInline = inlineTags[t.Val]
	if err := p.tag(n, true); err != nil {
		return nil, err
	}
	return n, nil
}

// tag parses the attributes, text, code, block expansion and block of a tag or mixin call into n.
func (p *Parser) tag(n *Node, selfClosingAllowed bool) error {
	seenAttrs := false
	var attributeNames []string
out:
	for {
		switch p.peek().Type {
		case "id", "class":
			t := p.advance()
			if t.Type == "id" {
				if contains(attributeNames, "id") {
					return p.error("DUPLICATE_ID", `Duplicate attribute "id" is not allowed.`, t)
				}
				attributeNames = append(attributeNames, "id")
			}
			n.Attrs = append(n.Attrs, Attr{Name: t.Type, Val: "'" + t.Val + "'", Line: t.Loc.Start.Line,
				Column: t.Loc.Start.Column, Filename: p.filename})
		case "start-attributes":
			if seenAttrs {
				p.warn("%s, line %d:\nYou should not have pug tags with multiple attributes.\n",
					undefinedName(p.filename), p.peek().Loc.Start.Line)
			}
			seenAttrs = true
			attrs, err := p.attrs(&attributeNames)
			if err != nil {
				return err
			}
			n.Attrs = append(n.Attrs, attrs...)
		case "&attributes":
			t := p.advance()
			b := p.node("AttributeBlock", t)
			b.Val, b.HasVal = t.Val, true
			n.AttrBlks = append(n.AttrBlks, b)
		default:
			break out
		}
	}
	if p.peek().Type == "dot" {
		n.TextOnly = true
		p.advance()
	}
	switch p.peek().Type {
	case "text", "interpolated-code":
		text, err := p.parseText(false)
		if err != nil {
			return err
		}
		if text.Type == "Block" {
			n.Block.Nodes = append(n.Block.Nodes, text.Nodes...)
		} else {
			n.Block.Nodes = append(n.Block.Nodes, text)
		}
	case "code":
		c, err := p.parseCode(true)
		if err != nil {
			return err
		}
		n.Block.Nodes = append(n.Block.Nodes, c)
	case ":":
		p.advance()
		expr, err := p.requiredExpr()
		if err != nil {
			return err
		}
		if expr.Type == "Block" {
			n.Block = expr
		} else {
			n.Block = p.initBlock(n.Line, []*Node{expr})
		}
	case "newline", "indent", "outdent", "eos", "start-pipeless-text", "end-pug-interpolation":
	case "slash":
		if selfClosingAllowed {
			p.advance()
			n.SelfClosing = true
			break
		}
		fallthrough
	default:
		expected := "`text`, `interpolated-code`, `code`, `:`"
		if selfClosingAllowed {
			expected += ", `slash`"
		}
		return p.error("INVALID_TOKEN", "Unexpected token `"+p.peek().Type+"` expected "+expected+
			", `newline` or `eos`", p.peek())
	}
	for p.peek().Type == "newline" {
		p.advance()
	}
	if n.TextOnly {
		b, err := p.parseTextBlock()
		if err != nil {
			return err
		}
		if b == nil {
			b = p.emptyBlock(n.Line)
		}
		n.Block = b
	} else if p.peek().Type == "indent" {
		b, err := p.block()
		if err != nil {
			return err
		}
		n.Block.Nodes = append(n.Block.Nodes, b.Nodes...)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// attrs parses an attribute list; names collects the names seen on the tag for the duplicate check (nil for
// filters, which allow duplicates).
func (p *Parser) attrs(names *[]string) ([]Attr, error) {
	if _, err := p.expect("start-attributes"); err != nil {
		return nil, err
	}
	var attrs []Attr
	t := p.advance()
	for t.Type == "attribute" {
		if t.Name != "class" && names != nil {
			if contains(*names, t.Name) {
				return nil, p.error("DUPLICATE_ATTRIBUTE", `Duplicate attribute "`+t.Name+`" is not allowed.`, t)
			}
			*names = append(*names, t.Name)
		}
		attrs = append(attrs, Attr{Name: t.Name, Val: t.AttrVal, True: t.AttrTrue, MustEscape: t.MustEscape,
			Line: t.Loc.Start.Line, Column: t.Loc.Start.Column, Filename: p.filename})
		t = p.advance()
	}
	p.deferToken(t)
	if _, err := p.expect("end-attributes"); err != nil {
		return nil, err
	}
	return attrs, nil
}
