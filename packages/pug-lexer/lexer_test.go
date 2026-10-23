// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-lexer); MIT licence, see LICENSE.

package lexer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	pugerror "github.com/pugjs/pug/packages/pug-error"
)

// The upstream suite records the tokens of test/cases and the errors of test/errors as jest snapshots; these tests
// render the Go results in the same pretty-format text and compare them with the recorded snapshots.

var snapshotRe = regexp.MustCompile("(?s)exports\\[`([^`]+) 1`\\] = `\n(.*?)\n`;")

func loadSnapshots(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("test/__snapshots__/index.test.js.snap")
	if err != nil {
		t.Fatal(err)
	}
	unescape := strings.NewReplacer("\\\\", "\\", "\\`", "`", "\\${", "${")
	out := map[string]string{}
	for _, m := range snapshotRe.FindAllStringSubmatch(string(data), -1) {
		out[m[1]] = unescape.Replace(m[2])
	}
	return out
}

// value is a JavaScript value in pretty-format: a string, number, bool, nil (null) or map.
type value any

func format(v value, indent string) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return `"` + strings.NewReplacer(`"`, `\"`, `\`, `\\`).Replace(x) + `"`
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case []value:
		if len(x) == 0 {
			return "Array []"
		}
		var b strings.Builder
		b.WriteString("Array [\n")
		for _, e := range x {
			b.WriteString(indent + "  " + format(e, indent+"  ") + ",\n")
		}
		return b.String() + indent + "]"
	case map[string]value:
		if len(x) == 0 {
			return "Object {}"
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("Object {\n")
		for _, k := range keys {
			b.WriteString(indent + "  " + strconv.Quote(k) + ": " + format(x[k], indent+"  ") + ",\n")
		}
		return b.String() + indent + "}"
	}
	panic(fmt.Sprintf("unexpected value %T", v))
}

func optional(s string, ok bool) value {
	if !ok {
		return nil
	}
	return s
}

// tokenValue renders a token with the fields the original's token of that type has.
func tokenValue(t Token) value {
	loc := map[string]value{
		"start": map[string]value{"line": t.Loc.Start.Line, "column": t.Loc.Start.Column},
		"end":   map[string]value{"line": t.Loc.End.Line, "column": t.Loc.End.Column},
	}
	if t.Loc.Filename != "" {
		loc["filename"] = t.Loc.Filename
	}
	m := map[string]value{"type": t.Type, "loc": loc}
	switch t.Type {
	case "attribute":
		m["name"] = t.Name
		m["mustEscape"] = t.MustEscape
		if t.AttrTrue {
			m["val"] = true
		} else {
			m["val"] = t.AttrVal
		}
		return m
	case "indent":
		m["val"] = t.Indent
		return m
	case "comment":
		m["buffer"] = t.Buffer
	case "code", "interpolated-code":
		m["buffer"] = t.Buffer
		m["mustEscape"] = t.MustEscape
	case "block":
		m["mode"] = t.Mode
	case "each":
		m["key"] = optional(t.Key, t.HasKey)
		m["code"] = t.Code
	case "eachOf":
		m["value"] = t.Value
		m["code"] = t.Code
	case "call", "mixin":
		m["args"] = optional(t.Args, t.HasArgs)
	}
	if t.HasVal {
		m["val"] = t.Val
	}
	return m
}

// needsFullJavaScript lists cases whose tokens depend on validating JavaScript outside the template subset (ES2015
// syntax and regular expressions), which only a complete JavaScript parser decides.
var needsFullJavaScript = map[string]bool{
	"attrs-data.pug":     true, // new Date(0)
	"attrs.js.pug":       true, // a function declaration
	"attrs.pug":          true, // new Date(0)
	"regression.784.pug": true, // a regular expression literal
}

func TestTokenSnapshots(t *testing.T) {
	snaps := loadSnapshots(t)
	files, _ := filepath.Glob("test/cases/*.pug")
	if len(files) == 0 {
		t.Fatal("no lexer cases found")
	}
	for _, f := range files {
		name := filepath.Base(f)
		if needsFullJavaScript[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			tokens, err := Lex(string(src), Options{Filename: "<basedir>/packages/pug-lexer/test/cases/" + name})
			if err != nil {
				t.Fatalf("lex: %v", err)
			}
			list := make([]value, len(tokens))
			for i, tok := range tokens {
				list[i] = tokenValue(tok)
			}
			if got, want := format(list, ""), snaps[name]; got != want {
				t.Errorf("tokens differ from the snapshot\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestErrorSnapshots(t *testing.T) {
	snaps := loadSnapshots(t)
	files, _ := filepath.Glob("test/errors/*.pug")
	if len(files) == 0 {
		t.Fatal("no lexer error cases found")
	}
	for _, f := range files {
		name := filepath.Base(f)
		want := snaps[name]
		if strings.Contains(want, "PUG:SYNTAX_ERROR") {
			// The text of JavaScript syntax errors comes from a full JavaScript parser.
			continue
		}
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Lex(string(src), Options{Filename: f})
			var pe *pugerror.Error
			if !errors.As(err, &pe) {
				t.Fatalf("expected a pug error, got %v", err)
			}
			got := format(map[string]value{"msg": pe.Msg, "code": pe.Code, "line": pe.Line, "column": pe.Column}, "")
			if got != want {
				t.Errorf("error differs from the snapshot\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
