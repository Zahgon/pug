// Copyright (c) 2015 Forbes Lindesay (pugjs/pug, pug-walk); MIT licence, see LICENSE.

// Package walk traverses a pug syntax tree and lets visitors replace nodes, as the original pug-walk does.
package walk

import (
	"errors"

	parser "github.com/pugjs/pug/packages/pug-parser"
)

// Replacer replaces the node being visited.
type Replacer struct {
	nodes        []*parser.Node
	many         bool
	arrayAllowed bool
}

// Replace replaces the visited node with another node.
func (r *Replacer) Replace(n *parser.Node) {
	r.nodes = []*parser.Node{n}
	r.many = false
}

// ReplaceMany replaces the visited node with the nodes of a list (the original's replace with an array).
func (r *Replacer) ReplaceMany(nodes []*parser.Node) error {
	if !r.arrayAllowed {
		return errors.New("replace() can only be called with an array if the last parent is a Block or NamedBlock")
	}
	r.nodes = append([]*parser.Node{}, nodes...)
	r.many = true
	return nil
}

// Func visits a node; a before visitor returns false to skip the node's children.
type Func func(n *parser.Node, r *Replacer) (bool, error)

// Options configure a walk.
type Options struct {
	// IncludeDependencies also walks the trees of included and extended files.
	IncludeDependencies bool
	parents             []*parser.Node
}

// Walk visits ast and its descendants and returns what ast was replaced with (ast itself when not replaced).
func Walk(ast *parser.Node, before, after Func, o *Options) ([]*parser.Node, error) {
	if o == nil {
		o = &Options{}
	}
	r := &Replacer{nodes: []*parser.Node{ast}}
	if len(o.parents) > 0 {
		p := o.parents[0]
		r.arrayAllowed = p.Type == "Block" || p.Type == "NamedBlock" || (p.Type == "RawInclude" && ast.Type == "IncludeFilter")
	}
	if before != nil {
		cont, err := before(ast, r)
		if err != nil {
			return nil, err
		}
		if !cont {
			return r.nodes, nil
		}
		if r.many {
			return walkAndMerge(r.nodes, before, after, o)
		}
		ast = r.nodes[0]
	}
	o.parents = append([]*parser.Node{ast}, o.parents...)
	var err error
	switch ast.Type {
	case "NamedBlock", "Block":
		ast.Nodes, err = walkAndMerge(ast.Nodes, before, after, o)
	case "Case", "Filter", "Mixin", "Tag", "InterpolatedTag", "When", "Code", "While":
		if ast.Block != nil {
			ast.Block, err = walkOne(ast.Block, before, after, o)
		}
	case "Each":
		if ast.Block != nil {
			ast.Block, err = walkOne(ast.Block, before, after, o)
		}
		if err == nil && ast.Alternate != nil {
			ast.Alternate, err = walkOne(ast.Alternate, before, after, o)
		}
	case "EachOf":
		if ast.Block != nil {
			ast.Block, err = walkOne(ast.Block, before, after, o)
		}
	case "Conditional":
		if ast.Consequent != nil {
			ast.Consequent, err = walkOne(ast.Consequent, before, after, o)
		}
		if err == nil && ast.Alternate != nil {
			ast.Alternate, err = walkOne(ast.Alternate, before, after, o)
		}
	case "Include":
		if ast.Block != nil {
			_, err = Walk(ast.Block, before, after, o)
		}
		if err == nil {
			err = walkFile(ast.File, before, after, o)
		}
	case "Extends":
		err = walkFile(ast.File, before, after, o)
	case "RawInclude":
		ast.Filters, err = walkAndMerge(ast.Filters, before, after, o)
		if err == nil {
			err = walkFile(ast.File, before, after, o)
		}
	case "Attrs", "BlockComment", "Comment", "Doctype", "IncludeFilter", "MixinBlock", "YieldBlock", "Text":
	default:
		err = errors.New("Unexpected node type " + ast.Type)
	}
	if err != nil {
		return nil, err
	}
	o.parents = o.parents[1:]
	r.nodes = []*parser.Node{ast}
	r.many = false
	if after != nil {
		if _, err := after(ast, r); err != nil {
			return nil, err
		}
	}
	return r.nodes, nil
}

// walkFile walks the tree of a referenced file when dependencies are included.
func walkFile(f *parser.FileReference, before, after Func, o *Options) error {
	if o.IncludeDependencies && f != nil && f.AST != nil {
		_, err := Walk(f.AST, before, after, o)
		return err
	}
	return nil
}

func walkOne(n *parser.Node, before, after Func, o *Options) (*parser.Node, error) {
	res, err := Walk(n, before, after, o)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, nil
	}
	return res[0], nil
}

func walkAndMerge(nodes []*parser.Node, before, after Func, o *Options) ([]*parser.Node, error) {
	out := []*parser.Node{}
	for _, n := range nodes {
		res, err := Walk(n, before, after, o)
		if err != nil {
			return nil, err
		}
		out = append(out, res...)
	}
	return out, nil
}
