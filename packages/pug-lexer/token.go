// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-lexer); MIT licence, see LICENSE.

package lexer

// Pos is a line and column; columns count UTF-16 code units from 1.
type Pos struct {
	Line   int
	Column int
}

// Loc is the location of a token.
type Loc struct {
	Start    Pos
	End      Pos
	Filename string
}

// Token is a lexical token of a pug template.
type Token struct {
	Type string
	// Val is the token's value; HasVal is false when the token has none (the original's undefined).
	Val    string
	HasVal bool
	// Indent is the value of an indent token: the number of indentation characters.
	Indent int
	Loc    Loc

	// Buffer and MustEscape qualify comment, code and interpolated-code tokens.
	Buffer     bool
	MustEscape bool
	// Mode is replace, append or prepend for block tokens.
	Mode string
	// Key is the key variable of an each token (HasKey false without one).
	Key    string
	HasKey bool
	// Args holds the arguments of call and mixin tokens (HasArgs false without any).
	Args    string
	HasArgs bool
	// Code is the expression of each and eachOf tokens.
	Code string
	// Name, AttrVal and AttrTrue describe an attribute token: a value expression, or AttrTrue for a bare
	// attribute name.
	Name     string
	AttrVal  string
	AttrTrue bool
	// Value is the loop variable part of an eachOf token.
	Value string
}
