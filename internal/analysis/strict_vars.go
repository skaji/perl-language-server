package analysis

import (
	"strconv"
	"strings"

	ppi "github.com/skaji/go-ppi"
)

type VarDiagnostic struct {
	Message string
	Offset  int
}

type CallDiagnostic struct {
	Message string
	Offset  int
}

// StrictVarDiagnostics reports undeclared variable usages under "use strict".
// This is a best-effort heuristic and does not attempt to fully emulate Perl.
func StrictVarDiagnostics(doc *ppi.Document) []VarDiagnostic {
	return StrictVarDiagnosticsWithExtra(doc, nil)
}

// StrictVarDiagnosticsWithExtra reports undeclared variable usages under "use strict".
// extra contains additional variable names to treat as declared.
func StrictVarDiagnosticsWithExtra(doc *ppi.Document, extra map[string]struct{}) []VarDiagnostic {
	if doc == nil || doc.Root == nil {
		return nil
	}
	index := IndexDocument(doc)
	if index == nil {
		return nil
	}
	allowClass := hasUseModule(doc.Root, "Test2::Tools::Target")
	declared := collectDeclaredSymbols(index.Root)
	var diags []VarDiagnostic
	for i := 0; i < len(doc.Tokens); i++ {
		tok := doc.Tokens[i]
		if tok.Type != ppi.TokenSymbol {
			continue
		}
		if tok.Value == "$" {
			if i+1 < len(doc.Tokens) && doc.Tokens[i+1].Type == ppi.TokenComment && strings.HasPrefix(doc.Tokens[i+1].Value, "#{") {
				if name := parseHashSizeCommentVar(doc.Tokens[i+1].Value); name != "" {
					if isSpecialVar(name) {
						continue
					}
					if _, ok := declared.visible(name, tok.Start); ok {
						continue
					}
				}
			}
		}
		if strings.HasPrefix(tok.Value, "*") {
			continue
		}
		if tok.Value == "&" || strings.HasPrefix(tok.Value, "&") {
			continue
		}
		if tok.Value == "%" && isModuloOperator(doc.Tokens, i) {
			continue
		}
		if strings.HasPrefix(tok.Value, "@") && len(tok.Value) > 1 {
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 && doc.Tokens[next].Type == ppi.TokenOperator && doc.Tokens[next].Value == "{" {
				alt := "%" + tok.Value[1:]
				if extra != nil {
					if _, ok := extra[alt]; ok {
						continue
					}
				}
				if isSpecialVar(alt) {
					continue
				}
				if _, ok := declared.visible(alt, tok.Start); ok {
					continue
				}
			}
		}
		if tok.Value == "@" || tok.Value == "%" {
			if isPostDeref(doc.Tokens, i) {
				continue
			}
			if isSigilDeref(doc.Tokens, i) {
				continue
			}
		}
		if tok.Value == "$" {
			if isHashSizeDeref(doc.Tokens, i) {
				continue
			}
			if name, next := compositeSpecialVar(doc.Tokens, i); name != "" {
				i = next
				continue
			}
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 && doc.Tokens[next].Type == ppi.TokenSymbol && strings.HasPrefix(doc.Tokens[next].Value, "$") {
				if isSpecialVar(doc.Tokens[next].Value) {
					continue
				}
				if _, ok := declared.visible(doc.Tokens[next].Value, tok.Start); ok {
					continue
				}
			}
		}
		if tok.Value == "$#" {
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 {
				if doc.Tokens[next].Type == ppi.TokenOperator && doc.Tokens[next].Value == "{" {
					continue
				}
				if doc.Tokens[next].Type == ppi.TokenSymbol && strings.HasPrefix(doc.Tokens[next].Value, "$") {
					if _, ok := declared.visible(doc.Tokens[next].Value, tok.Start); ok {
						continue
					}
				}
			}
		}
		if strings.HasPrefix(tok.Value, "$#") && len(tok.Value) > 2 {
			alt := "@" + tok.Value[2:]
			if isSpecialVar(alt) {
				continue
			}
			if _, ok := declared.visible(alt, tok.Start); ok {
				continue
			}
		}
		if tok.Value == "$" {
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 && doc.Tokens[next].Type == ppi.TokenOperator && doc.Tokens[next].Value == "{" {
				continue
			}
		}
		if !strictAt(doc.Root, tok.Start) {
			continue
		}
		if extra != nil {
			if _, ok := extra[tok.Value]; ok {
				continue
			}
		}
		if isSpecialVar(tok.Value) {
			continue
		}
		if allowClass && tok.Value == "$CLASS" {
			continue
		}
		if strings.Contains(tok.Value, "::") {
			continue
		}
		if _, ok := declared.visible(tok.Value, tok.Start); ok {
			continue
		}
		if strings.HasPrefix(tok.Value, "$") && len(tok.Value) > 1 {
			next := nextNonTrivia(doc.Tokens, i+1)
			if next >= 0 && doc.Tokens[next].Type == ppi.TokenOperator {
				switch doc.Tokens[next].Value {
				case "{":
					alt := "%" + tok.Value[1:]
					if extra != nil {
						if _, ok := extra[alt]; ok {
							goto declaredOK
						}
					}
					if isSpecialVar(alt) {
						goto declaredOK
					}
					if _, ok := declared.visible(alt, tok.Start); ok {
						goto declaredOK
					}
				case "[":
					alt := "@" + tok.Value[1:]
					if extra != nil {
						if _, ok := extra[alt]; ok {
							goto declaredOK
						}
					}
					if isSpecialVar(alt) {
						goto declaredOK
					}
					if _, ok := declared.visible(alt, tok.Start); ok {
						goto declaredOK
					}
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

// SigCallDiagnostics reports argument count mismatches for calls to subroutines
// that have a :SIG(...) function signature. This only checks simple calls.
func SigCallDiagnostics(doc *ppi.Document) []CallDiagnostic {
	if doc == nil || doc.Root == nil {
		return nil
	}
	var diags []CallDiagnostic
	tokens := doc.Tokens
	for i, tok := range tokens {
		if tok.Type != ppi.TokenWord || tok.Value == "" {
			continue
		}
		name := tok.Value
		node := findSubNode(doc.Root, name)
		if node == nil {
			continue
		}
		start, ok := nodeFirstNonTriviaStart(node)
		if !ok {
			continue
		}
		sig := sigCommentBeforeOffset(doc.Source, start)
		if sig == "" || !strings.Contains(sig, "->") {
			continue
		}
		args, err := ParseSigArgs(sig)
		if err != nil {
			continue
		}
		callArgs, ok := parseSimpleCallArgs(tokens, i+1)
		if !ok {
			continue
		}
		if len(callArgs) != len(args) {
			msg := "call to " + name + ": expected " + itoa(len(args)) + " args, got " + itoa(len(callArgs))
			diags = append(diags, CallDiagnostic{Message: msg, Offset: tok.Start})
		}
	}
	return diags
}

func parseSimpleCallArgs(tokens []ppi.Token, idx int) ([]string, bool) {
	i := nextNonTrivia(tokens, idx)
	if i < 0 {
		return nil, false
	}
	if tokens[i].Type != ppi.TokenOperator || tokens[i].Value != "(" {
		return nil, false
	}
	depth := 0
	var count int
	var seen bool
	var invalid bool
	for j := i; j < len(tokens); j++ {
		tok := tokens[j]
		if tok.Type == ppi.TokenOperator {
			switch tok.Value {
			case "(":
				depth++
			case ")":
				depth--
				if depth == 0 {
					if invalid {
						return nil, false
					}
					if seen {
						count++
					}
					return make([]string, count), true
				}
			case ",":
				if depth == 1 {
					count++
				}
			case "@", "%", "*", "..", "=>":
				if depth == 1 {
					invalid = true
				}
			}
		}
		if depth == 1 && tok.Type == ppi.TokenSymbol {
			if strings.HasPrefix(tok.Value, "@") || strings.HasPrefix(tok.Value, "%") || strings.HasPrefix(tok.Value, "*") {
				invalid = true
			}
		}
		if depth == 1 && tok.Type != ppi.TokenWhitespace && tok.Type != ppi.TokenComment && tok.Type != ppi.TokenHereDocContent {
			if tok.Type == ppi.TokenOperator && (tok.Value == "(" || tok.Value == ",") {
				continue
			}
			seen = true
		}
	}
	return nil, false
}

func findSubNode(root *ppi.Node, name string) *ppi.Node {
	var out *ppi.Node
	walkNodes(root, func(n *ppi.Node) {
		if out != nil || n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::sub" {
			return
		}
		if n.Name == name {
			out = n
		}
	})
	return out
}

func nodeFirstNonTriviaStart(n *ppi.Node) (int, bool) {
	if n == nil || len(n.Tokens) == 0 {
		return 0, false
	}
	for _, tok := range n.Tokens {
		switch tok.Type {
		case ppi.TokenWhitespace, ppi.TokenComment, ppi.TokenHereDocContent:
			continue
		default:
			return tok.Start, true
		}
	}
	return 0, false
}

func sigCommentBeforeOffset(text string, offset int) string {
	lineStart, _ := lineBounds(text, offset)
	if lineStart <= 0 {
		return ""
	}
	prevEnd := lineStart - 1
	if prevEnd < 0 {
		return ""
	}
	prevStart := 0
	if idx := strings.LastIndexByte(text[:prevEnd], '\n'); idx >= 0 {
		prevStart = idx + 1
	}
	line := strings.TrimSpace(text[prevStart:prevEnd])
	if !strings.HasPrefix(line, "#") {
		return ""
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	if body, ok := strings.CutPrefix(line, ":SIG"); ok {
		line = strings.TrimSpace(body)
	} else if body, ok := strings.CutPrefix(line, "SIG"); ok {
		line = strings.TrimSpace(body)
	} else {
		return ""
	}
	open := strings.IndexByte(line, '(')
	closeIdx := strings.LastIndexByte(line, ')')
	if open < 0 || closeIdx < open+1 {
		return ""
	}
	return strings.TrimSpace(line[open+1 : closeIdx])
}

func lineBounds(text string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	start := 0
	if idx := strings.LastIndexByte(text[:offset], '\n'); idx >= 0 {
		start = idx + 1
	}
	end := len(text)
	if idx := strings.IndexByte(text[offset:], '\n'); idx >= 0 {
		end = offset + idx
	}
	return start, end
}

func itoa(n int) string {
	return strconv.Itoa(n)
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

func hasUseModule(root *ppi.Node, name string) bool {
	if root == nil || name == "" {
		return false
	}
	found := false
	walkNodes(root, func(n *ppi.Node) {
		if found || n == nil || n.Kind != "statement::include" {
			return
		}
		if strings.ToLower(n.Keyword) != "use" {
			return
		}
		if n.Name == name {
			found = true
		}
	})
	return found
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
		"$^", "$:", "$?", "$!", "$@", "$$", "$<", "$>", "$[", "$]", "$;", "$\"", "$'", "$`",
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
	case "%^H":
		return true
	}
	if name[0] == '$' && len(name) == 2 {
		switch name[1] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return true
		case 'a', 'b':
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
	if tok.Type == ppi.TokenOperator || tok.Type == ppi.TokenSymbol || tok.Type == ppi.TokenComment {
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
		case "#":
			nextVar := nextNonTrivia(tokens, next+1)
			if nextVar < 0 {
				return "", idx
			}
			if tokens[nextVar].Type == ppi.TokenOperator && tokens[nextVar].Value == "{" {
				return "$#{", nextVar
			}
		case "]", "[", "?", "!", "@", "$", "<", ">", "|", ",", ";", ":", "-", "~", "*", "'", "\"", "/", "=", "\\":
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
	if tok.Type == ppi.TokenOperator && (tok.Value == "@" || tok.Value == "$") {
		name := "$" + tok.Value
		if isSpecialVar(name) {
			return true
		}
	}
	if tok.Type == ppi.TokenSymbol && strings.HasPrefix(tok.Value, "$") {
		return true
	}
	if tok.Type == ppi.TokenSymbol && strings.HasPrefix(tok.Value, "%") {
		return true
	}
	if tok.Type == ppi.TokenOperator && tok.Value == "{" {
		prev := prevNonTrivia(tokens, idx-1)
		if prev >= 0 && tokens[prev].Type == ppi.TokenSymbol && tokens[prev].Value == "$#" {
			return true
		}
	}
	if tok.Type == ppi.TokenOperator && tok.Value == "{" {
		return true
	}
	return false
}

func prevNonTrivia(tokens []ppi.Token, idx int) int {
	for i := idx; i >= 0; i-- {
		switch tokens[i].Type {
		case ppi.TokenWhitespace, ppi.TokenComment, ppi.TokenHereDocContent:
			continue
		default:
			return i
		}
	}
	return -1
}

func isModuloOperator(tokens []ppi.Token, idx int) bool {
	prev := prevNonTrivia(tokens, idx-1)
	next := nextNonTrivia(tokens, idx+1)
	if prev < 0 || next < 0 {
		return false
	}
	return isOperandToken(tokens[prev], true) && isOperandToken(tokens[next], false)
}

func isOperandToken(tok ppi.Token, left bool) bool {
	switch tok.Type {
	case ppi.TokenSymbol:
		switch tok.Value {
		case "$", "@", "%", "&":
			return false
		default:
			return true
		}
	case ppi.TokenWord, ppi.TokenNumber, ppi.TokenQuote, ppi.TokenQuoteLike, ppi.TokenHereDocContent:
		return true
	case ppi.TokenOperator:
		if left {
			return tok.Value == ")" || tok.Value == "]" || tok.Value == "}"
		}
		return tok.Value == "(" || tok.Value == "[" || tok.Value == "{"
	}
	return false
}

func isHashSizeDeref(tokens []ppi.Token, idx int) bool {
	next := nextNonTrivia(tokens, idx+1)
	if next < 0 || tokens[next].Type != ppi.TokenOperator || tokens[next].Value != "#" {
		if next >= 0 && tokens[next].Type == ppi.TokenComment && strings.HasPrefix(tokens[next].Value, "#{") {
			return true
		}
		return false
	}
	next = nextNonTrivia(tokens, next+1)
	if next < 0 {
		return false
	}
	tok := tokens[next]
	if tok.Type == ppi.TokenOperator && tok.Value == "{" {
		return true
	}
	if tok.Type == ppi.TokenSymbol && strings.HasPrefix(tok.Value, "$") {
		return true
	}
	return false
}

func isPostDeref(tokens []ppi.Token, idx int) bool {
	prev := prevNonTrivia(tokens, idx-1)
	if prev < 0 {
		return false
	}
	return tokens[prev].Type == ppi.TokenOperator && tokens[prev].Value == "->"
}

func parseHashSizeCommentVar(value string) string {
	pos := strings.Index(value, "$")
	if pos < 0 || pos+1 >= len(value) {
		return ""
	}
	start := pos
	pos++
	for pos < len(value) {
		ch := value[pos]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == ':' {
			pos++
			continue
		}
		break
	}
	if pos == start+1 {
		return ""
	}
	return value[start:pos]
}
