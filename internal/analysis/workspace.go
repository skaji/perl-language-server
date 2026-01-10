package analysis

import (
	"os"
	"path/filepath"
	"strings"

	ppi "github.com/skaji/go-ppi"
)

type Definition struct {
	Name  string
	Kind  SymbolKind
	File  string
	Start int
	End   int
}

type WorkspaceIndex struct {
	Packages map[string][]Definition
	Subs     map[string][]Definition
	Files    int
}

func BuildWorkspaceIndex(roots []string) (*WorkspaceIndex, error) {
	index := &WorkspaceIndex{
		Packages: make(map[string][]Definition),
		Subs:     make(map[string][]Definition),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".pm" {
				return nil
			}
			if err := indexFile(path, index); err != nil {
				return err
			}
			index.Files++
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return index, nil
}

func (w *WorkspaceIndex) FindPackages(name string, exclude string) []Definition {
	return filterDefinitions(w.Packages[name], exclude)
}

func (w *WorkspaceIndex) FindSubs(name string, exclude string) []Definition {
	return filterDefinitions(w.Subs[name], exclude)
}

func filterDefinitions(defs []Definition, exclude string) []Definition {
	if len(defs) == 0 {
		return nil
	}
	if exclude == "" {
		return append([]Definition(nil), defs...)
	}
	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if def.File == exclude {
			continue
		}
		out = append(out, def)
	}
	return out
}

func indexFile(path string, w *WorkspaceIndex) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc := ppi.NewDocument(string(src))
	doc.ParseWithDiagnostics()
	defs := collectFileDefinitions(doc)
	for _, def := range defs {
		def.File = path
		switch def.Kind {
		case SymbolPackage:
			w.Packages[def.Name] = append(w.Packages[def.Name], def)
		case SymbolSub:
			w.Subs[def.Name] = append(w.Subs[def.Name], def)
		}
	}
	return nil
}

func collectFileDefinitions(doc *ppi.Document) []Definition {
	if doc == nil || doc.Root == nil {
		return nil
	}
	var defs []Definition
	walkNodes(doc.Root, func(n *ppi.Node) {
		if n == nil || n.Type != ppi.NodeStatement {
			return
		}
		switch n.Kind {
		case "statement::sub":
			start, end, ok := nodeNameRange(n)
			if !ok || n.Name == "" {
				return
			}
			defs = append(defs, Definition{
				Name:  n.Name,
				Kind:  SymbolSub,
				Start: start,
				End:   end,
			})
		case "statement::package":
			start, end, ok := nodeNameRange(n)
			if !ok || n.Name == "" {
				return
			}
			defs = append(defs, Definition{
				Name:  n.Name,
				Kind:  SymbolPackage,
				Start: start,
				End:   end,
			})
		}
	})
	return defs
}

func nodeNameRange(n *ppi.Node) (int, int, bool) {
	if n == nil || n.Name == "" {
		return 0, 0, false
	}
	for i := range n.Tokens {
		tok := n.Tokens[i]
		if tok.Value == n.Name {
			return tok.Start, tok.End, true
		}
	}
	return nodeTokenRange(n)
}
