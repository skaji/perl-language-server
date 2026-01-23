package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ppi "github.com/skaji/go-ppi"
	"github.com/skaji/perl-language-server/internal/analysis"
	"github.com/skaji/perl-language-server/internal/lsp"
)

type diag struct {
	path string
	line int
	col  int
	msg  string
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"Module-Build", "ExtUtils-MakeMaker"}
	}
	var diags []diag
	visited := map[string]struct{}{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		realRoot := root
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			realRoot = resolved
		}
		if _, ok := visited[realRoot]; ok {
			continue
		}
		visited[realRoot] = struct{}{}
		err := walkRoot(realRoot, &diags, visited)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	sort.Slice(diags, func(i, j int) bool {
		if diags[i].path != diags[j].path {
			return diags[i].path < diags[j].path
		}
		if diags[i].line != diags[j].line {
			return diags[i].line < diags[j].line
		}
		if diags[i].col != diags[j].col {
			return diags[i].col < diags[j].col
		}
		return diags[i].msg < diags[j].msg
	})

	for _, d := range diags {
		fmt.Printf("%s:%d:%d: %s\n", d.path, d.line, d.col, d.msg)
	}
}

func walkRoot(root string, diags *[]diag, visited map[string]struct{}) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(path)
			if err == nil && info.IsDir() {
				real := path
				if resolved, err := filepath.EvalSymlinks(path); err == nil {
					real = resolved
				}
				if _, ok := visited[real]; ok {
					return nil
				}
				visited[real] = struct{}{}
				return walkRoot(real, diags, visited)
			}
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".pm" && ext != ".pl" && ext != ".t" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		doc := ppi.NewDocument(src)
		doc.ParseWithDiagnostics()
		for _, errd := range doc.Errors {
			line, col := lineCol(src, errd.Offset)
			*diags = append(*diags, diag{path: path, line: line, col: col, msg: errd.Message})
		}
		extra := lsp.ExportedStrictVars(doc, path)
		for _, errd := range analysis.StrictVarDiagnosticsWithExtra(doc, extra) {
			line, col := lineCol(src, errd.Offset)
			*diags = append(*diags, diag{path: path, line: line, col: col, msg: errd.Message})
		}
		return nil
	})
}

func lineCol(text string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	line := 1
	lastNL := -1
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
			lastNL = i
		}
	}
	col := offset - lastNL
	if col < 1 {
		col = 1
	}
	return line, col
}
