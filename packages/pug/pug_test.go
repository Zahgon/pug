// Copyright (c) 2009-2014 TJ Holowaychuk (pugjs/pug); MIT licence, see LICENSE.

package pug

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	filters "github.com/pugjs/pug/packages/pug-filters"

	"github.com/pugjs/pug/internal/js"
)

// testFilters mirrors the filters of the upstream suite: custom checks its constant options.
var testFilters = map[string]filters.Filter{
	"custom": func(text string, o *js.Object) (string, error) {
		opt, _ := o.Get("opt")
		num, _ := o.Get("num")
		if opt != "val" || num != 2.0 {
			return "", errors.New("custom filter: unexpected options")
		}
		return "BEGIN" + text + "END", nil
	},
}

// skipped cases use what the Go port leaves out: JavaScript beyond the template subset (functions, new, regular
// expressions, statements split across braces) or filters provided by JSTransformer packages.
var skipped = map[string]string{
	"attrs-data":           "new Date",
	"attrs.js":             "a function declaration",
	"attrs":                "new Date",
	"code.conditionals":    "statements split across braces",
	"code.iteration":       "function expressions",
	"each.else":            "Object.create",
	"regression.784":       "a regular expression literal",
	"filter-in-include":    "the markdown-it filter",
	"filters-empty":        "the cdata filter",
	"filters.coffeescript": "the coffee-script filter",
	"filters.include":      "the markdown-it and coffee-script filters",
	"filters.inline":       "the cdata filter",
	"filters.less":         "the less filter",
	"filters.markdown":     "the markdown-it filter",
	"filters.nested":       "the uglify-js and coffee-script filters",
	"filters.stylus":       "the stylus filter",
	"includes":             "the markdown-it and coffee-script filters",
	"pipeless-filters":     "the markdown-it filter",
}

var whitespaceRe = regexp.MustCompile(`\n| `)

func TestCases(t *testing.T) {
	files, _ := filepath.Glob("test/cases/*.pug")
	if len(files) == 0 {
		t.Fatal("no cases found")
	}
	locals := js.NewObject()
	locals.Set("title", "Pug")
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".pug")
		if _, ok := skipped[name]; ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			tmpl, err := CompileFile(f, Options{Pretty: "  ", PrettySet: true, Basedir: "test/cases", Filters: testFilters})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got, err := tmpl.Render(locals)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			data, err := os.ReadFile(strings.TrimSuffix(f, ".pug") + ".html")
			if err != nil {
				t.Fatal(err)
			}
			want := strings.ReplaceAll(strings.TrimSpace(string(data)), "\r", "")
			got = strings.TrimSpace(got)
			if strings.Contains(name, "filter") {
				got, want = whitespaceRe.ReplaceAllString(got, ""), whitespaceRe.ReplaceAllString(want, "")
			}
			if got != want {
				t.Errorf("output differs\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestAntiCases checks that every template of test/anti-cases fails to compile or render.
func TestAntiCases(t *testing.T) {
	files, _ := filepath.Glob("test/anti-cases/*.pug")
	if len(files) == 0 {
		t.Fatal("no anti-cases found")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			tmpl, err := CompileFile(f, Options{Basedir: "test/anti-cases"})
			if err == nil {
				_, err = tmpl.Render(js.NewObject())
			}
			if err == nil {
				t.Errorf("expected %s to fail", f)
			}
		})
	}
}
