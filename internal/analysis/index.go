package analysis

import (
	"strings"

	ppi "github.com/skaji/go-ppi"
)

type SymbolKind string

const (
	SymbolVar     SymbolKind = "var"
	SymbolSub     SymbolKind = "sub"
	SymbolPackage SymbolKind = "package"
)

type Symbol struct {
	Name  string
	Kind  SymbolKind
	Start int
	End   int
}

type Scope struct {
	Kind     string
	Start    int
	End      int
	Symbols  []Symbol
	Children []*Scope
}

type Index struct {
	Root     *Scope
	Packages []Symbol
	Subs     []Symbol
}

func IndexDocument(doc *ppi.Document) *Index {
	if doc == nil || doc.Root == nil {
		return nil
	}
	root := &Scope{
		Kind:  "document",
		Start: 0,
		End:   len(doc.Source),
	}
	index := &Index{Root: root}

	subScopes := buildSubScopes(doc.Root)
	root.Children = append(root.Children, subScopes...)

	collectDefinitions(doc.Root, index)
	collectVariables(doc, root)

	return index
}

func (idx *Index) VariablesAt(offset int) []Symbol {
	if idx == nil || idx.Root == nil {
		return nil
	}
	scope := scopeForOffset(idx.Root, offset)
	if scope == nil {
		return nil
	}
	return scope.Symbols
}

func collectDefinitions(node *ppi.Node, idx *Index) {
	walkNodes(node, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement {
			return
		}
		switch n.Kind {
		case "statement::sub":
			start, end, ok := nodeTokenRange(n)
			if !ok || n.Name == "" {
				return
			}
			sym := Symbol{Name: n.Name, Kind: SymbolSub, Start: start, End: end}
			idx.Subs = append(idx.Subs, sym)
		case "statement::package":
			start, end, ok := nodeTokenRange(n)
			if !ok || n.Name == "" {
				return
			}
			sym := Symbol{Name: n.Name, Kind: SymbolPackage, Start: start, End: end}
			idx.Packages = append(idx.Packages, sym)
		}
	})
}

func collectVariables(doc *ppi.Document, root *Scope) {
	tokens := doc.Tokens
	if len(tokens) == 0 {
		return
	}
	activeDecl := false
	for _, tok := range tokens {
		if tok.Type == ppi.TokenWord {
			switch strings.ToLower(tok.Value) {
			case "my", "our", "state":
				activeDecl = true
			}
		}
		if tok.Type == ppi.TokenOperator && tok.Value == ";" {
			activeDecl = false
			continue
		}
		if !activeDecl {
			continue
		}
		if tok.Type == ppi.TokenSymbol {
			sym := Symbol{
				Name:  tok.Value,
				Kind:  SymbolVar,
				Start: tok.Start,
				End:   tok.End,
			}
			scope := scopeForOffset(root, tok.Start)
			if scope == nil {
				scope = root
			}
			scope.Symbols = append(scope.Symbols, sym)
		}
	}
}

func buildSubScopes(root *ppi.Node) []*Scope {
	var scopes []*Scope
	walkNodes(root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::sub" {
			return
		}
		start, end, ok := nodeTokenRange(n)
		if !ok {
			return
		}
		scopes = append(scopes, &Scope{
			Kind:  "sub",
			Start: start,
			End:   end,
		})
	})
	return scopes
}

func scopeForOffset(scope *Scope, offset int) *Scope {
	if scope == nil {
		return nil
	}
	if offset < scope.Start || offset > scope.End {
		return nil
	}
	for _, child := range scope.Children {
		if found := scopeForOffset(child, offset); found != nil {
			return found
		}
	}
	return scope
}

func walkNodes(node *ppi.Node, fn func(*ppi.Node)) {
	if node == nil {
		return
	}
	fn(node)
	for _, child := range node.Children {
		walkNodes(child, fn)
	}
}

func nodeTokenRange(n *ppi.Node) (int, int, bool) {
	if n == nil {
		return 0, 0, false
	}
	found := false
	start := 0
	end := 0
	for i := range n.Tokens {
		tok := n.Tokens[i]
		if !found {
			start, end, found = tok.Start, tok.End, true
			continue
		}
		if tok.Start < start {
			start = tok.Start
		}
		if tok.End > end {
			end = tok.End
		}
	}
	for _, child := range n.Children {
		cs, ce, ok := nodeTokenRange(child)
		if !ok {
			continue
		}
		if !found {
			start, end, found = cs, ce, true
			continue
		}
		if cs < start {
			start = cs
		}
		if ce > end {
			end = ce
		}
	}
	return start, end, found
}
