package catalog

import "testing"

func TestLookupKnownGBAHash(t *testing.T) {
	entry, ok := Lookup("gba", "CAF2E99F", "")
	if !ok || entry.Title != "007 - Everything or Nothing (Japan)" {
		t.Fatalf("unexpected catalog entry: %#v, %v", entry, ok)
	}
}
