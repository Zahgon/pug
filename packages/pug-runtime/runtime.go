// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-runtime); MIT licence, see LICENSE.

// Package runtime holds the helpers compiled templates call while rendering: HTML escaping, attribute rendering
// and merging, class and style serialisation, and the decoration of runtime errors with template context. They
// work on JavaScript values and keep the original's coercions.
package runtime

import (
	"os"
	"strconv"
	"strings"

	"github.com/pugjs/pug/internal/js"
)

func concat(a, b js.Value) (js.Value, error) {
	return js.Add(a, b)
}

// Escape implements pug.escape: the value converted to a string with ", &, < and > replaced by entities; a value
// whose string form needs no escaping is returned unchanged (not converted).
func Escape(v js.Value) (js.Value, error) {
	html, err := concat("", v)
	if err != nil {
		return nil, err
	}
	s := html.(string)
	if !strings.ContainsAny(s, `"&<>`) {
		return v, nil
	}
	return EscapeString(s), nil
}

// EscapeString escapes a string.
func EscapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			b.WriteString("&quot;")
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// ForIn returns the keys a for-in loop visits on v.
func ForIn(v js.Value) []string {
	switch x := v.(type) {
	case *js.Object:
		return x.Keys()
	case *js.Array:
		keys := make([]string, len(x.Elems))
		for i := range x.Elems {
			keys[i] = strconv.Itoa(i)
		}
		return keys
	case string:
		n := js.Length(x)
		keys := make([]string, n)
		for i := range keys {
			keys[i] = strconv.Itoa(i)
		}
		return keys
	}
	return nil
}

// hasOwn reports Object.prototype.hasOwnProperty.call(v, key) for the keys ForIn returns.
func hasOwn(v js.Value, key string) bool {
	switch x := v.(type) {
	case *js.Object:
		return x.Has(key)
	case *js.Array, string:
		return true
	}
	return false
}

func get(v js.Value, key string) js.Value {
	r, err := js.GetMember(v, key)
	if err != nil {
		return js.Undefined
	}
	return r
}

// Merge implements pug.merge on a list: every later object is merged into the first, which is modified.
func Merge(list []js.Value) (js.Value, error) {
	attrs := list[0]
	for _, b := range list[1:] {
		var err error
		if attrs, err = merge(attrs, b); err != nil {
			return nil, err
		}
	}
	return attrs, nil
}

func merge(a, b js.Value) (js.Value, error) {
	for _, key := range ForIn(b) {
		bv := get(b, key)
		switch key {
		case "class":
			va := get(a, "class")
			if !js.Truthy(va) {
				va = js.NewArray()
			}
			var list *js.Array
			if arr, ok := va.(*js.Array); ok {
				list = js.NewArray(append([]js.Value(nil), arr.Elems...)...)
			} else {
				list = js.NewArray(va)
			}
			if !js.Truthy(bv) {
				bv = js.NewArray()
			}
			if arr, ok := bv.(*js.Array); ok {
				list.Elems = append(list.Elems, arr.Elems...)
			} else {
				list.Elems = append(list.Elems, bv)
			}
			if err := js.SetMember(a, key, list); err != nil {
				return nil, err
			}
		case "style":
			sa, err := withSemicolon(get(a, "style"))
			if err != nil {
				return nil, err
			}
			sb, err := withSemicolon(bv)
			if err != nil {
				return nil, err
			}
			if err := js.SetMember(a, key, js.Concat(sa, sb)); err != nil {
				return nil, err
			}
		default:
			if err := js.SetMember(a, key, bv); err != nil {
				return nil, err
			}
		}
	}
	return a, nil
}

func withSemicolon(v js.Value) (string, error) {
	s, err := Style(v)
	if err != nil {
		return "", err
	}
	if s != "" && !strings.HasSuffix(s, ";") {
		s += ";"
	}
	return s, nil
}

// Classes implements pug.classes: arrays (with per-item escaping flags), objects of truthy flags and strings
// become a space-separated class list.
func Classes(v js.Value, escaping []bool) (js.Value, error) {
	switch x := v.(type) {
	case *js.Array:
		return classesArray(x, escaping)
	case *js.Object:
		return classesObject(x), nil
	}
	if !js.Truthy(v) {
		return "", nil
	}
	return v, nil
}

func classesArray(a *js.Array, escaping []bool) (js.Value, error) {
	var out js.Value = ""
	padding := ""
	for i, item := range a.Elems {
		name, err := Classes(item, nil)
		if err != nil {
			return nil, err
		}
		if !js.Truthy(name) {
			continue
		}
		if escaping != nil && i < len(escaping) && escaping[i] {
			if name, err = Escape(name); err != nil {
				return nil, err
			}
		}
		if out, err = concat(out, padding); err != nil {
			return nil, err
		}
		if out, err = concat(out, name); err != nil {
			return nil, err
		}
		padding = " "
	}
	return out, nil
}

func classesObject(o *js.Object) js.Value {
	var b strings.Builder
	padding := ""
	for _, key := range o.Keys() {
		v, _ := o.Get(key)
		if key != "" && js.Truthy(v) {
			b.WriteString(padding + key)
			padding = " "
		}
	}
	return b.String()
}

// Style implements pug.style: an object becomes name:value; pairs, anything else its string form.
func Style(v js.Value) (string, error) {
	if !js.Truthy(v) {
		return "", nil
	}
	switch v.(type) {
	case *js.Object, *js.Array:
		var out js.Value = ""
		for _, key := range ForIn(v) {
			if !hasOwn(v, key) {
				continue
			}
			var err error
			if out, err = concat(out, key+":"); err != nil {
				return "", err
			}
			if out, err = concat(out, get(v, key)); err != nil {
				return "", err
			}
			if out, err = concat(out, ";"); err != nil {
				return "", err
			}
		}
		return out.(string), nil
	}
	s, err := concat(v, "")
	if err != nil {
		return "", err
	}
	return s.(string), nil
}

// Attr implements pug.attr: one rendered attribute (with its leading space), or nothing for false, null and
// undefined (and for an empty class or style).
func Attr(key string, v js.Value, escaped, terse bool) (string, error) {
	switch x := v.(type) {
	case bool:
		if !x {
			return "", nil
		}
		if terse {
			return " " + key, nil
		}
		return " " + key + `="` + key + `"`, nil
	case js.UndefinedType, js.NullType:
		return "", nil
	}
	if !js.Truthy(v) && (key == "class" || key == "style") {
		return "", nil
	}
	s, isString := v.(string)
	if !isString {
		j, ok, err := js.Stringify(v, js.Undefined)
		if err != nil {
			return "", err
		}
		if !ok {
			// JSON.stringify returns undefined for functions; the original then fails on indexOf.
			return "", &js.Error{Name: "TypeError", Message: "Cannot read properties of undefined (reading 'indexOf')"}
		}
		s = j
		if !escaped && strings.Contains(s, `"`) {
			return " " + key + "='" + strings.ReplaceAll(s, "'", "&#39;") + "'", nil
		}
	}
	if escaped {
		s = EscapeString(s)
	}
	return " " + key + `="` + s + `"`, nil
}

// Attrs implements pug.attrs: every own property of an object rendered without escaping, the class first.
func Attrs(v js.Value, terse bool) (string, error) {
	out := ""
	for _, key := range ForIn(v) {
		if !hasOwn(v, key) {
			continue
		}
		val := get(v, key)
		switch key {
		case "class":
			c, err := Classes(val, nil)
			if err != nil {
				return "", err
			}
			a, err := Attr(key, c, false, terse)
			if err != nil {
				return "", err
			}
			out = a + out
			continue
		case "style":
			s, err := Style(val)
			if err != nil {
				return "", err
			}
			val = s
		}
		a, err := Attr(key, val, false, terse)
		if err != nil {
			return "", err
		}
		out = js.Concat(out, a)
	}
	return out, nil
}

// Rethrow adds the template context to a runtime error: the file name and line, the lines around it read from
// the file, and the original message, as pug.rethrow does.
func Rethrow(err *js.Error, filename string, hasFile bool, line int, hasLine bool) *js.Error {
	lineText := "undefined"
	if hasLine {
		lineText = strconv.Itoa(line)
	}
	if !hasFile || filename == "" {
		return &js.Error{Name: err.Name, Message: err.Message + " on line " + lineText}
	}
	data, rerr := os.ReadFile(filename)
	if rerr != nil {
		msg := err.Message + " - could not read from " + filename + " (" + readError(filename, rerr) + ")"
		return &js.Error{Name: err.Name, Message: msg + " on line " + lineText}
	}
	lines := strings.Split(js.DecodeUTF8(data), "\n")
	start := max(line-3, 0)
	end := min(len(lines), line+3)
	var ctx []string
	for i := start; i < end; i++ {
		curr := i + 1
		mark := "    "
		if curr == line {
			mark = "  > "
		}
		ctx = append(ctx, mark+strconv.Itoa(curr)+"| "+lines[i])
	}
	return &js.Error{Name: err.Name, Message: filename + ":" + lineText + "\n" + strings.Join(ctx, "\n") + "\n\n" + err.Message}
}

func readError(name string, err error) string {
	if os.IsNotExist(err) {
		return "ENOENT: no such file or directory, open '" + name + "'"
	}
	return err.Error()
}
