// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-lexer); MIT licence, see LICENSE.

package lexer

import (
	"regexp"
	"strings"

	"github.com/pugjs/pug/internal/charparser"
	"github.com/pugjs/pug/internal/js"
)

func isWS(c uint16) bool {
	return c == ' ' || c == '\n' || c == '\t'
}

func isQuote(c uint16) bool {
	return c == '\'' || c == '"'
}

// unit returns u[i], or -1 past the end (an undefined character).
func unit(u []uint16, i int) int {
	if i < 0 || i >= len(u) {
		return -1
	}
	return int(u[i])
}

// attribute lexes one attribute from the start of str and returns the rest of str.
func (l *Lexer) attribute(src string) (string, error) {
	str := js.ToUTF16(src)
	i := 0
	for ; i < len(str); i++ {
		if !isWS(str[i]) {
			break
		}
		if str[i] == '\n' {
			l.incrementLine(1)
		} else {
			l.incrementColumn(1)
		}
	}
	if i == len(str) {
		return "", nil
	}
	t := l.tok("attribute")
	var quote uint16
	if isQuote(str[i]) {
		quote = str[i]
		l.incrementColumn(1)
		i++
	}
	var key []uint16
	for ; i < len(str); i++ {
		if quote != 0 {
			if str[i] == quote {
				l.incrementColumn(1)
				i++
				break
			}
		} else if isWS(str[i]) || str[i] == '!' || str[i] == '=' || str[i] == ',' {
			break
		}
		key = append(key, str[i])
		if str[i] == '\n' {
			l.incrementLine(1)
		} else {
			l.incrementColumn(1)
		}
	}
	t.Name = js.FromUTF16(key)
	val, mustEscape, remaining, err := l.attributeValue(str[i:])
	if err != nil {
		return "", err
	}
	if val != "" {
		t.AttrVal = val
		t.MustEscape = mustEscape
	} else {
		t.AttrTrue = true
		t.MustEscape = true
	}
	str = remaining
	l.push(t)
	for i = 0; i < len(str); i++ {
		if !isWS(str[i]) {
			break
		}
		if str[i] == '\n' {
			l.incrementLine(1)
		} else {
			l.incrementColumn(1)
		}
	}
	if unit(str, i) == ',' {
		l.incrementColumn(1)
		i++
	}
	return js.FromUTF16(str[i:]), nil
}

// attributeValue lexes the optional =value or !=value after an attribute name. It returns the value expression
// (empty for none), whether it is escaped, and the rest of the source.
func (l *Lexer) attributeValue(str []uint16) (string, bool, []uint16, error) {
	escapeAttr := true
	st := &charparser.State{}
	col, line := l.colno, l.lineno
	i := 0
	for ; i < len(str); i++ {
		if !isWS(str[i]) {
			break
		}
		if str[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	if i == len(str) {
		return "", true, str, nil
	}
	if str[i] == '!' {
		escapeAttr = false
		col++
		i++
		if unit(str, i) != '=' {
			return "", false, nil, l.error("INVALID_KEY_CHARACTER", "Unexpected character "+charText(str, i)+" expected `=`")
		}
	}
	if unit(str, i) != '=' {
		if i == 0 && len(str) > 0 && !isWS(str[0]) && str[0] != ',' {
			return "", false, nil, l.error("INVALID_KEY_CHARACTER", "Unexpected character "+charText(str, 0)+" expected `=`")
		}
		return "", true, str, nil
	}
	l.lineno = line
	l.colno = col + 1
	i++
	for ; i < len(str); i++ {
		if !isWS(str[i]) {
			break
		}
		if str[i] == '\n' {
			l.incrementLine(1)
		} else {
			l.incrementColumn(1)
		}
	}
	line, col = l.lineno, l.colno
	var val []uint16
	for ; i < len(str); i++ {
		if !(st.IsNesting() || st.IsString()) {
			if isWS(str[i]) {
				done := false
				x := i
				for ; x < len(str); x++ {
					if !isWS(str[x]) {
						isNotPunctuator := !charparser.IsPunctuator(int(str[x]))
						isColon := str[x] == ':'
						isSpread := unit(str, x) == '.' && unit(str, x+1) == '.' && unit(str, x+2) == '.'
						if isNotPunctuator || isQuote(str[x]) || isColon || isSpread {
							if ok, _ := l.assertExpression(js.FromUTF16(val), true); ok {
								done = true
							}
						}
						break
					}
				}
				if done || x == len(str) {
					break
				}
			}
			if str[i] == ',' {
				if ok, _ := l.assertExpression(js.FromUTF16(val), true); ok {
					break
				}
			}
		}
		if err := st.ParseChar(str[i]); err != nil {
			return "", false, nil, parserError(err)
		}
		val = append(val, str[i])
		if str[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	if _, err := l.assertExpression(js.FromUTF16(val), false); err != nil {
		return "", false, nil, err
	}
	l.lineno = line
	l.colno = col
	return js.FromUTF16(val), escapeAttr, str[i:], nil
}

// charText renders str[i] as JavaScript string concatenation does: "undefined" past the end.
func charText(str []uint16, i int) string {
	if i >= len(str) {
		return "undefined"
	}
	return js.FromUTF16(str[i : i+1])
}

func (l *Lexer) attrs() (bool, error) {
	if !strings.HasPrefix(l.input, "(") {
		return false, nil
	}
	t := l.tok("start-attributes")
	r, err := l.bracketExpression(0)
	if err != nil {
		return false, err
	}
	str := js.Slice(l.input, 1, r.End)
	l.incrementColumn(1)
	l.push(t)
	if err := l.assertNestingCorrect(str); err != nil {
		return false, err
	}
	l.consume(r.End + 1)
	for str != "" {
		if str, err = l.attribute(str); err != nil {
			return false, err
		}
	}
	t = l.tok("end-attributes")
	l.incrementColumn(1)
	l.push(t)
	return true, nil
}

var attributesBlockRe = regexp.MustCompile(`^&attributes\b`)

func (l *Lexer) attributesBlock() (bool, error) {
	if !attributesBlockRe.MatchString(l.input) {
		return false, nil
	}
	l.consume(11)
	t := l.tok("&attributes")
	l.incrementColumn(11)
	r, err := l.bracketExpression(0)
	if err != nil {
		return false, err
	}
	l.consume(r.End + 1)
	t.Val, t.HasVal = r.Src, true
	l.incrementColumn(r.End + 1)
	l.push(t)
	return true, nil
}

func (l *Lexer) indent() (bool, error) {
	captures := l.scanIndentation()
	if captures == nil {
		return false, nil
	}
	indents := len(captures[1])
	l.incrementLine(1)
	l.consume(indents + 1)
	if strings.HasPrefix(l.input, " ") || strings.HasPrefix(l.input, "\t") {
		return false, l.error("INVALID_INDENTATION", "Invalid indentation, you can use tabs or spaces but not both")
	}
	if strings.HasPrefix(l.input, "\n") {
		// A blank line: the original returns the token without pushing it.
		l.interpolationAllowed = true
		return true, nil
	}
	switch {
	case indents < l.indentStack[0]:
		outdents := 0
		for l.indentStack[0] > indents {
			if len(l.indentStack) > 1 && l.indentStack[1] < indents {
				return false, l.error("INCONSISTENT_INDENTATION", "Inconsistent indentation. Expecting either "+
					itoa(l.indentStack[1])+" or "+itoa(l.indentStack[0])+" spaces/tabs.")
			}
			outdents++
			l.indentStack = l.indentStack[1:]
		}
		for ; outdents > 0; outdents-- {
			l.colno = 1
			t := l.tok("outdent")
			l.colno = l.indentStack[0] + 1
			l.push(t)
		}
	case indents != 0 && indents != l.indentStack[0]:
		t := l.tok("indent")
		t.Indent = indents
		l.colno = 1 + indents
		l.push(t)
		l.indentStack = append([]int{indents}, l.indentStack...)
	default:
		t := l.tok("newline")
		l.colno = 1 + min(l.indentStack[0], indents)
		l.push(t)
	}
	l.interpolationAllowed = true
	return true, nil
}

func itoa(n int) string {
	return js.NumberToString(float64(n))
}

// pipelessText lexes an indented text block (after a dot, a filter, a comment or a code dash).
func (l *Lexer) pipelessText(indents int) (bool, error) {
	for l.blank() {
	}
	captures := l.scanIndentation()
	if indents == 0 && captures != nil {
		indents = len(captures[1])
	}
	if indents <= l.indentStack[0] {
		return false, nil
	}
	l.push(l.tok("start-pipeless-text"))
	var lines []string
	var tokenIndent []bool
	ptr := 0 // byte offset in l.input
	isMatch := true
	for {
		rest := l.input[min(ptr+1, len(l.input)):]
		i := strings.IndexByte(rest, '\n')
		if i == -1 {
			i = len(l.input) - ptr - 1
		}
		if i < 0 {
			i = 0
		}
		str := rest[:min(i, len(rest))]
		lineCaptures := l.indentRe.FindStringSubmatch("\n" + str)
		lineIndents := 0
		if lineCaptures != nil {
			lineIndents = len(lineCaptures[1])
		}
		isMatch = lineIndents >= indents
		tokenIndent = append(tokenIndent, isMatch)
		isMatch = isMatch || js.TrimSpace(str) == ""
		if isMatch {
			ptr += len(str) + 1
			lines = append(lines, js.From(str, indents))
		} else if lineIndents > l.indentStack[0] {
			// The line is indented less than the first line but more than the block: lex the block again with
			// that indentation.
			l.tokens = l.tokens[:len(l.tokens)-1]
			return l.pipelessText(len(lineCaptures[1]))
		}
		if !(len(l.input)-ptr > 0 && isMatch) {
			break
		}
	}
	l.input = l.input[min(ptr, len(l.input)):]
	for l.input == "" && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		l.incrementLine(1)
		var t Token
		if i != 0 {
			t = l.tok("newline")
		}
		if tokenIndent[i] {
			l.incrementColumn(indents)
		}
		if i != 0 {
			l.push(t)
		}
		if err := l.addText("text", line); err != nil {
			return false, err
		}
	}
	l.push(l.tok("end-pipeless-text"))
	return true, nil
}

var (
	slashRe = regexp.MustCompile(`^/`)
	colonRe = regexp.MustCompile(`^: +`)
)

func (l *Lexer) fail() error {
	return l.error("UNEXPECTED_TEXT", `unexpected text "`+js.Slice(l.input, 0, 5)+`"`)
}

func (l *Lexer) advance() error {
	steps := []func() (bool, error){
		func() (bool, error) { return l.blank(), nil },
		l.eos,
		func() (bool, error) { return l.endInterpolation(), nil },
		func() (bool, error) { return l.endOfLineToken(yieldRe, "yield"), nil },
		func() (bool, error) { return l.doctype(), nil },
		l.interpolation,
		l.caseToken,
		l.when,
		l.defaultToken,
		l.extends,
		func() (bool, error) { return l.namedBlock(appendRe, "append"), nil },
		func() (bool, error) { return l.namedBlock(prependRe, "prepend"), nil },
		func() (bool, error) { return l.namedBlock(blockRe, "replace"), nil },
		func() (bool, error) { return l.endOfLineToken(mixinBlockRe, "mixin-block"), nil },
		l.include,
		func() (bool, error) { return l.mixin(), nil },
		l.call,
		l.conditional,
		l.eachOf,
		l.each,
		l.while,
		func() (bool, error) { return l.tag(), nil },
		func() (bool, error) { return l.filter(false) },
		l.blockCode,
		l.code,
		l.id,
		l.dot,
		l.className,
		l.attrs,
		l.attributesBlock,
		l.indent,
		l.text,
		l.textHTML,
		l.comment,
		func() (bool, error) {
			t, ok := l.scan(slashRe, "slash")
			if ok {
				l.push(t)
			}
			return ok, nil
		},
		func() (bool, error) {
			t, ok := l.scan(colonRe, ":")
			if ok {
				l.push(t)
			}
			return ok, nil
		},
	}
	for _, step := range steps {
		ok, err := step()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return l.fail()
}
