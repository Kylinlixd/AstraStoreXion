package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskStoreResumableUploadPersistsOffsetAcrossRestart(t *testing.T) {
	root := t.TempDir()
	first, err := NewDiskStore(root)
	require.NoError(t, err)

	session, err := first.StartUpload(context.Background(), MultipartStartInput{
		Name: "large.bin", ContentType: "application/octet-stream", Size: 11,
		Checksum: sha256String("hello world"), Metadata: map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)
	assert.Equal(t, UploadStatusUploading, session.Status)
	assert.Zero(t, session.ReceivedBytes)

	updated, err := first.AppendUpload(context.Background(), session.ID, 0, strings.NewReader("hello "), "")
	require.NoError(t, err)
	assert.EqualValues(t, 6, updated.ReceivedBytes)

	second, err := NewDiskStore(root)
	require.NoError(t, err)
	restored, err := second.GetUpload(context.Background(), session.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 6, restored.ReceivedBytes)
	assert.Equal(t, session.ID, restored.ID)
}

func TestDiskStoreResumableUploadRejectsOffsetAndChunkChecksum(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	session, err := store.StartUpload(context.Background(), MultipartStartInput{Name: "file.txt", Size: 5})
	require.NoError(t, err)

	_, err = store.AppendUpload(context.Background(), session.ID, 1, strings.NewReader("hello"), "")
	assert.ErrorIs(t, err, ErrUploadOffsetConflict)

	_, err = store.AppendUpload(context.Background(), session.ID, 0, strings.NewReader("hello"), strings.Repeat("0", 64))
	assert.ErrorIs(t, err, ErrUploadChecksumMismatch)
	updated, err := store.GetUpload(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Zero(t, updated.ReceivedBytes)
}

func TestDiskStoreResumableUploadCompletesAtomicallyAndValidatesChecksum(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	want := "hello world"
	sum := sha256.Sum256([]byte(want))
	session, err := store.StartUpload(context.Background(), MultipartStartInput{
		Name: "hello.txt", ContentType: "text/plain", Size: int64(len(want)), Checksum: hex.EncodeToString(sum[:]),
	})
	require.NoError(t, err)

	_, err = store.AppendUpload(context.Background(), session.ID, 0, strings.NewReader("hello"), "")
	require.NoError(t, err)
	_, err = store.CompleteUpload(context.Background(), session.ID)
	assert.ErrorIs(t, err, ErrUploadIncomplete)

	_, err = store.AppendUpload(context.Background(), session.ID, 5, strings.NewReader(" world"), sha256String(" world"))
	require.NoError(t, err)
	created, err := store.CompleteUpload(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusAvailable, created.Status)
	assert.Equal(t, sha256String(want), created.Checksum)

	completed, err := store.GetUpload(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Equal(t, UploadStatusCompleted, completed.Status)
	assert.Equal(t, created.ID, completed.FileID)

	_, body, err := store.Open(context.Background(), created.ID)
	require.NoError(t, err)
	defer body.Close()
	contents := make([]byte, len(want))
	_, err = body.Read(contents)
	require.NoError(t, err)
	assert.Equal(t, want, string(contents))
}

func TestDiskStoreAbortResumableUploadIsIdempotent(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	session, err := store.StartUpload(context.Background(), MultipartStartInput{Name: "cancel.bin", Size: 4})
	require.NoError(t, err)
	require.NoError(t, store.AbortUpload(context.Background(), session.ID))
	require.NoError(t, store.AbortUpload(context.Background(), session.ID))
	_, err = store.GetUpload(context.Background(), session.ID)
	assert.ErrorIs(t, err, ErrUploadNotFound)
}
