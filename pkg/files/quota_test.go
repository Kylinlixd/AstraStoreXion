package files

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerQuotaReportsUsageAndRejectsOneShotUpload(t *testing.T) {
	store, err := NewDiskStoreWithPauseAndQuota(t.TempDir(), 0, 10)
	require.NoError(t, err)
	_, err = store.Put(context.Background(), UploadInput{
		Name: "first.txt", Reader: strings.NewReader("123456"), Metadata: map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)

	quota, err := store.Quota(context.Background(), "blog")
	require.NoError(t, err)
	assert.Equal(t, int64(10), quota.LimitBytes)
	assert.Equal(t, int64(6), quota.UsedBytes)
	assert.Equal(t, int64(4), quota.AvailableBytes)
	assert.InDelta(t, 60, quota.UsedPercent, 0.001)

	_, err = store.Put(context.Background(), UploadInput{
		Name: "too-large.txt", Reader: strings.NewReader("12345"), Metadata: map[string]string{"owner": "blog"},
	})
	assert.ErrorIs(t, err, ErrQuotaExceeded)

	other, err := store.Put(context.Background(), UploadInput{
		Name: "other.txt", Reader: strings.NewReader("12345"), Metadata: map[string]string{"owner": "other"},
	})
	require.NoError(t, err)
	assert.Equal(t, "other.txt", other.Name)
}

func TestOwnerQuotaCountsTrashAndResumableReservation(t *testing.T) {
	store, err := NewDiskStoreWithPauseAndQuota(t.TempDir(), 0, 10)
	require.NoError(t, err)
	saved, err := store.Put(context.Background(), UploadInput{
		Name: "trash.txt", Reader: strings.NewReader("123456"), Metadata: map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Delete(context.Background(), saved.ID))

	quota, err := store.Quota(context.Background(), "blog")
	require.NoError(t, err)
	assert.Equal(t, int64(6), quota.UsedBytes)
	_, err = store.StartUpload(context.Background(), MultipartStartInput{Name: "resume.bin", Size: 5, Metadata: map[string]string{"owner": "blog"}})
	assert.ErrorIs(t, err, ErrQuotaExceeded)

	_, err = store.StartUpload(context.Background(), MultipartStartInput{Name: "resume.bin", Size: 4, Metadata: map[string]string{"owner": "blog"}})
	require.NoError(t, err)
}

func TestOwnerQuotaDisabledByDefault(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	quota, err := store.Quota(context.Background(), "blog")
	require.NoError(t, err)
	assert.Zero(t, quota.LimitBytes)
	assert.Zero(t, quota.UsedPercent)
}
