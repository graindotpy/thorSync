package archive

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var errReaderFailed = errors.New("reader failed")

type failingReader struct {
	read bool
}

func (r *failingReader) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, errReaderFailed
	}
	r.read = true
	return copy(buffer, []byte("partial archive data")), errReaderFailed
}

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

func TestPutBytesAndReaderShareDurableStorage(t *testing.T) {
	root := t.TempDir()
	content := bytes.Repeat([]byte("streamed-content"), 4096)
	archiveRoot := filepath.Join(root, "archive")
	archive := New(archiveRoot, 5<<30, 0)

	fromBytes, err := archive.PutBytes(content)
	if err != nil {
		t.Fatal(err)
	}
	fromReader, err := archive.PutReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	wantHash := fmt.Sprintf("%x", sha256.Sum256(content))
	if fromBytes.Hash != wantHash {
		t.Fatalf("archive hash %q, want %q", fromBytes.Hash, wantHash)
	}
	if fromBytes != fromReader {
		t.Fatalf("expected byte and reader inputs to deduplicate: %#v %#v", fromBytes, fromReader)
	}
	if fromBytes.Size != int64(len(content)) {
		t.Fatalf("archive size %d, want %d", fromBytes.Size, len(content))
	}

	stored, err := os.ReadFile(fromBytes.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, content) {
		t.Fatal("stored archive content did not match input")
	}
	if err := New(archiveRoot, 5<<30, 0).Verify(fromBytes.Hash); err != nil {
		t.Fatalf("archive object was not durable across instances: %v", err)
	}

	capacity, err := archive.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if capacity.UsedBytes != int64(len(content)) {
		t.Fatalf("archive used %d, want %d", capacity.UsedBytes, len(content))
	}
}

func TestPutReaderRejectsInvalidInputAndCleansPartial(t *testing.T) {
	root := t.TempDir()
	archiveRoot := filepath.Join(root, "archive")
	archive := New(archiveRoot, 5<<30, 0)

	if _, err := archive.PutReader(nil); err == nil {
		t.Fatal("expected nil reader to be rejected")
	}
	if _, err := archive.PutBytes(nil); err == nil {
		t.Fatal("expected zero-byte input to be rejected")
	}
	if _, err := archive.PutReader(&failingReader{}); !errors.Is(err, errReaderFailed) {
		t.Fatalf("PutReader error = %v, want %v", err, errReaderFailed)
	}
	if _, err := archive.PutReader(bytes.NewReader(make([]byte, 16*1024*1024+1))); err == nil {
		t.Fatal("expected oversized reader input to be rejected")
	}

	entries, err := os.ReadDir(filepath.Join(archiveRoot, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary archive files were not cleaned up: %v", entries)
	}
	capacity, err := archive.Capacity()
	if err != nil {
		t.Fatal(err)
	}
	if capacity.UsedBytes != 0 {
		t.Fatalf("failed reader consumed %d archive bytes", capacity.UsedBytes)
	}
}

func TestReserveCheckIncludesIncomingBlobSize(t *testing.T) {
	for _, test := range []struct {
		name                 string
		free, reserve, write int64
		allowed              bool
	}{
		{"room remains", 4096, 1024, 2048, true},
		{"exactly reaches reserve", 4096, 1024, 3072, true},
		{"would cross reserve", 4096, 1024, 3073, false},
		{"reserve already reached", 1024, 1024, 1, false},
		{"negative requirement", 4096, 1024, -1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := reserveAllows(uint64(test.free), test.reserve, test.write); got != test.allowed {
				t.Fatalf("reserveAllows(%d,%d,%d)=%v, want %v", test.free, test.reserve, test.write, got, test.allowed)
			}
		})
	}
}

func TestSyncDirTreeRejectsUnrelatedStopDirectory(t *testing.T) {
	root := t.TempDir()
	start := filepath.Join(root, "archive", "blobs", "aa", "bb")
	if err := os.MkdirAll(start, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := syncDirTree(start, filepath.Join(root, "elsewhere")); err == nil {
		t.Fatal("expected an unrelated archive root to be rejected")
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
