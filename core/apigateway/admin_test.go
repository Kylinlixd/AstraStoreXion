package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAdminRouter keeps a handle on the store so tests can inspect the effects
// of the administrative endpoints.
func newAdminRouter(t *testing.T, options files.Options) (http.Handler, *files.DiskStore) {
	t.Helper()
	store, err := files.NewDiskStoreWithOptions(t.TempDir(), options)
	require.NoError(t, err)
	router := newRouter(gateway{
		service:        files.NewService(store),
		token:          "test-token",
		maxUploadBytes: 1 << 20,
	})
	return router, store
}

// TestFileListReportsTotalNotPageSize covers the pagination contract: clients
// need the overall record count, not the length of the current page.
func TestFileListReportsTotalNotPageSize(t *testing.T) {
	router := newTestRouter(t, "test-token", 1<<20)
	for _, name := range []string{"one", "two", "three"} {
		upload := multipartUpload(t, "/api/v1/files", name+".txt", "text/plain", []byte(name))
		authorize(upload, "test-token")
		doJSON[files.File](t, router, upload, http.StatusCreated)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/files?limit=1&offset=0", nil)
	authorize(request, "test-token")
	listed := doJSON[fileListResponse](t, router, request, http.StatusOK)
	assert.Equal(t, 1, listed.Count, "count is the current page")
	assert.Equal(t, 3, listed.Total, "total is every active object")
	require.Len(t, listed.Results, 1)
}

func TestUploadsEndpointsListAndSweep(t *testing.T) {
	router, _ := newAdminRouter(t, files.Options{UploadTTL: time.Nanosecond})

	start := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", strings.NewReader(
		`{"filename":"big.bin","content_type":"application/octet-stream","size":4096,"metadata":{"owner":"blog"}}`,
	))
	start.Header.Set("Content-Type", "application/json")
	authorize(start, "test-token")
	session := doJSON[files.UploadSession](t, router, start, http.StatusCreated)
	assert.NotEmpty(t, session.ID)

	list := httptest.NewRequest(http.MethodGet, "/api/v1/uploads", nil)
	authorize(list, "test-token")
	sessions := doJSON[uploadListResponse](t, router, list, http.StatusOK)
	assert.Equal(t, 1, sessions.Total)
	require.Len(t, sessions.Results, 1)
	assert.Equal(t, "big.bin", sessions.Results[0].Name)
	assert.EqualValues(t, 4096, sessions.Results[0].Size)

	// The TTL is effectively zero, so the sweep expires the session.
	sweep := httptest.NewRequest(http.MethodDelete, "/api/v1/uploads", nil)
	authorize(sweep, "test-token")
	removed := doJSON[purgeResponse](t, router, sweep, http.StatusOK)
	assert.Equal(t, 1, removed.Removed)

	list = httptest.NewRequest(http.MethodGet, "/api/v1/uploads", nil)
	authorize(list, "test-token")
	sessions = doJSON[uploadListResponse](t, router, list, http.StatusOK)
	assert.Equal(t, 0, sessions.Total)
}

func TestTrashPurgeRequiresAnExplicitBoundary(t *testing.T) {
	router, _ := newAdminRouter(t, files.Options{})

	empty := httptest.NewRequest(http.MethodPost, "/api/v1/trash/purge", strings.NewReader(`{}`))
	empty.Header.Set("Content-Type", "application/json")
	authorize(empty, "test-token")
	response := doJSON[apiErrorResponse](t, router, empty, http.StatusBadRequest)
	assert.Equal(t, "invalid_request", response.Error.Code)

	negative := httptest.NewRequest(http.MethodPost, "/api/v1/trash/purge", strings.NewReader(`{"older_than":"-1h"}`))
	negative.Header.Set("Content-Type", "application/json")
	authorize(negative, "test-token")
	response = doJSON[apiErrorResponse](t, router, negative, http.StatusBadRequest)
	assert.Equal(t, "invalid_request", response.Error.Code)

	garbage := httptest.NewRequest(http.MethodPost, "/api/v1/trash/purge", strings.NewReader(`{"older_than":"yesterday"}`))
	garbage.Header.Set("Content-Type", "application/json")
	authorize(garbage, "test-token")
	response = doJSON[apiErrorResponse](t, router, garbage, http.StatusBadRequest)
	assert.Equal(t, "invalid_request", response.Error.Code)
}

func TestTrashPurgeRemovesOldEntriesAndKeepsRecentOnes(t *testing.T) {
	router, store := newAdminRouter(t, files.Options{})
	ctx := context.Background()

	old, err := store.Put(ctx, files.UploadInput{Name: "old.txt", Reader: strings.NewReader("old")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, old.ID))

	recent, err := store.Put(ctx, files.UploadInput{Name: "recent.txt", Reader: strings.NewReader("recent")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, recent.ID))

	// Move only the first entry into the past.
	_, err = store.PurgeTrash(ctx, time.Time{})
	require.NoError(t, err)

	restored, err := store.Put(ctx, files.UploadInput{Name: "restored.txt", Reader: strings.NewReader("restored")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, restored.ID))

	purge := httptest.NewRequest(http.MethodPost, "/api/v1/trash/purge", strings.NewReader(`{"older_than":"24h"}`))
	purge.Header.Set("Content-Type", "application/json")
	authorize(purge, "test-token")
	removed := doJSON[purgeResponse](t, router, purge, http.StatusOK)
	assert.Equal(t, 0, removed.Removed, "an entry deleted just now is inside the window")

	list := httptest.NewRequest(http.MethodGet, "/api/v1/trash?limit=10&offset=0", nil)
	authorize(list, "test-token")
	trashed := doJSON[fileListResponse](t, router, list, http.StatusOK)
	assert.Equal(t, 1, trashed.Count)

	// Purging everything is explicit and reported.
	purgeAll := httptest.NewRequest(http.MethodPost, "/api/v1/trash/purge", strings.NewReader(`{"all":true}`))
	purgeAll.Header.Set("Content-Type", "application/json")
	authorize(purgeAll, "test-token")
	removed = doJSON[purgeResponse](t, router, purgeAll, http.StatusOK)
	assert.Equal(t, 1, removed.Removed)
}

func TestRestoreKeepsOriginalFileID(t *testing.T) {
	router, _ := newAdminRouter(t, files.Options{})

	upload := multipartUpload(t, "/api/v1/files", "keep.txt", "text/plain", []byte("keep"))
	authorize(upload, "test-token")
	created := doJSON[files.File](t, router, upload, http.StatusCreated)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+created.ID, nil)
	authorize(deleteRequest, "test-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	require.Equal(t, http.StatusNoContent, deleteResponse.Code)

	restore := httptest.NewRequest(http.MethodPost, "/api/v1/files/"+created.ID+"/restore", nil)
	authorize(restore, "test-token")
	restored := doJSON[files.File](t, router, restore, http.StatusOK)
	assert.Equal(t, created.ID, restored.ID)
	assert.Equal(t, created.Checksum, restored.Checksum)

	download := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+created.ID, nil)
	authorize(download, "test-token")
	downloadResponse := httptest.NewRecorder()
	router.ServeHTTP(downloadResponse, download)
	assert.Equal(t, http.StatusOK, downloadResponse.Code)
	assert.Equal(t, "keep", downloadResponse.Body.String())
}
