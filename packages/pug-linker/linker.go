// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-linker); MIT licence, see LICENSE.

// Package linker resolves template inheritance and includes into one syntax tree, as pug-linker does: it applies
// extends with named blocks (replace, append, prepend), inlines included files and yields, and hoists the mixins
// of an extending template.
package linker

import (
	pugerror "github.com/pugjs/pug/packages/pug-error"
	parser "github.com/pugjs/pug/packages/pug-parser"
	walk "github.com/pugjs/pug/packages/pug-walk"

	"github.com/pugjs/pug/internal/js"
)

func nodeError(code, message string, n *parser.Node) error {
	return pugerror.New(code, message, pugerror.Options{Line: n.Line, Column: n.Column, Filename: n.Filename})
}

// Link returns the linked tree of ast.
func Link(ast *parser.Node) (*parser.Node, error) {
	if ast.Type != "Block" {
		return nil, &js.Error{Name: "AssertionError [ERR_ASSERTION]", Message: "The top level element should always be a block"}
	}
	var extendsNode *parser.Node
	if len(ast.Nodes) > 0 {
		hasExtends := ast.Nodes[0].Type == "Extends"
		if err := checkExtendPosition(ast, hasExtends); err != nil {
			return nil, err
		}
		if hasExtends {
			extendsNode = ast.Nodes[0]
			ast.Nodes = ast.Nodes[1:]
		}
	}
	ast, err := applyIncludes(ast)
	if err != nil {
		return nil, err
	}
	if ast.DeclaredBlocks, err = findDeclaredBlocks(ast); err != nil {
		return nil, err
	}
	if extendsNode == nil {
		return ast, nil
	}
	var mixins, expectedBlocks []*parser.Node
	var addNode func(n *parser.Node) error
	addNode = func(n *parser.Node) error {
		switch {
		case n.Type == "NamedBlock":
			expectedBlocks = append(expectedBlocks, n)
		case n.Type == "Block":
			for _, c := range n.Nodes {
				if err := addNode(c); err != nil {
					return err
				}
			}
		case n.Type == "Mixin" && !n.Call:
			mixins = append(mixins, n)
		default:
			return nodeError("UNEXPECTED_NODES_IN_EXTENDING_ROOT",
				"Only named blocks and mixins can appear at the top level of an extending template", n)
		}
		return nil
	}
	for _, n := range ast.Nodes {
		if err := addNode(n); err != nil {
			return nil, err
		}
	}
	parent, err := Link(extendsNode.File.AST)
	if err != nil {
		return nil, err
	}
	if err := extend(parent.DeclaredBlocks, ast); err != nil {
		return nil, err
	}
	var found []string
	if _, err := walk.Walk(parent, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type == "NamedBlock" {
			found = append(found, n.Name)
		}
		return true, nil
	}, nil, nil); err != nil {
		return nil, err
	}
	for _, b := range expectedBlocks {
		if !contains(found, b.Name) {
			return nil, nodeError("UNEXPECTED_BLOCK", "Unexpected block "+b.Name, b)
		}
	}
	for _, name := range declaredOrder(ast) {
		parent.DeclaredBlocks[name] = ast.DeclaredBlocks[name]
	}
	parent.Nodes = append(append([]*parser.Node{}, mixins...), parent.Nodes...)
	parent.HasExtends = true
	return parent, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// declaredOrder returns the names of a tree's declared blocks in declaration order.
func declaredOrder(ast *parser.Node) []string {
	var names []string
	walk.Walk(ast, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type == "NamedBlock" && n.Mode == "replace" && !contains(names, n.Name) {
			names = append(names, n.Name)
		}
		return true, nil
	}, nil, nil)
	var out []string
	for _, name := range names {
		if _, ok := ast.DeclaredBlocks[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func findDeclaredBlocks(ast *parser.Node) (map[string][]*parser.Node, error) {
	definitions := map[string][]*parser.Node{}
	_, err := walk.Walk(ast, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type == "NamedBlock" && n.Mode == "replace" {
			definitions[n.Name] = append(definitions[n.Name], n)
		}
		return true, nil
	}, nil, nil)
	return definitions, err
}

func flattenParentBlocks(blocks []*parser.Node, acc []*parser.Node) []*parser.Node {
	for _, b := range blocks {
		if b.Parents != nil {
			acc = flattenParentBlocks(b.Parents, acc)
		}
		acc = append(acc, b)
	}
	return acc
}

func extend(parentBlocks map[string][]*parser.Node, ast *parser.Node) error {
	stack := map[string]bool{}
	_, err := walk.Walk(ast, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type != "NamedBlock" {
			return true, nil
		}
		if stack[n.Name] {
			n.Ignore = true
			return true, nil
		}
		stack[n.Name] = true
		var list []*parser.Node
		if parentBlocks[n.Name] != nil {
			list = flattenParentBlocks(parentBlocks[n.Name], nil)
		}
		if len(list) > 0 {
			n.Parents = list
			for _, pb := range list {
				switch n.Mode {
				case "append":
					pb.Nodes = append(append([]*parser.Node{}, pb.Nodes...), n.Nodes...)
				case "prepend":
					pb.Nodes = append(append([]*parser.Node{}, n.Nodes...), pb.Nodes...)
				case "replace":
					pb.Nodes = n.Nodes
				}
			}
		}
		return true, nil
	}, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type == "NamedBlock" && !n.Ignore {
			delete(stack, n.Name)
		}
		return true, nil
	}, nil)
	return err
}

func applyIncludes(ast *parser.Node) (*parser.Node, error) {
	res, err := walk.Walk(ast, func(n *parser.Node, r *walk.Replacer) (bool, error) {
		if n.Type == "RawInclude" {
			r.Replace(&parser.Node{Type: "Text", Val: crRe(n.File.Str), HasVal: true})
		}
		return true, nil
	}, func(n *parser.Node, r *walk.Replacer) (bool, error) {
		if n.Type != "Include" {
			return true, nil
		}
		child, err := Link(n.File.AST)
		if err != nil {
			return false, err
		}
		if child.HasExtends {
			if child, err = removeBlocks(child); err != nil {
				return false, err
			}
		}
		y, err := applyYield(child, n.Block)
		if err != nil {
			return false, err
		}
		r.Replace(y)
		return true, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return res[0], nil
}

func crRe(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\r' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func removeBlocks(ast *parser.Node) (*parser.Node, error) {
	res, err := walk.Walk(ast, func(n *parser.Node, r *walk.Replacer) (bool, error) {
		if n.Type == "NamedBlock" {
			r.Replace(&parser.Node{Type: "Block", Nodes: n.Nodes})
		}
		return true, nil
	}, nil, nil)
	if err != nil {
		return nil, err
	}
	return res[0], nil
}

func applyYield(ast, block *parser.Node) (*parser.Node, error) {
	if block == nil || len(block.Nodes) == 0 {
		return ast, nil
	}
	replaced := false
	res, err := walk.Walk(ast, nil, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type == "YieldBlock" {
			replaced = true
			n.Type = "Block"
			n.Nodes = []*parser.Node{block}
		}
		return true, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	ast = res[0]
	if !replaced {
		loc := defaultYieldLocation(ast)
		loc.Nodes = append(loc.Nodes, block)
	}
	return ast, nil
}

func defaultYieldLocation(n *parser.Node) *parser.Node {
	res := n
	for _, c := range n.Nodes {
		if c.TextOnly {
			continue
		}
		if c.Type == "Block" {
			res = defaultYieldLocation(c)
		} else if c.Block != nil && len(c.Block.Nodes) > 0 {
			res = defaultYieldLocation(c.Block)
		}
	}
	return res
}

func checkExtendPosition(ast *parser.Node, hasExtends bool) error {
	legit := false
	_, err := walk.Walk(ast, func(n *parser.Node, _ *walk.Replacer) (bool, error) {
		if n.Type == "Extends" {
			if hasExtends && !legit {
				legit = true
			} else {
				return false, nodeError("EXTENDS_NOT_FIRST", "Declaration of template inheritance (\"extends\") should be "+
					"the first thing in the file. There can only be one extends statement per file.", n)
			}
		}
		return true, nil
	}, nil, nil)
	return err
}
