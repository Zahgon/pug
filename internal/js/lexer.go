package js

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tKeyword
	tNumber
	tString
	tTemplate
	tPunct
)

type token struct {
	kind tokKind
	text string // identifier name, keyword, punctuator, or the source of a template literal
	num  float64
	str  string // cooked string value
	nl   bool   // a line terminator precedes the token
	pos  int
}

var keywords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true, "debugger": true,
	"default": true, "delete": true, "do": true, "else": true, "export": true, "extends": true, "false": true,
	"finally": true, "for": true, "function": true, "if": true, "import": true, "in": true, "instanceof": true,
	"new": true, "null": true, "return": true, "super": true, "switch": true, "this": true, "throw": true,
	"true": true, "try": true, "typeof": true, "var": true, "void": true, "while": true, "with": true,
	"enum": true,
}

var punctuators = []string{
	">>>=", "...", "===", "!==", "**=", "<<=", ">>=", ">>>", "&&=", "||=", "??=",
	"=>", "==", "!=", "<=", ">=", "&&", "||", "??", "?.", "++", "--", "+=", "-=", "*=", "/=", "%=", "&=", "|=",
	"^=", "**", "<<", ">>",
	"{", "}", "(", ")", "[", "]", ".", ";", ",", "<", ">", "+", "-", "*", "/", "%", "&", "|", "^", "!", "~", "?",
	":", "=", "@", "#",
}

// SyntaxError is a JavaScript syntax error in template code.
type SyntaxError struct {
	Msg string
	Pos int
}

// Error returns the message.
func (e *SyntaxError) Error() string {
	return e.Msg
}

type lexer struct {
	src  string
	pos  int
	toks []token
}

func isIDStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r) || unicode.Is(unicode.Nl, r)
}

func isIDPart(r rune) bool {
	return isIDStart(r) || unicode.IsDigit(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc) || r == 0x200C || r == 0x200D
}

func isLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

func isSpace(r rune) bool {
	return r == '\t' || r == '\v' || r == '\f' || r == ' ' || r == 0xA0 || r == 0xFEFF || unicode.Is(unicode.Zs, r)
}

func tokenize(src string) ([]token, error) {
	lx := &lexer{src: src}
	nl := false
	for {
		// whitespace and comments
		for lx.pos < len(src) {
			r, size := utf8.DecodeRuneInString(src[lx.pos:])
			switch {
			case isLineTerminator(r):
				nl = true
				lx.pos += size
			case isSpace(r):
				lx.pos += size
			case strings.HasPrefix(src[lx.pos:], "//"):
				for lx.pos < len(src) {
					r, size := utf8.DecodeRuneInString(src[lx.pos:])
					if isLineTerminator(r) {
						break
					}
					lx.pos += size
				}
			case strings.HasPrefix(src[lx.pos:], "/*"):
				end := strings.Index(src[lx.pos+2:], "*/")
				if end < 0 {
					return nil, &SyntaxError{Msg: "Unterminated comment", Pos: lx.pos}
				}
				if strings.ContainsAny(src[lx.pos:lx.pos+2+end], "\n\r  ") {
					nl = true
				}
				lx.pos += end + 4
			default:
				goto token
			}
		}
	token:
		if lx.pos >= len(src) {
			lx.toks = append(lx.toks, token{kind: tEOF, nl: nl, pos: lx.pos})
			return lx.toks, nil
		}
		tok, err := lx.next()
		if err != nil {
			return nil, err
		}
		tok.nl = nl
		nl = false
		lx.toks = append(lx.toks, tok)
	}
}

func (lx *lexer) next() (token, error) {
	start := lx.pos
	r, _ := utf8.DecodeRuneInString(lx.src[lx.pos:])
	switch {
	case isIDStart(r) || r == '\\':
		for lx.pos < len(lx.src) {
			r, size := utf8.DecodeRuneInString(lx.src[lx.pos:])
			if !isIDPart(r) {
				break
			}
			lx.pos += size
		}
		name := lx.src[start:lx.pos]
		if name == "" {
			return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: start}
		}
		if keywords[name] {
			return token{kind: tKeyword, text: name, pos: start}, nil
		}
		return token{kind: tIdent, text: name, pos: start}, nil
	case r >= '0' && r <= '9' || (r == '.' && lx.pos+1 < len(lx.src) && lx.src[lx.pos+1] >= '0' && lx.src[lx.pos+1] <= '9'):
		return lx.number()
	case r == '"' || r == '\'':
		return lx.str(byte(r))
	case r == '`':
		return lx.template()
	}
	for _, p := range punctuators {
		if strings.HasPrefix(lx.src[lx.pos:], p) {
			lx.pos += len(p)
			return token{kind: tPunct, text: p, pos: start}, nil
		}
	}
	return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: start}
}

func (lx *lexer) number() (token, error) {
	start := lx.pos
	src := lx.src
	if src[lx.pos] == '0' && lx.pos+1 < len(src) {
		base := 0
		switch src[lx.pos+1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			lx.pos += 2
			ds := lx.pos
			for lx.pos < len(src) && digitValue(src[lx.pos]) < base {
				lx.pos++
			}
			if lx.pos == ds {
				return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: start}
			}
			v, _ := new(big.Int).SetString(src[ds:lx.pos], base)
			f, _ := new(big.Float).SetInt(v).Float64()
			return lx.afterNumber(token{kind: tNumber, num: f, text: src[start:lx.pos], pos: start})
		}
	}
	for lx.pos < len(src) && isDigit(src[lx.pos]) {
		lx.pos++
	}
	if lx.pos < len(src) && src[lx.pos] == '.' {
		lx.pos++
		for lx.pos < len(src) && isDigit(src[lx.pos]) {
			lx.pos++
		}
	}
	if lx.pos < len(src) && (src[lx.pos] == 'e' || src[lx.pos] == 'E') {
		k := lx.pos + 1
		if k < len(src) && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < len(src) && isDigit(src[k]) {
			for k < len(src) && isDigit(src[k]) {
				k++
			}
			lx.pos = k
		} else {
			return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: start}
		}
	}
	f, _ := strconv.ParseFloat(src[start:lx.pos], 64)
	return lx.afterNumber(token{kind: tNumber, num: f, text: src[start:lx.pos], pos: start})
}

// afterNumber rejects an identifier character directly after a numeric literal (3in, 1px).
func (lx *lexer) afterNumber(t token) (token, error) {
	if lx.pos < len(lx.src) {
		r, _ := utf8.DecodeRuneInString(lx.src[lx.pos:])
		if isIDStart(r) || (r >= '0' && r <= '9') {
			return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: t.pos}
		}
	}
	return t, nil
}

// escape decodes the escape sequence after a backslash at lx.pos and appends its code units to out.
func (lx *lexer) escape(out []uint16, template bool) ([]uint16, error) {
	src := lx.src
	if lx.pos >= len(src) {
		return nil, &SyntaxError{Msg: "Invalid or unexpected token", Pos: lx.pos}
	}
	c := src[lx.pos]
	lx.pos++
	switch c {
	case 'n':
		return append(out, '\n'), nil
	case 'r':
		return append(out, '\r'), nil
	case 't':
		return append(out, '\t'), nil
	case 'b':
		return append(out, '\b'), nil
	case 'f':
		return append(out, '\f'), nil
	case 'v':
		return append(out, '\v'), nil
	case '\r':
		if lx.pos < len(src) && src[lx.pos] == '\n' {
			lx.pos++
		}
		return out, nil
	case '\n':
		return out, nil
	case 'x':
		if lx.pos+2 > len(src) || digitValue(src[lx.pos]) >= 16 || digitValue(src[lx.pos+1]) >= 16 {
			return nil, &SyntaxError{Msg: "Invalid hexadecimal escape sequence", Pos: lx.pos}
		}
		v := digitValue(src[lx.pos])*16 + digitValue(src[lx.pos+1])
		lx.pos += 2
		return append(out, uint16(v)), nil
	case 'u':
		if lx.pos < len(src) && src[lx.pos] == '{' {
			end := strings.IndexByte(src[lx.pos:], '}')
			if end < 2 {
				return nil, &SyntaxError{Msg: "Invalid Unicode escape sequence", Pos: lx.pos}
			}
			v, err := strconv.ParseUint(src[lx.pos+1:lx.pos+end], 16, 32)
			if err != nil || v > 0x10FFFF {
				return nil, &SyntaxError{Msg: "Undefined Unicode code-point", Pos: lx.pos}
			}
			lx.pos += end + 1
			return appendRune(out, rune(v)), nil
		}
		if lx.pos+4 > len(src) {
			return nil, &SyntaxError{Msg: "Invalid Unicode escape sequence", Pos: lx.pos}
		}
		v, err := strconv.ParseUint(src[lx.pos:lx.pos+4], 16, 16)
		if err != nil {
			return nil, &SyntaxError{Msg: "Invalid Unicode escape sequence", Pos: lx.pos}
		}
		lx.pos += 4
		return append(out, uint16(v)), nil
	case '0':
		if lx.pos < len(src) && isDigit(src[lx.pos]) {
			if template {
				return nil, &SyntaxError{Msg: "Octal escape sequences are not allowed in template strings", Pos: lx.pos}
			}
			return lx.legacyOctal(out, 0)
		}
		return append(out, 0), nil
	case '1', '2', '3', '4', '5', '6', '7':
		if template {
			return nil, &SyntaxError{Msg: "Octal escape sequences are not allowed in template strings", Pos: lx.pos}
		}
		return lx.legacyOctal(out, int(c-'0'))
	}
	lx.pos--
	r, size := utf8.DecodeRuneInString(src[lx.pos:])
	lx.pos += size
	return appendRune(out, r), nil
}

func (lx *lexer) legacyOctal(out []uint16, v int) ([]uint16, error) {
	for n := 0; n < 2 && lx.pos < len(lx.src) && lx.src[lx.pos] >= '0' && lx.src[lx.pos] <= '7'; n++ {
		nv := v*8 + int(lx.src[lx.pos]-'0')
		if nv > 255 {
			break
		}
		v = nv
		lx.pos++
	}
	return append(out, uint16(v)), nil
}

func (lx *lexer) str(quote byte) (token, error) {
	start := lx.pos
	lx.pos++
	var out []uint16
	for {
		if lx.pos >= len(lx.src) {
			return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: start}
		}
		c := lx.src[lx.pos]
		switch {
		case c == quote:
			lx.pos++
			return token{kind: tString, str: FromUTF16(out), text: lx.src[start:lx.pos], pos: start}, nil
		case c == '\\':
			lx.pos++
			var err error
			out, err = lx.escape(out, false)
			if err != nil {
				return token{}, err
			}
		case c == '\n' || c == '\r':
			return token{}, &SyntaxError{Msg: "Invalid or unexpected token", Pos: start}
		default:
			r, size := utf8.DecodeRuneInString(lx.src[lx.pos:])
			out = appendRune(out, r)
			lx.pos += size
		}
	}
}

// template scans a whole template literal, including its substitutions, into one token; the parser splits it.
func (lx *lexer) template() (token, error) {
	start := lx.pos
	lx.pos++
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch {
		case c == '\\':
			lx.pos += 2
			continue
		case c == '`':
			lx.pos++
			return token{kind: tTemplate, text: lx.src[start:lx.pos], pos: start}, nil
		case c == '$' && lx.pos+1 < len(lx.src) && lx.src[lx.pos+1] == '{':
			end, err := matchBrace(lx.src, lx.pos+2)
			if err != nil {
				return token{}, err
			}
			lx.pos = end + 1
			continue
		}
		lx.pos++
	}
	return token{}, &SyntaxError{Msg: "Unterminated template literal", Pos: start}
}

// matchBrace returns the index of the } that closes a substitution whose body starts at i.
func matchBrace(src string, i int) (int, error) {
	sub := &lexer{src: src, pos: i}
	depth := 0
	for {
		for sub.pos < len(src) {
			r, size := utf8.DecodeRuneInString(src[sub.pos:])
			if !isSpace(r) && !isLineTerminator(r) {
				break
			}
			sub.pos += size
		}
		if sub.pos >= len(src) {
			return 0, &SyntaxError{Msg: "Unterminated template literal", Pos: i}
		}
		if src[sub.pos] == '}' && depth == 0 {
			return sub.pos, nil
		}
		tok, err := sub.next()
		if err != nil {
			return 0, err
		}
		if tok.kind == tPunct {
			switch tok.text {
			case "{":
				depth++
			case "}":
				depth--
			}
		}
	}
}

// cookTemplate splits the source of a template literal into its cooked strings and the sources of its
// substitutions.
func cookTemplate(src string) ([]string, []string, error) {
	lx := &lexer{src: src, pos: 1}
	var quasis, exprs []string
	var cur []uint16
	for {
		c := src[lx.pos]
		switch {
		case c == '`':
			return append(quasis, FromUTF16(cur)), exprs, nil
		case c == '\\':
			lx.pos++
			var err error
			cur, err = lx.escape(cur, true)
			if err != nil {
				return nil, nil, err
			}
		case c == '$' && src[lx.pos+1] == '{':
			end, err := matchBrace(src, lx.pos+2)
			if err != nil {
				return nil, nil, err
			}
			quasis = append(quasis, FromUTF16(cur))
			cur = nil
			exprs = append(exprs, src[lx.pos+2:end])
			lx.pos = end + 1
		case c == '\r':
			// Template literals normalise CR and CRLF to LF.
			cur = append(cur, '\n')
			lx.pos++
			if lx.pos < len(src) && src[lx.pos] == '\n' {
				lx.pos++
			}
		default:
			r, size := utf8.DecodeRuneInString(src[lx.pos:])
			cur = appendRune(cur, r)
			lx.pos += size
		}
	}
}

// String describes the token for error messages.
func (t token) String() string {
	switch t.kind {
	case tEOF:
		return "end of input"
	case tNumber, tString, tTemplate:
		return t.text
	}
	return fmt.Sprintf("%q", t.text)
}
