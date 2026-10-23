// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-error); MIT licence, see LICENSE.

// Package pugerror builds the errors pug reports for problems in templates: a PUG: code, a message, and a location
// rendered together with the surrounding source lines.
package pugerror

import (
	"strconv"
	"strings"
)

// Error is a template error.
type Error struct {
	// Code is the error code including its PUG: prefix.
	Code string
	// Msg is the message without location.
	Msg string
	// Line and Column locate the error; a Column of 0 means unknown.
	Line   int
	Column int
	// Filename names the template; empty when unknown.
	Filename string
	// Message is the full message: the location, the source context and Msg.
	Message string
}

// Error returns String(error) of the original: "Error: " and the full message.
func (e *Error) Error() string {
	return "Error: " + e.Message
}

// Options locate an error.
type Options struct {
	Line     int
	Column   int
	Filename string
	// Src is the template source, used to show the lines around the error; HasSrc tells an empty source apart
	// from none.
	Src    string
	HasSrc bool
}

// New returns an error with the code (without its PUG: prefix), the message and the location.
func New(code, message string, o Options) *Error {
	location := strconv.Itoa(o.Line)
	if o.Column != 0 {
		location += ":" + strconv.Itoa(o.Column)
	}
	name := o.Filename
	if name == "" {
		name = "Pug"
	}
	var full string
	if o.HasSrc && o.Src != "" && o.Line >= 1 && o.Line <= strings.Count(o.Src, "\n")+1 {
		full = name + ":" + location + "\n" + Context(o.Src, o.Line, o.Column) + "\n\n" + message
	} else {
		full = name + ":" + location + "\n\n" + message
	}
	return &Error{Code: "PUG:" + code, Msg: message, Line: o.Line, Column: o.Column, Filename: o.Filename, Message: full}
}

// Context renders the lines around line of src: two before and three after, the error line marked with "> " and,
// when column is positive, a caret under the column.
func Context(src string, line, column int) string {
	lines := strings.Split(src, "\n")
	start := max(line-3, 0)
	end := min(len(lines), line+3)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		curr := i + 1
		preamble := "    " + strconv.Itoa(curr) + "| "
		if curr == line {
			preamble = "  > " + strconv.Itoa(curr) + "| "
		}
		text := preamble + lines[i]
		if curr == line && column > 0 {
			text += "\n" + strings.Repeat("-", len(preamble)+column-1) + "^"
		}
		out = append(out, text)
	}
	return strings.Join(out, "\n")
}
