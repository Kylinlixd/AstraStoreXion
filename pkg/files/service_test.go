package files

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDelegatesCompleteFileLifecycle(t *testing.T) {
	store, err := NewDiskStore(t.TempDir())
	require.NoError(t, err)
	service := NewService(store)

	created, err := service.Upload(context.Background(), UploadInput{
		Name: "guide.txt", Reader: strings.NewReader("guide"),
	})
	require.NoError(t, err)

	status, err := service.Status(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, status)

	listed, err := service.List(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 1)

	require.NoError(t, service.Delete(context.Background(), created.ID))
	_, err = service.Status(context.Background(), created.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
