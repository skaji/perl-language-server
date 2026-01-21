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
	if !hasUseStrict(doc.Root) {
		return nil
	}
	index := IndexDocument(doc)
	if index == nil {
		return nil
	}
	declared := collectDeclaredSymbols(index.Root)
	var diags []VarDiagnostic
	for _, tok := range doc.Tokens {
		if tok.Type != ppi.TokenSymbol {
			continue
		}
		if isSpecialVar(tok.Value) {
			continue
		}
		if _, ok := declared.visible(tok.Value, tok.Start); ok {
			continue
		}
		diags = append(diags, VarDiagnostic{
			Message: "use strict vars: variable " + tok.Value + " is not declared",
			Offset:  tok.Start,
		})
	}
	return diags
}

func hasUseStrict(root *ppi.Node) bool {
	found := false
	walkNodes(root, func(n *ppi.Node) {
		if found || n == nil || n.Type != ppi.NodeStatement || n.Kind != "statement::include" {
			return
		}
		if strings.ToLower(n.Keyword) != "use" {
			return
		}
		if strings.ToLower(n.Name) == "strict" {
			found = true
		}
	})
	return found
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
