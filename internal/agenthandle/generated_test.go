package agenthandle

import "testing"

func TestGeneratedNames(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		name, err := Generate()
		if err != nil || !Valid(name) || seen[name] {
			t.Fatalf("invalid or duplicate generated name: %q, %v", name, err)
		}
		seen[name] = true
	}
}
