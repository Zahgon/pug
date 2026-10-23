// Copyright (c) 2009-2014 TJ Holowaychuk (pugjs/pug); MIT licence, see LICENSE.

// Package pug compiles pug templates to HTML renderers. It runs the stages of the original pug in the same order
// (lexing, comment stripping, parsing and loading of included and extended files, filters, linking, code
// generation), so that every error is the one the original reports first.
package pug

import (
	"errors"
	"io"

	codegen "github.com/pugjs/pug/packages/pug-code-gen"
	filters "github.com/pugjs/pug/packages/pug-filters"
	lexer "github.com/pugjs/pug/packages/pug-lexer"
	linker "github.com/pugjs/pug/packages/pug-linker"
	load "github.com/pugjs/pug/packages/pug-load"
	parser "github.com/pugjs/pug/packages/pug-parser"
	stripcomments "github.com/pugjs/pug/packages/pug-strip-comments"

	"github.com/pugjs/pug/internal/js"
	"github.com/pugjs/pug/internal/nodepath"
)

// Options are the compile options.
type Options struct {
	// Filename names the template; includes and extends resolve relative to it and errors name it.
	Filename string
	// Basedir is the root of absolute include and extends paths.
	Basedir string
	// Pretty indents the output with this unit; PrettySet reports that the option was given.
	Pretty    string
	PrettySet bool
	// Doctype is the default doctype; HasDoctype reports that the option was given.
	Doctype    string
	HasDoctype bool
	// NoDebug turns compileDebug off: runtime errors then carry no template context.
	NoDebug bool
	// Self reads the locals through the variable self.
	Self bool
	// Filters are the filters templates can use.
	Filters map[string]filters.Filter
	// Warnings receives the warnings the original prints on standard error.
	Warnings io.Writer
}

// Template renders a compiled template.
type Template struct {
	t *codegen.Template
}

// Render renders the template with the locals.
func (t *Template) Render(locals *js.Object) (string, error) {
	return t.t.Render(locals)
}

// RenderJSON renders the template with locals given as the text of a JSON object.
func (t *Template) RenderJSON(locals string) (string, error) {
	v, err := js.ParseJSON(locals)
	if err != nil {
		return "", err
	}
	obj, ok := v.(*js.Object)
	if !ok {
		return "", errors.New("pug: the locals must be a JSON object")
	}
	html, err := t.t.Render(obj)
	return js.Output(html), err
}

// CompileFile reads and compiles the template file name.
func CompileFile(name string, o Options) (*Template, error) {
	data, err := nodepath.ReadFile(name)
	if err != nil {
		return nil, err
	}
	o.Filename = name
	return Compile(js.DecodeUTF8(data), o)
}

// Compile compiles template source.
func Compile(src string, o Options) (*Template, error) {
	parse := func(src, filename string) (*parser.Node, error) {
		tokens, err := lexer.Lex(src, lexer.Options{Filename: filename})
		if err != nil {
			return nil, err
		}
		for i, t := range tokens {
			if t.Type == "path" && nodepath.Extname(t.Val) == "" {
				tokens[i].Val += ".pug"
			}
		}
		if tokens, err = stripcomments.Strip(tokens, filename); err != nil {
			return nil, err
		}
		return parser.Parse(tokens, parser.Options{Filename: filename, Src: src, Warnings: o.Warnings})
	}
	ast, err := load.String(src, load.Options{Filename: o.Filename, Basedir: o.Basedir, Parse: parse, Read: load.Read})
	if err != nil {
		return nil, err
	}
	if ast, err = filters.Handle(ast, o.Filters); err != nil {
		return nil, err
	}
	if ast, err = linker.Link(ast); err != nil {
		return nil, err
	}
	t, err := codegen.Compile(ast, codegen.Options{Pretty: o.Pretty, PrettySet: o.PrettySet, CompileDebug: !o.NoDebug,
		Doctype: o.Doctype, HasDoctype: o.HasDoctype, Self: o.Self})
	if err != nil {
		return nil, err
	}
	return &Template{t: t}, nil
}
