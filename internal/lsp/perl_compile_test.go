package lsp

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	ppi "github.com/skaji/go-ppi"
)

func TestPerlCompileDiagnostics(t *testing.T) {
	src := "use strict;\nuse Foo;\nmy $x = ;\n"
	out := "Can't locate Foo.pm in @INC (you may need to install the Foo module) (@INC contains: /tmp/lib) at test.pl line 2.\nsyntax error at test.pl line 3, near \";\"\ntest.pl had compilation errors.\n"

	diags := perlCompileDiagnostics(src, "/tmp/test.pl", out)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if got := diags[0].Message; got != "Can't locate Foo.pm in @INC (you may need to install the Foo module) (@INC contains: /tmp/lib)" {
		t.Fatalf("unexpected first message: %q", got)
	}
	if got := diags[1].Message; got != "syntax error" {
		t.Fatalf("unexpected second message: %q", got)
	}
	if diags[0].Range.Start.Line != 1 {
		t.Fatalf("expected first diagnostic line 2, got %d", diags[0].Range.Start.Line+1)
	}
	if diags[1].Range.Start.Line != 2 {
		t.Fatalf("expected second diagnostic line 3, got %d", diags[1].Range.Start.Line+1)
	}
}

func TestPerlCompileDiagnosticsIgnoresOtherFiles(t *testing.T) {
	src := "use strict;\n"
	out := "syntax error at other.pl line 1.\n"
	diags := perlCompileDiagnostics(src, "/tmp/test.pl", out)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCompileIncludePathsIncludesWorkspaceRoots(t *testing.T) {
	tmp := t.TempDir()
	workspaceLib := filepath.Join(tmp, "lib")
	if err := os.MkdirAll(workspaceLib, 0o755); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewServer(logger, "test")
	s.workspaceRoots = []string{workspaceLib}

	filePath := filepath.Join(tmp, "lib", "App", "cpm", "Builder", "EUMM.pm")
	paths := s.compileIncludePathsWithBase(nil, filePath, "")
	if !slices.Contains(paths, workspaceLib) {
		t.Fatalf("expected compile include paths to contain workspace lib %q, got %#v", workspaceLib, paths)
	}

	fileLocalLib := filepath.Join(filepath.Dir(filePath), "lib")
	if slices.Contains(paths, fileLocalLib) {
		t.Fatalf("expected compile include paths not to contain file-local lib %q, got %#v", fileLocalLib, paths)
	}
}

func TestUseLibPathsResolveAgainstWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	xtLib := filepath.Join(root, "xt", "lib")
	src := `use lib "xt/lib";`
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()

	srv := newTestServer()
	srv.projectRoots = []string{root}
	path := filepath.Join(root, "xt", "41_issue.t")

	got := collectUseLibPathsWithBase(doc.Root, path, srv.projectBaseForFile(path))
	if len(got) != 1 {
		t.Fatalf("expected 1 use lib path, got %d: %v", len(got), got)
	}
	if got[0] != xtLib {
		t.Fatalf("expected %q, got %q", xtLib, got[0])
	}
}

func TestCompileIncludePathsUseWorkspaceRootBase(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	xtLib := filepath.Join(root, "xt", "lib")
	for _, dir := range []string{lib, xtLib} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	src := `use lib "xt/lib";`
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()

	srv := newTestServer()
	srv.projectRoots = []string{root}
	srv.workspaceRoots = []string{lib}
	path := filepath.Join(root, "xt", "41_issue.t")

	got := srv.compileIncludePathsWithBase(doc.Root, path, "")
	want := []string{xtLib, lib}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
