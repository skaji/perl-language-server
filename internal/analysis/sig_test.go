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
