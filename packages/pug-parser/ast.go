// Copyright (c) 2014 Forbes Lindesay (pugjs/pug, pug-parser); MIT licence, see LICENSE.

package parser

// Node is a node of the pug syntax tree. As in the original, one record type serves every kind of node and later
// stages (loading, filters, linking) rewrite nodes in place, changing their Type.
type Node struct {
	Type     string
	Line     int
	Column   int
	Filename string

	// Nodes are the children of Block and NamedBlock nodes.
	Nodes []*Node
	// Name names Tag, Mixin, NamedBlock, Filter and IncludeFilter nodes.
	Name string
	// Mode is replace, append or prepend for a NamedBlock.
	Mode string

	// Val is the text of Text, Comment, BlockComment, Doctype and Code nodes and the expression of
	// AttributeBlock nodes; HasVal is false where the original's value is undefined.
	Val    string
	HasVal bool
	// IsHTML marks text written as literal HTML.
	IsHTML bool

	Block    *Node
	Attrs    []Attr
	AttrBlks []*Node

	// Tag and InterpolatedTag fields.
	SelfClosing bool
	IsInline    bool
	TextOnly    bool
	// Expr is the expression of an InterpolatedTag, Case or When node.
	Expr string

	// Code fields.
	Buffer     bool
	MustEscape bool
	// NoDebug is the original's debug: false (When nodes and else code).
	NoDebug bool

	// Conditional fields.
	Test       string
	Consequent *Node
	Alternate  *Node

	// Each and EachOf fields: the loop variable, the optional key variable and the iterated expression.
	EachVal string
	Key     string
	HasKey  bool
	Obj     string

	// Mixin fields.
	Args    string
	HasArgs bool
	Call    bool

	// Include, RawInclude and Extends nodes reference a file; RawInclude nodes carry their filters.
	File    *FileReference
	Filters []*Node

	// Linker state.
	DeclaredBlocks map[string][]*Node
	HasExtends     bool
	Parents        []*Node
	Ignore         bool
}

// Attr is an attribute of a tag or a mixin call: a JavaScript expression, or True for a bare attribute name.
type Attr struct {
	Name       string
	Val        string
	True       bool
	MustEscape bool
	Line       int
	Column     int
	Filename   string
}

// FileReference is the file an include or extends names; loading fills in the resolved path and contents.
type FileReference struct {
	Path     string
	Line     int
	Column   int
	Filename string
	FullPath string
	Str      string
	Raw      []byte
	AST      *Node
}
