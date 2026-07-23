package files

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/astrastore/astrastore-xion/pkg/metadata"
	"github.com/astrastore/astrastore-xion/pkg/storage"
)

func TestServiceUploadsDownloadsListsAndDeletesFile(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewLocalStorageNode(storage.StorageConfig{
		NodeID:   "node-1",
		DataPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewLocalStorageNode() error = %v", err)
	}

	service := NewService(store, metadata.NewInMemoryMetadataService(), WithIDGenerator(func() string {
		return "file-test-1"
	}))

	uploaded, err := service.Upload(ctx, UploadInput{
		Name:        "hello.txt",
		ContentType: "text/plain",
		OwnerID:     "admin-001",
		Reader:      strings.NewReader("hello world"),
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if uploaded.ID != "file-test-1" {
		t.Fatalf("Upload().ID = %q, want file-test-1", uploaded.ID)
	}
	if uploaded.Size != 11 {
		t.Fatalf("Upload().Size = %d, want 11", uploaded.Size)
	}

	downloaded, err := service.Download(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer downloaded.Content.Close()

	body, err := io.ReadAll(downloaded.Content)
	if err != nil {
		t.Fatalf("ReadAll(downloaded.Content) error = %v", err)
	}
	if string(body) != "hello world" {
		t.Fatalf("downloaded body = %q, want hello world", string(body))
	}

	listed, err := service.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != uploaded.ID {
		t.Fatalf("List() = %+v, want uploaded file", listed)
	}

	if err := service.Delete(ctx, uploaded.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := service.Download(ctx, uploaded.ID); err != ErrNotFound {
		t.Fatalf("Download() after delete error = %v, want %v", err, ErrNotFound)
	}
}
