package js

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseJSON implements JSON.parse without a reviver. Errors are SyntaxErrors with V8's messages.
func ParseJSON(text string) (Value, error) {
	p := &jsonParser{src: ToUTF16(text)}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok != jsonEOS {
		loc := strings.TrimPrefix(p.location(), " in JSON")
		return nil, Throw("SyntaxError", "%s", "Unexpected non-whitespace character after JSON"+loc)
	}
	return v, nil
}

type jsonToken int

const (
	jsonEOS jsonToken = iota
	jsonString
	jsonNumber
	jsonLBrace
	jsonRBrace
	jsonLBrack
	jsonRBrack
	jsonColon
	jsonComma
	jsonTrue
	jsonFalse
	jsonNull
	jsonIllegal
)

type jsonParser struct {
	src []uint16
	pos int
}

func tokenOf(c uint16) jsonToken {
	switch {
	case c == '"':
		return jsonString
	case c == '-' || (c >= '0' && c <= '9'):
		return jsonNumber
	case c == '{':
		return jsonLBrace
	case c == '}':
		return jsonRBrace
	case c == '[':
		return jsonLBrack
	case c == ']':
		return jsonRBrack
	case c == ':':
		return jsonColon
	case c == ',':
		return jsonComma
	case c == 't':
		return jsonTrue
	case c == 'f':
		return jsonFalse
	case c == 'n':
		return jsonNull
	}
	return jsonIllegal
}

// peek skips JSON whitespace and returns the token that starts at the cursor.
func (p *jsonParser) peek() jsonToken {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
			continue
		}
		return tokenOf(p.src[p.pos])
	}
	return jsonEOS
}

// location returns V8's " at position N (line L column C)" suffix for the cursor.
func (p *jsonParser) location() string {
	line, lastBreak := 1, 0
	for i := 0; i < p.pos; i++ {
		c := p.src[i]
		if c == '\r' && i < p.pos-1 && p.src[i+1] == '\n' {
			i++
			c = '\n'
		}
		if c == '\r' || c == '\n' {
			line++
			lastBreak = i + 1
		}
	}
	return fmt.Sprintf(" in JSON at position %d (line %d column %d)", p.pos, line, p.pos-lastBreak+1)
}

// unexpected builds the SyntaxError for an unexpected token at the cursor; msg, when not empty, is the specific
// message of the place the token was found.
func (p *jsonParser) unexpected(tok jsonToken, msg string) error {
	if msg != "" {
		return Throw("SyntaxError", "%s", msg+p.location())
	}
	switch tok {
	case jsonEOS:
		return Throw("SyntaxError", "Unexpected end of JSON input")
	case jsonNumber:
		return Throw("SyntaxError", "%s", "Unexpected number"+p.location())
	case jsonString:
		return Throw("SyntaxError", "%s", "Unexpected string"+p.location())
	}
	src := FromUTF16(p.src)
	switch src {
	case "[object Object]", "undefined", "Infinity", "NaN":
		return Throw("SyntaxError", "\"%s\" is not valid JSON", src)
	}
	ch := FromUTF16(p.src[p.pos : p.pos+1])
	const context = 10
	n := len(p.src)
	if n <= 2*context+1 {
		return Throw("SyntaxError", "Unexpected token '%s', \"%s\" is not valid JSON", ch, src)
	}
	switch {
	case p.pos < context:
		return Throw("SyntaxError", "Unexpected token '%s', \"%s\"... is not valid JSON", ch, FromUTF16(p.src[:p.pos+context]))
	case p.pos < n-context:
		return Throw("SyntaxError", "Unexpected token '%s', ...\"%s\"... is not valid JSON", ch, FromUTF16(p.src[p.pos-context:p.pos+context]))
	}
	return Throw("SyntaxError", "Unexpected token '%s', ...\"%s\" is not valid JSON", ch, FromUTF16(p.src[p.pos-context:]))
}

// unexpectedChar reports the character at the cursor (or the end of the input) as unexpected.
func (p *jsonParser) unexpectedChar(msg string) error {
	if p.pos >= len(p.src) {
		return p.unexpected(jsonEOS, msg)
	}
	return p.unexpected(tokenOf(p.src[p.pos]), msg)
}

func (p *jsonParser) value() (Value, error) {
	tok := p.peek()
	switch tok {
	case jsonString:
		p.pos++
		return p.str()
	case jsonNumber:
		return p.number()
	case jsonLBrace:
		p.pos++
		return p.object()
	case jsonLBrack:
		p.pos++
		return p.array()
	case jsonTrue:
		return true, p.literal("true")
	case jsonFalse:
		return false, p.literal("false")
	case jsonNull:
		return Null, p.literal("null")
	}
	return nil, p.unexpected(tok, "")
}

func (p *jsonParser) literal(word string) error {
	w := ToUTF16(word)
	if p.pos+len(w) <= len(p.src) && unitsHasPrefix(p.src[p.pos:], w) {
		p.pos += len(w)
		return nil
	}
	p.pos++
	for i := 1; i < len(w); i++ {
		if p.pos >= len(p.src) {
			return p.unexpected(jsonEOS, "")
		}
		if p.src[p.pos] != w[i] {
			return p.unexpectedChar("")
		}
		p.pos++
	}
	return p.unexpected(jsonEOS, "")
}

func (p *jsonParser) object() (Value, error) {
	obj := NewObject()
	if p.peek() == jsonRBrace {
		p.pos++
		return obj, nil
	}
	first := true
	for {
		msg := "Expected double-quoted property name"
		if first {
			msg = "Expected property name or '}'"
		}
		first = false
		if tok := p.peek(); tok != jsonString {
			return nil, p.unexpected(tok, msg)
		}
		p.pos++
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		if tok := p.peek(); tok != jsonColon {
			return nil, p.unexpected(tok, "Expected ':' after property name")
		}
		p.pos++
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		obj.Set(key.(string), v)
		tok := p.peek()
		if tok == jsonComma {
			p.pos++
			continue
		}
		if tok != jsonRBrace {
			return nil, p.unexpected(tok, "Expected ',' or '}' after property value")
		}
		p.pos++
		return obj, nil
	}
}

func (p *jsonParser) array() (Value, error) {
	arr := NewArray()
	if p.peek() == jsonRBrack {
		p.pos++
		return arr, nil
	}
	for {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		arr.Elems = append(arr.Elems, v)
		tok := p.peek()
		if tok == jsonComma {
			p.pos++
			continue
		}
		if tok != jsonRBrack {
			return nil, p.unexpected(tok, "Expected ',' or ']' after array element")
		}
		p.pos++
		return arr, nil
	}
}

func isDigitUnit(c uint16) bool {
	return c >= '0' && c <= '9'
}

func (p *jsonParser) number() (Value, error) {
	start := p.pos
	if p.src[p.pos] == '-' {
		p.pos++
		if p.pos >= len(p.src) {
			return nil, p.unexpected(jsonEOS, "No number after minus sign")
		}
	}
	switch {
	case p.src[p.pos] == '0':
		p.pos++
		if p.pos < len(p.src) && isDigitUnit(p.src[p.pos]) {
			return nil, p.unexpected(jsonNumber, "")
		}
	case !isDigitUnit(p.src[p.pos]):
		return nil, p.unexpectedChar("No number after minus sign")
	default:
		for p.pos < len(p.src) && isDigitUnit(p.src[p.pos]) {
			p.pos++
		}
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		if p.pos >= len(p.src) || !isDigitUnit(p.src[p.pos]) {
			return nil, p.unexpectedChar("Unterminated fractional number")
		}
		for p.pos < len(p.src) && isDigitUnit(p.src[p.pos]) {
			p.pos++
		}
	}
	if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.pos++
		}
		if p.pos >= len(p.src) || !isDigitUnit(p.src[p.pos]) {
			return nil, p.unexpectedChar("Exponent part is missing a number")
		}
		for p.pos < len(p.src) && isDigitUnit(p.src[p.pos]) {
			p.pos++
		}
	}
	f, _ := strconv.ParseFloat(FromUTF16(p.src[start:p.pos]), 64)
	return f, nil
}

func hexUnit(c uint16) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// str scans a string whose opening quote has been consumed.
func (p *jsonParser) str() (Value, error) {
	var out []uint16
	for {
		if p.pos >= len(p.src) {
			return nil, p.unexpected(jsonEOS, "Unterminated string")
		}
		c := p.src[p.pos]
		switch {
		case c == '"':
			p.pos++
			return FromUTF16(out), nil
		case c < 0x20:
			return nil, p.unexpected(jsonIllegal, "Bad control character in string literal")
		case c == '\\':
			p.pos++
			if p.pos >= len(p.src) {
				return nil, p.unexpected(jsonEOS, "Unterminated string")
			}
			e := p.src[p.pos]
			switch e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				v := 0
				for k := 1; k <= 4; k++ {
					if p.pos+k >= len(p.src) || hexUnit(p.src[p.pos+k]) < 0 {
						p.pos += k
						if p.pos >= len(p.src) {
							return nil, p.unexpected(jsonEOS, "Bad Unicode escape")
						}
						return nil, p.unexpected(jsonIllegal, "Bad Unicode escape")
					}
					v = v*16 + hexUnit(p.src[p.pos+k])
				}
				out = append(out, uint16(v))
				p.pos += 4
			default:
				return nil, p.unexpected(jsonIllegal, "Bad escaped character")
			}
			p.pos++
		default:
			out = append(out, c)
			p.pos++
		}
	}
}

// Stringify implements JSON.stringify(value, null, space) for the values of this package; ok is false when the
// result is undefined.
func Stringify(v Value, space Value) (string, bool, error) {
	gap := ""
	switch s := space.(type) {
	case float64:
		n := int(math.Min(10, ToIntegerOrInfinity(s)))
		if n > 0 {
			gap = strings.Repeat(" ", n)
		}
	case string:
		u := ToUTF16(s)
		if len(u) > 10 {
			u = u[:10]
		}
		gap = FromUTF16(u)
	}
	st := &stringifier{gap: gap}
	return st.serialize(v, "")
}

type stringifier struct {
	gap   string
	stack []Value
}

func (st *stringifier) serialize(v Value, indent string) (string, bool, error) {
	switch x := v.(type) {
	case NullType:
		return "null", true, nil
	case bool:
		if x {
			return "true", true, nil
		}
		return "false", true, nil
	case string:
		return QuoteJSON(x), true, nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return "null", true, nil
		}
		return NumberToString(x), true, nil
	case *Array:
		s, err := st.serializeArray(x, indent)
		return s, true, err
	case *Object:
		s, err := st.serializeObject(x, indent)
		return s, true, err
	}
	return "", false, nil
}

func (st *stringifier) enter(v Value) error {
	for _, s := range st.stack {
		if s == v {
			return typeError("Converting circular structure to JSON")
		}
	}
	st.stack = append(st.stack, v)
	return nil
}

func (st *stringifier) serializeObject(o *Object, indent string) (string, error) {
	if err := st.enter(o); err != nil {
		return "", err
	}
	defer func() { st.stack = st.stack[:len(st.stack)-1] }()
	inner := indent + st.gap
	var parts []string
	for _, k := range o.Keys() {
		val, _ := o.Get(k)
		s, ok, err := st.serialize(val, inner)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		sep := ":"
		if st.gap != "" {
			sep = ": "
		}
		parts = append(parts, QuoteJSON(k)+sep+s)
	}
	return st.wrap("{", "}", parts, indent, inner), nil
}

func (st *stringifier) serializeArray(a *Array, indent string) (string, error) {
	if err := st.enter(a); err != nil {
		return "", err
	}
	defer func() { st.stack = st.stack[:len(st.stack)-1] }()
	inner := indent + st.gap
	parts := make([]string, 0, len(a.Elems))
	for _, e := range a.Elems {
		s, ok, err := st.serialize(e, inner)
		if err != nil {
			return "", err
		}
		if !ok {
			s = "null"
		}
		parts = append(parts, s)
	}
	return st.wrap("[", "]", parts, indent, inner), nil
}

func (st *stringifier) wrap(open, close string, parts []string, indent, inner string) string {
	if len(parts) == 0 {
		return open + close
	}
	if st.gap == "" {
		return open + strings.Join(parts, ",") + close
	}
	return open + "\n" + inner + strings.Join(parts, ",\n"+inner) + "\n" + indent + close
}

// QuoteJSON quotes a string as JSON.stringify does, escaping lone surrogates.
func QuoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	u := ToUTF16(s)
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			switch {
			case c < 0x20:
				fmt.Fprintf(&b, `\u%04x`, c)
			case c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF:
				b.WriteString(FromUTF16(u[i : i+2]))
				i++
			case c >= 0xD800 && c <= 0xDFFF:
				fmt.Fprintf(&b, `\u%04x`, c)
			default:
				b.WriteString(FromUTF16(u[i : i+1]))
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
