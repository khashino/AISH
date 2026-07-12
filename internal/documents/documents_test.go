package documents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFileAndDirectory(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	fileA := filepath.Join(root, "a.txt")
	dirB := filepath.Join(root, "nested")
	fileB := filepath.Join(dirB, "b.txt")
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := save(Store{Chunks: []Chunk{
		{Path: fileA, Text: "a1"},
		{Path: fileA, Text: "a2"},
		{Path: fileB, Text: "b1"},
	}}); err != nil {
		t.Fatal(err)
	}

	files, chunks, err := Remove(fileA)
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 || chunks != 2 {
		t.Fatalf("Remove(file): got files=%d chunks=%d", files, chunks)
	}

	files, chunks, err = Remove(dirB)
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 || chunks != 1 {
		t.Fatalf("Remove(dir): got files=%d chunks=%d", files, chunks)
	}

	listed, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected empty index, got %#v", listed)
	}
}

func TestClear(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := save(Store{Chunks: []Chunk{
		{Path: "a.txt", Text: "a"},
		{Path: "b.txt", Text: "b1"},
		{Path: "b.txt", Text: "b2"},
	}}); err != nil {
		t.Fatal(err)
	}

	files, chunks, err := Clear()
	if err != nil {
		t.Fatal(err)
	}
	if files != 2 || chunks != 3 {
		t.Fatalf("Clear: got files=%d chunks=%d", files, chunks)
	}

	listed, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected empty index, got %#v", listed)
	}
}
