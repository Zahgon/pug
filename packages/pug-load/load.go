// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-load); MIT licence, see LICENSE.

// Package load reads the files that include and extends statements reference and parses them, as pug-load does.
package load

import (
	"errors"
	"strconv"

	parser "github.com/pugjs/pug/packages/pug-parser"
	walk "github.com/pugjs/pug/packages/pug-walk"

	"github.com/pugjs/pug/internal/js"
	"github.com/pugjs/pug/internal/nodepath"
)

// Options configure loading.
type Options struct {
	Filename string
	Basedir  string
	// Parse turns the source of a file into its syntax tree.
	Parse func(src, filename string) (*parser.Node, error)
	// Read returns the contents of a resolved file.
	Read func(filename string) ([]byte, error)
}

// String parses src and loads everything it references.
func String(src string, o Options) (*parser.Node, error) {
	ast, err := o.Parse(src, o.Filename)
	if err != nil {
		return nil, err
	}
	return Load(ast, o)
}

// Load reads and parses the files that the include, raw include and extends nodes of ast reference.
func Load(ast *parser.Node, o Options) (*parser.Node, error) {
	_, err := walk.Walk(ast, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type != "Include" && n.Type != "RawInclude" && n.Type != "Extends" {
			return true, nil
		}
		f := n.File
		path, err := Resolve(f.Path, f.Filename, o.Basedir)
		var raw []byte
		if err == nil {
			f.FullPath = path
			raw, err = o.Read(path)
		}
		if err != nil {
			var je *js.Error
			if !errors.As(err, &je) {
				je = &js.Error{Name: "Error", Message: err.Error()}
			}
			return false, &js.Error{Name: je.Name, Message: je.Message + "\n    at " + undefinedName(n.Filename) +
				" line " + strconv.Itoa(n.Line)}
		}
		f.Raw = raw
		f.Str = js.DecodeUTF8(raw)
		if n.Type == "Extends" || n.Type == "Include" {
			child := o
			child.Filename = path
			if f.AST, err = String(f.Str, child); err != nil {
				return false, err
			}
		}
		return true, nil
	}, nil, nil)
	if err != nil {
		return nil, err
	}
	return ast, nil
}

func undefinedName(s string) string {
	if s == "" {
		return "undefined"
	}
	return s
}

// Resolve returns the path of the file a template references: relative paths start from the directory of the
// referencing file, paths starting with a slash from basedir.
func Resolve(filename, source, basedir string) (string, error) {
	filename = js.TrimSpace(filename)
	absolute := len(filename) > 0 && filename[0] == '/'
	if !absolute && source == "" {
		return "", &js.Error{Name: "Error", Message: `the "filename" option is required to use includes and extends with "relative" paths`}
	}
	if absolute && basedir == "" {
		return "", &js.Error{Name: "Error", Message: `the "basedir" option is required to use includes and extends with "absolute" paths`}
	}
	if absolute {
		return nodepath.Join(basedir, filename), nil
	}
	return nodepath.Join(nodepath.Dirname(js.TrimSpace(source)), filename), nil
}

// Read reads a file from disk.
func Read(filename string) ([]byte, error) {
	return nodepath.ReadFile(filename)
}
