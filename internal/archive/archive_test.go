package archive

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPutFileDeduplicatesByHash(t *testing.T) {
	root := t.TempDir()
	sourceA := filepath.Join(root, "a.sav")
	sourceB := filepath.Join(root, "b.sav")
	content := bytes.Repeat([]byte{0x5a}, 64*1024)
	if err := os.WriteFile(sourceA, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceB, content, 0o600); err != nil {
		t.Fatal(err)
	}
	archive := New(filepath.Join(root, "archive"), 5<<30, 0)
	first, err := archive.PutFile(sourceA)
	if err != nil {
		t.Fatal(err)
	}
	second, err := archive.PutFile(sourceB)
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash != second.Hash || first.Path != second.Path {
		t.Fatalf("expected one deduplicated blob: %#v %#v", first, second)
	}
	capacity, err := archive.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if capacity.UsedBytes != int64(len(content)) {
		t.Fatalf("archive used %d, want %d", capacity.UsedBytes, len(content))
	}
}

func TestPutFileRejectsZeroBytes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "empty.sav")
	if err := os.WriteFile(source, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(root, "archive"), 5<<30, 0).PutFile(source); err == nil {
		t.Fatal("expected zero-byte save to be rejected")
	}
}

func TestVerifyDetectsCorruptionAndRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "save.sav")
	content := bytes.Repeat([]byte{0x7c}, 64*1024)
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	archive := New(filepath.Join(root, "archive"), 5<<30, 0)
	blob, err := archive.PutFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = archive.Verify(blob.Hash); err != nil {
		t.Fatalf("valid blob did not verify: %v", err)
	}
	if _, err = archive.Open("../../outside"); err == nil {
		t.Fatal("expected invalid archive hash to be rejected")
	}
	if err = os.WriteFile(blob.Path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = archive.Verify(blob.Hash); err == nil {
		t.Fatal("expected corrupted blob to fail verification")
	}
}
