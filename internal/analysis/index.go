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
	Name    string
	Kind    SymbolKind
	Storage string
	Start   int
	End     int
}

type Scope struct {
	Kind     string
	Start    int
	End      int
	Symbols  []Symbol
	Children []*Scope
	Parent   *Scope
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

	scopes := buildScopes(doc.Root, root)
	root.Children = scopes

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
	return collectVisibleSymbols(scope, offset)
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
	declKind := ""
	for _, tok := range tokens {
		if tok.Type == ppi.TokenWord {
			switch strings.ToLower(tok.Value) {
			case "my", "our", "state":
				activeDecl = true
				declKind = strings.ToLower(tok.Value)
			}
		}
		if tok.Type == ppi.TokenOperator && tok.Value == ";" {
			activeDecl = false
			declKind = ""
			continue
		}
		if !activeDecl {
			continue
		}
		if tok.Type == ppi.TokenSymbol {
			sym := Symbol{
				Name:    tok.Value,
				Kind:    SymbolVar,
				Storage: declKind,
				Start:   tok.Start,
				End:     tok.End,
			}
			scope := scopeForOffset(root, tok.Start)
			if scope == nil {
				scope = root
			}
			if declKind == "our" {
				scope = root
			}
			scope.Symbols = append(scope.Symbols, sym)
		}
	}
}

func buildScopes(root *ppi.Node, parent *Scope) []*Scope {
	var scopes []*Scope
	walkNodes(root, func(n *ppi.Node) {
		if n == nil {
			return
		}
		switch {
		case n.Type == ppi.NodeStatement && n.Kind == "statement::sub":
			start, end, ok := nodeTokenRange(n)
			if !ok {
				return
			}
			scopes = append(scopes, &Scope{
				Kind:  "sub",
				Start: start,
				End:   end,
			})
		case n.Type == ppi.NodeBlock:
			start, end, ok := nodeTokenRange(n)
			if !ok {
				return
			}
			scopes = append(scopes, &Scope{
				Kind:  "block",
				Start: start,
				End:   end,
			})
		}
	})

	nestScopes(parent, scopes)
	return parent.Children
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

func nestScopes(root *Scope, scopes []*Scope) {
	// Assign scopes to the smallest parent that contains them.
	for _, scope := range scopes {
		scope.Parent = root
	}
	for _, scope := range scopes {
		parent := root
		for _, candidate := range scopes {
			if candidate == scope {
				continue
			}
			if candidate.Start <= scope.Start && candidate.End >= scope.End {
				if parent == root || (candidate.End-candidate.Start) < (parent.End-parent.Start) {
					parent = candidate
				}
			}
		}
		scope.Parent = parent
	}
	for _, scope := range scopes {
		scope.Parent.Children = append(scope.Parent.Children, scope)
	}
}

func collectVisibleSymbols(scope *Scope, offset int) []Symbol {
	seen := make(map[string]struct{})
	var out []Symbol
	for cur := scope; cur != nil; cur = cur.Parent {
		for _, sym := range cur.Symbols {
			if sym.Start > offset {
				continue
			}
			if _, ok := seen[sym.Name]; ok {
				continue
			}
			seen[sym.Name] = struct{}{}
			out = append(out, sym)
		}
	}
	return out
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
