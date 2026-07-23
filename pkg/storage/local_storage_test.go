package storage

import (
	"context"
	"errors"
	"testing"
)

func newTestLocalStorageNode(t *testing.T) StorageNode {
	t.Helper()

	node, err := NewLocalStorageNode(StorageConfig{
		NodeID:   "test-node",
		DataPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewLocalStorageNode() error = %v", err)
	}

	return node
}

func TestLocalStorageRejectsUnsafeChunkID(t *testing.T) {
	node := newTestLocalStorageNode(t)

	err := node.WriteChunk(context.Background(), "../outside", []byte("data"))
	if !errors.Is(err, ErrInvalidChunkID) {
		t.Fatalf("WriteChunk() error = %v, want %v", err, ErrInvalidChunkID)
	}
}

func TestLocalStorageReturnsChunkNotFound(t *testing.T) {
	node := newTestLocalStorageNode(t)

	_, err := node.ReadChunk(context.Background(), "missing")
	if !errors.Is(err, ErrChunkNotFound) {
		t.Fatalf("ReadChunk() error = %v, want %v", err, ErrChunkNotFound)
	}
}
