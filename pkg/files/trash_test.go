package files

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskStoreTrashDeleteListRestoreAndRestart(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskStore(root)
	require.NoError(t, err)
	saved, err := store.Put(context.Background(), UploadInput{
		Name: "cover.png", ContentType: "image/png", Reader: strings.NewReader("image"),
	})
	require.NoError(t, err)

	require.NoError(t, store.Delete(context.Background(), saved.ID))
	_, err = store.Get(context.Background(), saved.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	active, err := store.List(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Empty(t, active)
	trash, err := store.ListTrash(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Len(t, trash, 1)
	assert.Equal(t, saved.ID, trash[0].ID)
	assert.NotNil(t, trash[0].DeletedAt)

	capacity, err := store.Capacity(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, capacity.ObjectCount)
	assert.Equal(t, 1, capacity.TrashCount)
	assert.EqualValues(t, 5, capacity.TrashBytes)

	restarted, err := NewDiskStore(root)
	require.NoError(t, err)
	require.NoError(t, restarted.Ready(context.Background()))
	require.NoError(t, restarted.Delete(context.Background(), saved.ID))

	restored, err := restarted.Restore(context.Background(), saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, restored.ID)
	assert.Equal(t, saved.Checksum, restored.Checksum)
	_, body, err := restarted.Open(context.Background(), saved.ID)
	require.NoError(t, err)
	defer body.Close()
	contents, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "image", string(contents))
	trash, err = restarted.ListTrash(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Empty(t, trash)
}

func TestDiskStoreTrashDeleteIsIdempotentAndRestoreConflicts(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskStore(root)
	require.NoError(t, err)
	saved, err := store.Put(context.Background(), UploadInput{Name: "file.txt", Reader: strings.NewReader("data")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(context.Background(), saved.ID))
	require.NoError(t, store.Delete(context.Background(), saved.ID))

	activePath := filepath.Join(root, "objects", saved.ID)
	manifestPath := filepath.Join(root, "metadata", saved.ID+".json")
	manifest, err := json.Marshal(saved)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(activePath, []byte("conflict"), 0o640))
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o640))
	_, err = store.Restore(context.Background(), saved.ID)
	assert.ErrorIs(t, err, ErrRestoreConflict)
}
