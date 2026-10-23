package js

// Node is a node of the syntax tree of an expression or a statement.
type Node interface {
	node()
}

// Expression nodes.
type (
	// NumberLit is a numeric literal.
	NumberLit struct{ Value float64 }
	// StringLit is a string literal; Raw is its source text without the quotes.
	StringLit struct{ Value string }
	// TemplateLit is a template literal: Quasis has one more element than Exprs.
	TemplateLit struct {
		Quasis []string
		Exprs  []Node
	}
	// Ident is a reference to a variable.
	Ident struct{ Name string }
	// BoolLit is true or false.
	BoolLit struct{ Value bool }
	// NullLit is null.
	NullLit struct{}
	// ThisExpr is this.
	ThisExpr struct{}
	// ArrayLit is an array literal.
	ArrayLit struct{ Elems []Node }
	// ObjectLit is an object literal; a property with Computed set has its key in KeyExpr.
	ObjectLit struct{ Props []Property }
	// Member is obj.name or obj[expr] (Computed).
	Member struct {
		Object   Node
		Property Node // *StringLit for obj.name
		Computed bool
	}
	// Call is a call expression.
	Call struct {
		Callee Node
		Args   []Node
	}
	// Unary is a prefix operator: !, -, +, ~, typeof or void.
	Unary struct {
		Op string
		X  Node
	}
	// Update is ++ or -- in prefix or postfix position.
	Update struct {
		Op     string
		Prefix bool
		X      Node
	}
	// Binary is an arithmetic, relational or equality operator.
	Binary struct {
		Op   string
		L, R Node
	}
	// Logical is && or ||.
	Logical struct {
		Op   string
		L, R Node
	}
	// Conditional is test ? a : b.
	Conditional struct{ Test, Then, Else Node }
	// Assign is an assignment, plain (=) or compound (+=, -=, ...).
	Assign struct {
		Op     string
		Target Node
		Value  Node
	}
	// Sequence is a comma expression.
	Sequence struct{ Exprs []Node }
	// Paren is a parenthesized expression (kept for the printing of callee names).
	Paren struct{ X Node }
)

// Property is a property of an object literal.
type Property struct {
	Key      string
	KeyExpr  Node
	Computed bool
	Value    Node
	// shorthand marks {a}; numericKey marks a number literal key such as {1: x}.
	shorthand  bool
	numericKey bool
}

// Statement nodes.
type (
	// VarDecl is a var declaration with one or more declarators.
	VarDecl struct{ Decls []Declarator }
	// ExprStmt is an expression statement.
	ExprStmt struct{ X Node }
	// IfStmt is an if statement; Else is nil without an else branch.
	IfStmt struct {
		Test       Node
		Then, Else Node
	}
	// ForStmt is a for (init; test; update) statement.
	ForStmt struct {
		Init   Node // *VarDecl, *ExprStmt or nil
		Test   Node
		Update Node
		Body   Node
	}
	// WhileStmt is a while statement.
	WhileStmt struct {
		Test Node
		Body Node
	}
	// BlockStmt is a { } block.
	BlockStmt struct{ Body []Node }
	// EmptyStmt is a lone semicolon.
	EmptyStmt struct{}
)

// Declarator is one name of a var declaration, with an optional initializer.
type Declarator struct {
	Name string
	Init Node
}

func (*NumberLit) node()   {}
func (*StringLit) node()   {}
func (*TemplateLit) node() {}
func (*Ident) node()       {}
func (*BoolLit) node()     {}
func (*NullLit) node()     {}
func (*ThisExpr) node()    {}
func (*ArrayLit) node()    {}
func (*ObjectLit) node()   {}
func (*Member) node()      {}
func (*Call) node()        {}
func (*Unary) node()       {}
func (*Update) node()      {}
func (*Binary) node()      {}
func (*Logical) node()     {}
func (*Conditional) node() {}
func (*Assign) node()      {}
func (*Sequence) node()    {}
func (*Paren) node()       {}
func (*VarDecl) node()     {}
func (*ExprStmt) node()    {}
func (*IfStmt) node()      {}
func (*ForStmt) node()     {}
func (*WhileStmt) node()   {}
func (*BlockStmt) node()   {}
func (*EmptyStmt) node()   {}
