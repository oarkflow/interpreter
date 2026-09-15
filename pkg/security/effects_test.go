package security

import "testing"

// TestAllCapabilitiesAreRegistered ensures every Capability* constant
// appears as a value for at least one BuiltinCapabilities entry, so that a
// newly introduced capability doesn't silently go unmapped in the static
// effects analysis forever.
func TestAllCapabilitiesAreRegistered(t *testing.T) {
	seen := map[string]bool{}
	for _, caps := range BuiltinCapabilities {
		for _, c := range caps {
			seen[c] = true
		}
	}
	for _, cap := range AllCapabilities() {
		if !seen[cap] {
			t.Errorf("capability %q has no entry in BuiltinCapabilities; add at least one builtin that exercises it", cap)
		}
	}
}

func TestCapabilitiesForBuiltinReturnsCopy(t *testing.T) {
	caps := CapabilitiesForBuiltin("read_file")
	if len(caps) == 0 {
		t.Fatalf("expected capabilities for read_file")
	}
	caps[0] = "mutated"
	again := CapabilitiesForBuiltin("read_file")
	if again[0] == "mutated" {
		t.Fatalf("CapabilitiesForBuiltin must return a defensive copy")
	}
}

func TestResolveModuleBuiltinName(t *testing.T) {
	cases := []struct {
		module, member, want string
	}{
		{"pdf", "to_docx", "pdf_to_docx"},
		{"database", "query", "db_query"},
		{"fs", "read_file", "read_file"},
		{"server", "listen", "listen"},
	}
	for _, tc := range cases {
		if got := ResolveModuleBuiltinName(tc.module, tc.member); got != tc.want {
			t.Errorf("ResolveModuleBuiltinName(%q, %q) = %q, want %q", tc.module, tc.member, got, tc.want)
		}
	}
}
