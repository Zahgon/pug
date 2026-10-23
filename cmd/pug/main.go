// Command pug renders a pug template file with JSON locals.
//
//	pug [flags] FILE
//	  -O, --obj FILE        read the locals from a JSON file holding an object (default: no locals)
//	  -P, --pretty          pretty output with two-space indentation
//	      --indent STRING   pretty output with STRING as the indentation unit
//	  -D, --doctype NAME    the default doctype
//	  -b, --basedir DIR     the root of absolute include and extends paths
//	      --no-debug        compile without debug information (runtime errors carry no template context)
//	      --self            read the locals through the variable self
//
// FILE is the template's filename: includes and extends resolve relative to it and error messages name it. The
// filters cdata, upper, lower, wrap and length are available to templates.
//
// On success the HTML is written to standard output and the exit status is 0. A compile or render error is
// written to standard error (its PUG: code on a line of its own first, when it has one) with exit status 1. A
// usage error (unknown flag, missing flag value, not exactly one FILE, unreadable or invalid locals) is one line
// starting with "pug: " on standard error, with exit status 2.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	pug "github.com/pugjs/pug/packages/pug"
	pugerror "github.com/pugjs/pug/packages/pug-error"

	"github.com/pugjs/pug/internal/js"
)

type usageError struct {
	msg string
}

// Error returns the usage message.
func (e *usageError) Error() string {
	return e.msg
}

type options struct {
	obj     string
	hasObj  bool
	compile pug.Options
	file    string
}

func parseArgs(argv []string) (*options, error) {
	o := &options{}
	var positional []string
	value := func(i int, flag string) (string, error) {
		if i+1 >= len(argv) {
			return "", &usageError{"flag " + flag + " needs a value"}
		}
		return argv[i+1], nil
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			positional = append(positional, argv[i+1:]...)
			break
		}
		var err error
		switch a {
		case "-O", "--obj":
			o.obj, err = value(i, a)
			o.hasObj = true
			i++
		case "-P", "--pretty":
			o.compile.Pretty, o.compile.PrettySet = "  ", true
		case "--indent":
			o.compile.Pretty, err = value(i, a)
			o.compile.PrettySet = true
			i++
		case "-D", "--doctype":
			o.compile.Doctype, err = value(i, a)
			o.compile.HasDoctype = true
			i++
		case "-b", "--basedir":
			o.compile.Basedir, err = value(i, a)
			i++
		case "--no-debug":
			o.compile.NoDebug = true
		case "--self":
			o.compile.Self = true
		default:
			if len(a) > 1 && a[0] == '-' {
				return nil, &usageError{"unknown flag " + a}
			}
			positional = append(positional, a)
		}
		if err != nil {
			return nil, err
		}
	}
	if len(positional) != 1 {
		return nil, &usageError{"expected exactly one FILE argument"}
	}
	o.file = positional[0]
	return o, nil
}

func readLocals(o *options) (*js.Object, error) {
	if !o.hasObj {
		return js.NewObject(), nil
	}
	data, err := os.ReadFile(o.obj)
	if err != nil {
		return nil, &usageError{"cannot read locals file " + o.obj}
	}
	v, err := js.ParseJSON(js.DecodeUTF8(data))
	if err != nil {
		var je *js.Error
		msg := err.Error()
		if errors.As(err, &je) {
			msg = je.Message
		}
		return nil, &usageError{"locals file " + o.obj + " is not valid JSON: " + msg}
	}
	obj, ok := v.(*js.Object)
	if !ok {
		return nil, &usageError{"locals file " + o.obj + " must hold a JSON object"}
	}
	return obj, nil
}

// describe renders an error as the reference command line does: the pug error code on its own line, then
// String(error).
func describe(err error) string {
	var pe *pugerror.Error
	if errors.As(err, &pe) {
		return pe.Code + "\n" + pe.Error()
	}
	return err.Error()
}

func run(argv []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(argv)
	var locals *js.Object
	if err == nil {
		locals, err = readLocals(o)
	}
	if err != nil {
		fmt.Fprintf(stderr, "pug: %s\n", js.Output(err.Error()))
		return 2
	}
	o.compile.Filters = filterSet
	o.compile.Warnings = stderr
	t, err := pug.CompileFile(o.file, o.compile)
	var html string
	if err == nil {
		html, err = t.Render(locals)
	}
	if err != nil {
		// The exit status reports the failure even when standard error cannot be written.
		_, _ = io.WriteString(stderr, js.Output(describe(err))+"\n")
		return 1
	}
	if _, err := io.WriteString(stdout, js.Output(html)); err != nil {
		fmt.Fprintf(stderr, "pug: cannot write the output: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
