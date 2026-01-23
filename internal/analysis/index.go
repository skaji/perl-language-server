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

	scopes := buildScopes(doc.Root, root, doc.Tokens)
	root.Children = scopes

	collectDefinitions(doc.Root, index)
	collectVariables(doc, root)
	collectSignatureVars(doc.Root, root)
	collectAnonSignatureVars(doc, root)

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
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Type == ppi.TokenWord || tok.Type == ppi.TokenAttribute {
			switch strings.ToLower(tok.Value) {
			case "my", "our", "state":
				activeDecl = true
				declKind = strings.ToLower(tok.Value)
			case "use":
				next := nextNonTriviaToken(tokens, i+1)
				if next >= 0 && tokens[next].Type == ppi.TokenWord && strings.ToLower(tokens[next].Value) == "vars" {
					activeDecl = true
					declKind = "our"
				}
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
		if declKind == "our" && tok.Type == ppi.TokenQuoteLike && strings.HasPrefix(tok.Value, "qw") {
			for _, name := range splitQWNames(tok.Value) {
				if name == "" {
					continue
				}
				if !(strings.HasPrefix(name, "$") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "%")) {
					continue
				}
				sym := Symbol{
					Name:    name,
					Kind:    SymbolVar,
					Storage: declKind,
					Start:   tok.Start,
					End:     tok.End,
				}
				scope := scopeForOffset(root, tok.Start)
				if scope == nil {
					scope = root
				}
				scope.Symbols = append(scope.Symbols, sym)
			}
		}
	}
}

func collectSignatureVars(rootNode *ppi.Node, root *Scope) {
	if rootNode == nil || root == nil {
		return
	}
	walkNodes(rootNode, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement {
			return
		}
		switch n.Kind {
		case "statement::sub":
			if len(n.SubSigVars) == 0 {
				return
			}
			start, _, ok := nodeTokenRange(n)
			if !ok {
				return
			}
			scope := findScopeByRange(root, "sub", start)
			if scope == nil {
				scope = scopeForOffset(root, start)
			}
			if scope == nil {
				scope = root
			}
			for _, name := range n.SubSigVars {
				addSigVar(scope, start, name)
			}
		}
	})
}

func addSigVar(scope *Scope, start int, name string) {
	if scope == nil {
		return
	}
	if len(name) < 2 {
		return
	}
	if !(strings.HasPrefix(name, "$") || strings.HasPrefix(name, "@") || strings.HasPrefix(name, "%")) {
		return
	}
	sym := Symbol{
		Name:    name,
		Kind:    SymbolVar,
		Storage: "my",
		Start:   start,
		End:     start,
	}
	scope.Symbols = append(scope.Symbols, sym)
}

func anonSubSignatureVars(tokens []ppi.Token) []string {
	if len(tokens) == 0 {
		return nil
	}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Type != ppi.TokenWord || tok.Value != "sub" {
			continue
		}
		next := nextNonTrivia(tokens, i+1)
		if next < 0 {
			continue
		}
		if tokens[next].Type == ppi.TokenWord {
			continue
		}
		if tokens[next].Type != ppi.TokenPrototype {
			continue
		}
		return signatureVarsFromPrototype(tokens[next].Value)
	}
	return nil
}

func signatureVarsFromPrototype(proto string) []string {
	if proto == "" {
		return nil
	}
	if proto[0] != '(' || proto[len(proto)-1] != ')' {
		return nil
	}
	body := strings.TrimSpace(proto[1 : len(proto)-1])
	if body == "" {
		return nil
	}
	parts := strings.Split(body, ",")
	var vars []string
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		for i := 0; i < len(p); i++ {
			if p[i] != '$' && p[i] != '@' && p[i] != '%' {
				continue
			}
			j := i + 1
			for j < len(p) && (isWordStart(p[j]) || isDigit(p[j]) || p[j] == '_') {
				j++
			}
			if j == i+1 {
				continue
			}
			vars = append(vars, p[i:j])
			break
		}
	}
	return vars
}

func isWordStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func collectAnonSignatureVars(doc *ppi.Document, root *Scope) {
	if doc == nil || root == nil {
		return
	}
	tokens := doc.Tokens
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Type != ppi.TokenWord || tok.Value != "sub" {
			continue
		}
		next := nextNonTrivia(tokens, i+1)
		if next < 0 {
			continue
		}
		if tokens[next].Type == ppi.TokenWord {
			continue
		}
		if tokens[next].Type != ppi.TokenPrototype {
			continue
		}
		sigVars := signatureVarsFromPrototype(tokens[next].Value)
		if len(sigVars) == 0 {
			continue
		}
		open := nextOperator(tokens, next+1, "{")
		if open < 0 {
			continue
		}
		close := matchBrace(tokens, open)
		if close < 0 {
			continue
		}
		start := tokens[open].Start
		scope := findScopeByRange(root, "block", start)
		if scope == nil {
			scope = scopeForOffset(root, start)
		}
		if scope == nil {
			scope = root
		}
		for _, name := range sigVars {
			addSigVar(scope, start, name)
		}
	}
}

func buildScopes(root *ppi.Node, parent *Scope, tokens []ppi.Token) []*Scope {
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

	if len(tokens) > 0 {
		scopes = append(scopes, anonSubScopesFromTokens(tokens)...)
	}

	nestScopes(parent, scopes)
	return parent.Children
}

func anonSubScopesFromTokens(tokens []ppi.Token) []*Scope {
	var scopes []*Scope
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Type != ppi.TokenWord || tok.Value != "sub" {
			continue
		}
		next := nextNonTrivia(tokens, i+1)
		if next < 0 {
			continue
		}
		if tokens[next].Type == ppi.TokenWord {
			continue
		}
		open := nextOperator(tokens, next+1, "{")
		if open < 0 {
			continue
		}
		close := matchBrace(tokens, open)
		if close < 0 {
			continue
		}
		scopes = append(scopes, &Scope{
			Kind:  "block",
			Start: tokens[open].Start,
			End:   tokens[close].End,
		})
		i = close
	}
	return scopes
}

func nextOperator(tokens []ppi.Token, start int, op string) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Type == ppi.TokenOperator && tokens[i].Value == op {
			return i
		}
	}
	return -1
}

func matchBrace(tokens []ppi.Token, open int) int {
	if open < 0 || open >= len(tokens) {
		return -1
	}
	if tokens[open].Type != ppi.TokenOperator || tokens[open].Value != "{" {
		return -1
	}
	depth := 0
	for i := open; i < len(tokens); i++ {
		if tokens[i].Type != ppi.TokenOperator {
			continue
		}
		switch tokens[i].Value {
		case "{":
			depth++
		case "}":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func findScopeByRange(scope *Scope, kind string, start int) *Scope {
	if scope == nil {
		return nil
	}
	if scope.Kind == kind && scope.Start == start {
		return scope
	}
	for _, child := range scope.Children {
		if found := findScopeByRange(child, kind, start); found != nil {
			return found
		}
	}
	return nil
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

func nextNonTriviaToken(tokens []ppi.Token, idx int) int {
	for i := idx; i < len(tokens); i++ {
		switch tokens[i].Type {
		case ppi.TokenWhitespace, ppi.TokenComment, ppi.TokenHereDocContent:
			continue
		default:
			return i
		}
	}
	return -1
}

func splitQWNames(value string) []string {
	if !strings.HasPrefix(value, "qw") || len(value) < 3 {
		return nil
	}
	body := value[2:]
	open := body[0]
	close := matchingDelimiter(open)
	if close == 0 {
		return nil
	}
	content := body[1:]
	if idx := strings.LastIndexByte(content, close); idx >= 0 {
		content = content[:idx]
	}
	if content == "" {
		return nil
	}
	return strings.Fields(content)
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
