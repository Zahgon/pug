package main

import (
	filters "github.com/pugjs/pug/packages/pug-filters"

	"github.com/pugjs/pug/internal/js"
)

// filterSet holds the filters the command makes available to templates.
var filterSet = map[string]filters.Filter{
	// cdata wraps the text in a CDATA section.
	"cdata": func(text string, _ *js.Object) (string, error) {
		return "<![CDATA[" + text + "]]>", nil
	},
	// upper and lower change the case of the text as JavaScript's toUpperCase and toLowerCase do.
	"upper": func(text string, _ *js.Object) (string, error) {
		return js.ToUpperCase(text), nil
	},
	"lower": func(text string, _ *js.Object) (string, error) {
		return js.ToLowerCase(text), nil
	},
	// wrap wraps the text in an element: the tag option (default div) and an optional class option.
	"wrap": func(text string, options *js.Object) (string, error) {
		tag := "div"
		if v, ok := options.Get("tag"); ok && v != js.Undefined {
			s, err := js.ToString(v)
			if err != nil {
				return "", err
			}
			tag = s
		}
		class := ""
		if v, ok := options.Get("class"); ok && v != js.Undefined {
			s, err := js.ToString(v)
			if err != nil {
				return "", err
			}
			class = ` class="` + s + `"`
		}
		return js.Concat("<"+tag+class+">", text, "</"+tag+">"), nil
	},
	// length replaces the text with its length in UTF-16 code units.
	"length": func(text string, _ *js.Object) (string, error) {
		return js.NumberToString(float64(js.Length(text))), nil
	},
}
