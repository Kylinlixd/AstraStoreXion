package metadata

import "testing"

func TestMetadataServiceReturnsStableDetachedSnapshots(t *testing.T) {
	service := NewInMemoryMetadataService()

	if err := service.CreateFileMetadata(CreateFileMetadataInput{
		FileID:      "b",
		Name:        "b.txt",
		ContentType: "text/plain",
		Size:        2,
		Checksum:    "checksum-b",
		OwnerID:     "user-1",
		Status:      FileStatusReady,
		Chunks:      []ChunkInfo{{ChunkID: "chunk-b", NodeID: "node-1", Size: 2}},
	}); err != nil {
		t.Fatalf("CreateFileMetadata(b) error = %v", err)
	}

	if err := service.CreateFileMetadata(CreateFileMetadataInput{
		FileID:      "a",
		Name:        "a.txt",
		ContentType: "text/plain",
		Size:        1,
		Checksum:    "checksum-a",
		OwnerID:     "user-1",
		Status:      FileStatusReady,
		Chunks:      []ChunkInfo{{ChunkID: "chunk-a", NodeID: "node-1", Size: 1}},
	}); err != nil {
		t.Fatalf("CreateFileMetadata(a) error = %v", err)
	}

	listed, err := service.ListMetadata(10, 0)
	if err != nil {
		t.Fatalf("ListMetadata() error = %v", err)
	}
	if got := []string{listed[0].FileID, listed[1].FileID}; got[0] != "a" || got[1] != "b" {
		t.Fatalf("ListMetadata() order = %v, want [a b]", got)
	}

	listed[0].FileID = "mutated"
	listed[0].Chunks[0].ChunkID = "mutated"

	original, err := service.GetMetadata("a")
	if err != nil {
		t.Fatalf("GetMetadata(a) error = %v", err)
	}
	if original.FileID != "a" {
		t.Fatalf("GetMetadata(a).FileID = %q, want a", original.FileID)
	}
	if original.Chunks[0].ChunkID != "chunk-a" {
		t.Fatalf("GetMetadata(a).Chunks[0].ChunkID = %q, want chunk-a", original.Chunks[0].ChunkID)
	}
}
