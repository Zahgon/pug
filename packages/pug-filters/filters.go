// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-filters); MIT licence, see LICENSE.

// Package filters applies filters at compile time, as pug-filters does: :name blocks, nested filters and
// include:name files become text. Filters come from the caller; any other name is an unknown filter.
package filters

import (
	"errors"
	"strings"

	pugerror "github.com/pugjs/pug/packages/pug-error"
	parser "github.com/pugjs/pug/packages/pug-parser"
	walk "github.com/pugjs/pug/packages/pug-walk"

	"github.com/pugjs/pug/internal/js"
)

// Filter transforms the text of a filter block or an included file. options holds the filter's attributes
// (constants) and filename.
type Filter func(text string, options *js.Object) (string, error)

// Handle replaces every filter in ast, and in the files it includes or extends, by its output.
func Handle(ast *parser.Node, filters map[string]Filter) (*parser.Node, error) {
	_, err := walk.Walk(ast, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		switch {
		case n.Type == "Filter":
			if err := handleNested(n, filters); err != nil {
				return false, err
			}
			var text strings.Builder
			for _, c := range n.Block.Nodes {
				text.WriteString(c.Val)
			}
			attrs, err := attributes(n)
			if err != nil {
				return false, err
			}
			attrs.Set("filename", optional(n.Filename))
			n.Type = "Text"
			n.Val, n.HasVal = "", true
			out, err := apply(filters, n, text.String(), attrs)
			if err != nil {
				return false, err
			}
			n.Val = out
		case n.Type == "RawInclude" && len(n.Filters) > 0:
			first := n.Filters[len(n.Filters)-1]
			rest := n.Filters[:len(n.Filters)-1]
			attrs, err := attributes(first)
			if err != nil {
				return false, err
			}
			filename := n.File.FullPath
			attrs.Set("filename", filename)
			n.Type = "Text"
			out, err := apply(filters, first, n.File.Str, attrs)
			if err != nil {
				return false, err
			}
			for i := len(rest) - 1; i >= 0; i-- {
				a, err := attributes(rest[i])
				if err != nil {
					return false, err
				}
				a.Set("filename", filename)
				if out, err = apply(filters, rest[i], out, a); err != nil {
					return false, err
				}
			}
			n.Val, n.HasVal = out, true
			n.Filters = nil
			n.File = nil
		}
		return true, nil
	}, nil, &walk.Options{IncludeDependencies: true})
	if err != nil {
		return nil, err
	}
	return ast, nil
}

func optional(s string) js.Value {
	if s == "" {
		return js.Undefined
	}
	return s
}

func handleNested(n *parser.Node, filters map[string]Filter) error {
	if len(n.Block.Nodes) > 0 && n.Block.Nodes[0].Type == "Filter" {
		if _, err := Handle(n.Block, filters); err != nil {
			return err
		}
	}
	return nil
}

// apply runs the filter named by node f; an unknown name is a pug error at the filter.
func apply(filters map[string]Filter, f *parser.Node, text string, attrs *js.Object) (string, error) {
	if fn, ok := filters[f.Name]; ok {
		return fn(text, attrs)
	}
	return "", pugerror.New("UNKNOWN_FILTER", `unknown filter ":`+f.Name+`"`,
		pugerror.Options{Line: f.Line, Column: f.Column, Filename: f.Filename})
}

// attributes evaluates the attributes of a filter, which must be constants.
func attributes(n *parser.Node) (*js.Object, error) {
	attrs := js.NewObject()
	for _, a := range n.Attrs {
		if a.True {
			attrs.Set(a.Name, true)
			continue
		}
		expr, err := js.ParseExpression(a.Val)
		var v js.Value
		constant := false
		if err == nil {
			var cerr error
			v, constant, cerr = toConstantNoGlobals(expr)
			if cerr != nil {
				return nil, cerr
			}
		}
		if !constant {
			return nil, pugerror.New("FILTER_OPTION_NOT_CONSTANT", js.QuoteJSON(a.Val)+" is not constant. All filters "+
				"are rendered compile-time so filter options must be constants.",
				pugerror.Options{Line: n.Line, Column: n.Column, Filename: n.Filename})
		}
		attrs.Set(a.Name, v)
	}
	return attrs, nil
}

func toConstantNoGlobals(expr js.Node) (js.Value, bool, error) {
	v, ok, err := js.ToConstant(expr)
	var je *js.Error
	if err != nil && !errors.As(err, &je) {
		return nil, false, nil
	}
	return v, ok, err
}
