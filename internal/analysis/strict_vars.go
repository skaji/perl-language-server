package analysis

import (
	"strings"

	ppi "github.com/skaji/go-ppi"
)

type VarDiagnostic struct {
	Message string
	Offset  int
}

// StrictVarDiagnostics reports undeclared variable usages under "use strict".
// This is a best-effort heuristic and does not attempt to fully emulate Perl.
func StrictVarDiagnostics(doc *ppi.Document) []VarDiagnostic {
	if doc == nil || doc.Root == nil {
		return nil
	}
	index := IndexDocument(doc)
	if index == nil {
		return nil
	}
	declared := collectDeclaredSymbols(index.Root)
	var diags []VarDiagnostic
	for i := 0; i < len(doc.Tokens); i++ {
		tok := doc.Tokens[i]
		if tok.Type != ppi.TokenSymbol {
			continue
		}
		if strings.HasPrefix(tok.Value, "@") && len(tok.Value) > 1 {
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 && doc.Tokens[next].Type == ppi.TokenOperator && doc.Tokens[next].Value == "{" {
				alt := "%" + tok.Value[1:]
				if _, ok := declared.visible(alt, tok.Start); ok {
					continue
				}
			}
		}
		if tok.Value == "@" || tok.Value == "%" {
			if isSigilDeref(doc.Tokens, i) {
				continue
			}
		}
		if tok.Value == "$" {
			if name, next := compositeSpecialVar(doc.Tokens, i); name != "" {
				i = next
				continue
			}
		}
		if !strictAt(doc.Root, tok.Start) {
			continue
		}
		if isSpecialVar(tok.Value) {
			continue
		}
		if _, ok := declared.visible(tok.Value, tok.Start); ok {
			continue
		}
		if strings.HasPrefix(tok.Value, "$") && len(tok.Value) > 1 {
			allowed := []string{"@", "%"}
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 && doc.Tokens[next].Type == ppi.TokenOperator {
				switch doc.Tokens[next].Value {
				case "{":
					allowed = []string{"%"}
				case "[":
					allowed = []string{"@"}
				}
			}
			if allowed[0] == "%" {
				alt := "%" + tok.Value[1:]
				if isSpecialVar(alt) {
					goto declaredOK
				}
			}
			for _, sigil := range allowed {
				alt := sigil + tok.Value[1:]
				if _, ok := declared.visible(alt, tok.Start); ok {
					goto declaredOK
				}
			}
		}
		// fallthrough: not declared
		goto undeclared
	declaredOK:
		continue
	undeclared:
		diags = append(diags, VarDiagnostic{
			Message: "use strict vars: variable " + tok.Value + " is not declared",
			Offset:  tok.Start,
		})
	}
	return diags
}

func isStrictVersion(version string) bool {
	major, minor, patch, ok := parsePerlVersion(version)
	if !ok {
		return false
	}
	if major > 5 {
		return true
	}
	if major < 5 {
		return false
	}
	if minor > 12 {
		return true
	}
	if minor < 12 {
		return false
	}
	return patch >= 0
}

func parsePerlVersion(version string) (int, int, int, bool) {
	v := strings.TrimSpace(version)
	if v == "" {
		return 0, 0, 0, false
	}
	v = strings.TrimPrefix(v, "v")
	if strings.Contains(v, ".") {
		parts := strings.Split(v, ".")
		if len(parts) == 0 {
			return 0, 0, 0, false
		}
		major, ok := parseInt(parts[0])
		if !ok {
			return 0, 0, 0, false
		}
		minor := 0
		patch := 0
		if len(parts) > 1 {
			if val, ok := parseInt(parts[1]); ok {
				minor = val
			}
		}
		if len(parts) > 2 {
			if val, ok := parseInt(parts[2]); ok {
				patch = val
			}
		}
		return major, minor, patch, true
	}
	if val, ok := parseInt(v); ok {
		if val < 10 {
			return val, 0, 0, true
		}
		if val >= 1000000 {
			major := val / 1000000
			minor := (val / 1000) % 1000
			patch := val % 1000
			return major, minor, patch, true
		}
		if val >= 1000 {
			major := val / 1000
			minor := val % 1000
			return major, minor, 0, true
		}
	}
	return 0, 0, 0, false
}

func parseInt(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int(ch-'0')
	}
	return n, true
}

func strictAt(root *ppi.Node, offset int) bool {
	if root == nil {
		return false
	}
	return strictAtNodes(root.Children, offset, false)
}

func strictAtNodes(nodes []*ppi.Node, offset int, strict bool) bool {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		start, end, ok := nodeTokenRange(n)
		if ok && offset < start {
			return strict
		}
		if blk := nodeBlockChild(n); blk != nil {
			bs, be, ok := nodeTokenRange(blk)
			if ok && offset >= bs && offset < be {
				return strictAtNodes(blk.Children, offset, strict)
			}
		}
		if isStrictToggle(n) {
			if ok && offset < end {
				return strict
			}
			strict = strictValue(n)
		}
	}
	return strict
}

func isStrictToggle(n *ppi.Node) bool {
	if n == nil || n.Kind != "statement::include" {
		return false
	}
	if strings.ToLower(n.Keyword) == "use" && n.Version != "" && isStrictVersion(n.Version) {
		return true
	}
	if strings.ToLower(n.Name) != "strict" {
		return false
	}
	return strings.ToLower(n.Keyword) == "use" || strings.ToLower(n.Keyword) == "no"
}

func strictValue(n *ppi.Node) bool {
	if n == nil {
		return false
	}
	if strings.ToLower(n.Keyword) == "use" && n.Version != "" && isStrictVersion(n.Version) {
		return true
	}
	return strings.ToLower(n.Keyword) == "use"
}

func nodeBlockChild(n *ppi.Node) *ppi.Node {
	if n == nil {
		return nil
	}
	if n.Type == ppi.NodeBlock {
		return n
	}
	for _, child := range n.Children {
		if child != nil && child.Type == ppi.NodeBlock {
			return child
		}
	}
	return nil
}

type declaredSymbols struct {
	scope *Scope
}

func collectDeclaredSymbols(root *Scope) *declaredSymbols {
	return &declaredSymbols{scope: root}
}

func (d *declaredSymbols) visible(name string, offset int) (Symbol, bool) {
	if d == nil || d.scope == nil {
		return Symbol{}, false
	}
	scope := scopeForOffset(d.scope, offset)
	if scope == nil {
		return Symbol{}, false
	}
	for cur := scope; cur != nil; cur = cur.Parent {
		for _, sym := range cur.Symbols {
			if sym.Kind != SymbolVar {
				continue
			}
			if sym.Name != name {
				continue
			}
			if sym.Start > offset {
				continue
			}
			return sym, true
		}
	}
	return Symbol{}, false
}

func isSpecialVar(name string) bool {
	if name == "" {
		return true
	}
	switch name {
	case "$_", "$.", "$/", "$,", "$\\", "$|", "$%", "$=", "$-", "$~",
		"$^", "$:", "$?", "$!", "$@", "$$", "$<", "$>", "$[", "$]", "$;",
		"$^L", "$^A", "$^E", "$^F", "$^H", "$^I", "$^M", "$^O", "$^P",
		"$^R", "$^S", "$^T", "$^V", "$^W", "$^X", "$^CHILD_ERROR_NATIVE",
		"$ARGV", "$ARGVOUT", "$LAST_PAREN_MATCH", "$LAST_SUBMATCH_RESULT",
		"$INPUT_LINE_NUMBER", "$NR", "$INPUT_RECORD_SEPARATOR", "$RS",
		"$OUTPUT_FIELD_SEPARATOR", "$OFS", "$OUTPUT_RECORD_SEPARATOR", "$ORS",
		"$OUTPUT_AUTOFLUSH", "$OFMT", "$FORMAT_PAGE_NUMBER", "$FORMAT_LINES_PER_PAGE",
		"$FORMAT_LINES_LEFT", "$FORMAT_NAME", "$FORMAT_TOP_NAME", "$FORMAT_LINE_BREAK_CHARACTERS",
		"$FORMAT_FORMFEED", "$ACCUMULATOR", "$CHILD_ERROR", "$CHILD_ERROR_NATIVE",
		"$ENCODING", "$OS_ERROR", "$EVAL_ERROR", "$PROCESS_ID", "$PID",
		"$REAL_USER_ID", "$UID", "$EFFECTIVE_USER_ID", "$EUID", "$REAL_GROUP_ID", "$GID",
		"$EFFECTIVE_GROUP_ID", "$EGID", "$PROGRAM_NAME", "$0", "$SUBSCRIPT_SEPARATOR",
		"$DB::single", "$DB::trace", "$DB::signal", "$DB::deep", "$^C", "$^D":
		return true
	case "@ARGV", "@INC", "@_", "@EXPORT", "@EXPORT_OK", "@ISA", "@F":
		return true
	case "%ENV", "%SIG", "%INC", "%ARGV", "%EXPORT_TAGS":
		return true
	}
	if name[0] == '$' && len(name) == 2 {
		switch name[1] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return true
		}
	}
	if strings.HasPrefix(name, "$^") {
		return true
	}
	return false
}

func compositeSpecialVar(tokens []ppi.Token, idx int) (string, int) {
	next := nextNonTrivia(tokens, idx+1)
	if next < 0 {
		return "", idx
	}
	tok := tokens[next]
	if tok.Type == ppi.TokenOperator {
		switch tok.Value {
		case "^":
			nextWord := nextNonTrivia(tokens, next+1)
			if nextWord < 0 {
				return "", idx
			}
			word := tokens[nextWord]
			if word.Type != ppi.TokenWord && word.Type != ppi.TokenOperator {
				return "", idx
			}
			name := "$^" + word.Value
			if isSpecialVar(name) {
				return name, nextWord
			}
		case "]", "[", "?", "!", "@", "$", "<", ">", "|", ",", ";", "#", ":", "-", "~", "*", "'", "\"", "/", "=", "\\":
			name := "$" + tok.Value
			if isSpecialVar(name) {
				return name, next
			}
		}
	}
	return "", idx
}

func nextNonTrivia(tokens []ppi.Token, idx int) int {
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

func isSigilDeref(tokens []ppi.Token, idx int) bool {
	next := nextNonTrivia(tokens, idx+1)
	if next < 0 {
		return false
	}
	tok := tokens[next]
	if tok.Type == ppi.TokenSymbol && strings.HasPrefix(tok.Value, "$") {
		return true
	}
	if tok.Type == ppi.TokenSymbol && strings.HasPrefix(tok.Value, "%") {
		return true
	}
	if tok.Type == ppi.TokenOperator && tok.Value == "{" {
		nextVar := nextNonTrivia(tokens, next+1)
		if nextVar < 0 {
			return false
		}
		if tokens[nextVar].Type == ppi.TokenSymbol && strings.HasPrefix(tokens[nextVar].Value, "$") {
			return true
		}
		if tokens[nextVar].Type == ppi.TokenSymbol && strings.HasPrefix(tokens[nextVar].Value, "%") {
			return true
		}
	}
	return false
}
