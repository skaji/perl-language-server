package analysis

import (
	"strings"

	ppi "github.com/skaji/go-ppi"
)

// ExportedSymbols returns variables exported via "our @EXPORT = qw(...)".
// It only extracts sigil-prefixed names ($, @, %) from qw lists.
func ExportedSymbols(doc *ppi.Document) map[string]struct{} {
	if doc == nil || doc.Root == nil {
		return nil
	}
	out := make(map[string]struct{})
	walkNodes(doc.Root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement {
			return
		}
		tokens := n.Tokens
		if len(tokens) == 0 {
			return
		}
		pos := nextNonTrivia(tokens, 0)
		if pos < 0 {
			return
		}
		if tokens[pos].Type == ppi.TokenWord && tokens[pos].Value == "our" {
			pos = nextNonTrivia(tokens, pos+1)
		}
		if pos < 0 || tokens[pos].Type != ppi.TokenSymbol || !isExportArraySymbol(tokens[pos].Value) {
			return
		}
		pos = nextNonTrivia(tokens, pos+1)
		if pos < 0 || tokens[pos].Type != ppi.TokenOperator || tokens[pos].Value != "=" {
			return
		}
		pos = nextNonTrivia(tokens, pos+1)
		if pos < 0 {
			return
		}
		if tokens[pos].Type == ppi.TokenOperator && tokens[pos].Value == "(" {
			pos = nextNonTrivia(tokens, pos+1)
		}
		if pos < 0 || tokens[pos].Type != ppi.TokenQuoteLike {
			return
		}
		items := splitQW(tokens[pos].Value)
		for _, item := range items {
			if item == "" {
				continue
			}
			if strings.HasPrefix(item, "$") || strings.HasPrefix(item, "@") || strings.HasPrefix(item, "%") {
				out[item] = struct{}{}
			}
		}
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitQW(value string) []string {
	if !strings.HasPrefix(value, "qw") || len(value) < 3 {
		return nil
	}
	body := value[2:]
	open := body[0]
	closeDelim := matchingDelimiter(open)
	content := body[1:]
	if closeDelim == 0 {
		return nil
	}
	if idx := strings.LastIndexByte(content, closeDelim); idx >= 0 {
		content = content[:idx]
	}
	if content == "" {
		return nil
	}
	return strings.Fields(content)
}

func matchingDelimiter(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	default:
		return open
	}
}

func isExportArraySymbol(name string) bool {
	if name == "@EXPORT" {
		return true
	}
	if !strings.HasPrefix(name, "@") {
		return false
	}
	return strings.HasSuffix(name, "::EXPORT")
}
