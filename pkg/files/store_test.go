package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskStorePersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	first, err := NewDiskStore(root)
	require.NoError(t, err)

	saved, err := first.Put(context.Background(), UploadInput{
		Name:        "hello.txt",
		ContentType: "text/plain",
		Reader:      strings.NewReader("hello"),
		Metadata:    map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)
	assert.Equal(t, sha256String("hello"), saved.Checksum)
	assert.EqualValues(t, 5, saved.Size)

	second, err := NewDiskStore(root)
	require.NoError(t, err)
	got, body, err := second.Open(context.Background(), saved.ID)
	require.NoError(t, err)
	defer body.Close()

	contents, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, saved, got)
	assert.Equal(t, []byte("hello"), contents)
}

func TestDiskStoreRejectsUnsafeID(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)

	for _, id := range []string{"../outside", "a/b", "", strings.Repeat("a", 36)} {
		_, _, err := store.Open(context.Background(), id)
		assert.ErrorIs(t, err, ErrInvalidID, id)
	}
}

func TestDiskStoreListIsStableAndDetached(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)

	first, err := store.Put(context.Background(), UploadInput{
		Name: "one.txt", Reader: strings.NewReader("one"), Metadata: map[string]string{"key": "original"},
	})
	require.NoError(t, err)
	time.Sleep(time.Millisecond)
	second, err := store.Put(context.Background(), UploadInput{
		Name: "two.txt", Reader: strings.NewReader("two"),
	})
	require.NoError(t, err)

	listed, err := store.List(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, []string{second.ID, first.ID}, []string{listed[0].ID, listed[1].ID})

	listed[1].Metadata["key"] = "mutated"
	original, err := store.Get(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, "original", original.Metadata["key"])

	paged, err := store.List(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Len(t, paged, 1)
	assert.Equal(t, first.ID, paged[0].ID)
}

func TestDiskStoreDeleteIsIdempotent(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	saved, err := store.Put(context.Background(), UploadInput{Name: "gone.txt", Reader: strings.NewReader("gone")})
	require.NoError(t, err)

	require.NoError(t, store.Delete(context.Background(), saved.ID))
	require.NoError(t, store.Delete(context.Background(), saved.ID))
	_, err = store.Get(context.Background(), saved.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDiskStoreReadyChecksWritableDirectories(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.Ready(context.Background()))
}

func TestDiskStoreReadyRejectsIncompleteCrashArtifacts(t *testing.T) {
	const orphanID = "b8c21d60-e970-4df5-890b-0d2dba93a654"

	tests := map[string]func(string) error{
		"object without manifest": func(root string) error {
			return os.WriteFile(filepath.Join(root, "objects", orphanID), []byte("orphan"), 0o640)
		},
		"temporary manifest": func(root string) error {
			return os.WriteFile(filepath.Join(root, "metadata", orphanID+".json.tmp"), []byte("{}"), 0o640)
		},
		"partial object": func(root string) error {
			return os.WriteFile(filepath.Join(root, "tmp", orphanID+".part"), []byte("partial"), 0o640)
		},
	}

	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			store, err := NewDiskStore(root)
			require.NoError(t, err)
			require.NoError(t, arrange(root))

			err = store.Ready(context.Background())

			assert.ErrorIs(t, err, ErrCorruptMetadata)
		})
	}
}

func sha256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
