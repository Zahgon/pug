// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-code-gen); MIT licence, see LICENSE.

package codegen

import "github.com/pugjs/pug/internal/js"

// The original generates JavaScript; this package generates a list of operations with the same meaning, which
// Render executes. Each operation corresponds to a statement of the generated function.
type op interface {
	isOp()
}

type (
	// opText appends constant text.
	opText struct{ s string }
	// opRuntimeIndent appends the indentation of the enclosing mixin calls (pug_indent.join("")).
	opRuntimeIndent struct{}
	// opDebug records the position runtime errors are reported at.
	opDebug struct {
		line    int
		file    string
		hasFile bool
	}
	// opValue appends the value of an expression: buffered code (null and undefined render as nothing, escaped
	// unless written with !=) or an interpolated tag name (raw).
	opValue struct {
		expr   js.Node
		code   bool
		escape bool
	}
	// opAttrs appends rendered attributes.
	opAttrs struct{ plan *attrPlan }
	// opStatements runs unbuffered code.
	opStatements struct{ stmts []js.Node }
	// opIf is a conditional; els may itself start with an opIf (else if).
	opIf struct {
		test      js.Node
		then, els []op
		hasElse   bool
	}
	// opFor is a for (init; test; update) loop written as code with a block.
	opFor struct {
		init, test, update js.Node
		body               []op
	}
	// opWhile is a while loop.
	opWhile struct {
		test js.Node
		body []op
	}
	// opSwitch is a case statement.
	opSwitch struct {
		disc    js.Node
		clauses []clause
	}
	// opEach is an each ... in loop, run in its own function scope like the original's IIFE.
	opEach struct {
		obj           js.Node
		val, key      string
		arrBody       []op
		objBody       []op
		arrAlt        []op
		objAlt        []op
		hasAlt        bool
		hoisted       []string
		indexVariable string
	}
	// opEachOf is an each ... of loop.
	opEachOf struct {
		obj      js.Node
		val      string
		pair     []string
		body     []op
		conflict bool
	}
	// opMixinDef defines a mixin.
	opMixinDef struct {
		name    string
		params  []param
		rest    string
		hasRest bool
		body    []op
		hoisted []string
	}
	// opMixinCall calls a mixin.
	opMixinCall struct {
		name     string
		dynamic  js.Node
		args     []js.Node
		viaCall  bool
		hasBlock bool
		block    []op
		blockVar []string
		attrs    *attrPlan
		blocks   []js.Node
		indent   string
		pretty   bool
	}
	// opMixinBlock renders the block passed to the current mixin.
	opMixinBlock struct {
		indent string
		pretty bool
	}
	// opPlainBlock runs code followed by its block, which the original wraps in { } without a condition.
	opPlainBlock struct{ body []op }
)

type clause struct {
	test    js.Node // nil for default
	body    []op
	hasBody bool
}

type param struct {
	name string
	def  js.Node
}

func (opText) isOp()          {}
func (opRuntimeIndent) isOp() {}
func (opDebug) isOp()         {}
func (opValue) isOp()         {}
func (opAttrs) isOp()         {}
func (opStatements) isOp()    {}
func (*opIf) isOp()           {}
func (opFor) isOp()           {}
func (opWhile) isOp()         {}
func (opSwitch) isOp()        {}
func (opEach) isOp()          {}
func (opEachOf) isOp()        {}
func (opMixinDef) isOp()      {}
func (opMixinCall) isOp()     {}
func (opMixinBlock) isOp()    {}
func (opPlainBlock) isOp()    {}

// attrPlan describes attributes compiled as the original's pug-attrs does: parts rendered at compile time and
// parts left to run time, in order; or, in object form, the entries of an attributes object, possibly merged with
// &attributes objects.
type attrPlan struct {
	terse bool
	// HTML form.
	parts []attrPart
	// Object form: entries plus the &attributes expressions merged into them.
	object  []attrEntry
	hasObj  bool
	blocks  []js.Node
	asAttrs bool // render the merged object with pug.attrs (a tag) rather than return it (a mixin call)
}

type attrPart struct {
	static     string
	isStatic   bool
	key        string
	expr       js.Node
	mustEscape bool
	style      bool
	class      []classItem
	isClass    bool
}

type classItem struct {
	value    js.Value
	expr     js.Node
	isStatic bool
	escape   bool
}

type attrEntry struct {
	key        string
	value      js.Value
	isStatic   bool
	expr       js.Node
	mustEscape bool
	style      bool
	class      []classItem
	isClass    bool
}
