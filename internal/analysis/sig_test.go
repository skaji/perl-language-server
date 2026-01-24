package analysis

import "testing"

func TestValidateSigType(t *testing.T) {
	ok := []string{
		"any",
		"int",
		"undef",
		"Foo",
		"Foo::Bar",
		"array[any]",
		"hash[int]",
		"array[hash[any]]",
	}
	for _, sig := range ok {
		if err := ValidateSig(sig); err != nil {
			t.Fatalf("expected valid sig %q: %v", sig, err)
		}
	}
}

func TestValidateSigFunc(t *testing.T) {
	ok := []string{
		"void -> void",
		"(void) -> (void)",
		"any -> any",
		"(any) -> (any)",
		"(any, int) -> any",
		"(any, array[int]) -> (any, any)",
	}
	for _, sig := range ok {
		if err := ValidateSig(sig); err != nil {
			t.Fatalf("expected valid sig %q: %v", sig, err)
		}
	}
}

func TestParseSigArgs(t *testing.T) {
	args, err := ParseSigArgs("(any, int) -> void")
	if err != nil {
		t.Fatalf("ParseSigArgs error: %v", err)
	}
	if len(args) != 2 || args[0] != "any" || args[1] != "int" {
		t.Fatalf("unexpected args: %#v", args)
	}

	args, err = ParseSigArgs("void -> void")
	if err != nil {
		t.Fatalf("ParseSigArgs error: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %#v", args)
	}
}

func TestParseSigReturn(t *testing.T) {
	ret, err := ParseSigReturn("(any, int) -> App::Foo")
	if err != nil {
		t.Fatalf("ParseSigReturn error: %v", err)
	}
	if len(ret) != 1 || ret[0] != "App::Foo" {
		t.Fatalf("expected App::Foo, got %#v", ret)
	}

	ret, err = ParseSigReturn("(any, int) -> void")
	if err != nil {
		t.Fatalf("ParseSigReturn error: %v", err)
	}
	if len(ret) != 0 {
		t.Fatalf("expected no return, got %#v", ret)
	}
}

func TestValidateSigInvalid(t *testing.T) {
	bad := []string{
		"",
		"(any, int) ->",
		"-> any",
		"any, int",
		"(any, )",
		"array[]",
		"hash[]",
		"(any, int -> any",
	}
	for _, sig := range bad {
		if err := ValidateSig(sig); err == nil {
			t.Fatalf("expected invalid sig %q", sig)
		}
	}
}
