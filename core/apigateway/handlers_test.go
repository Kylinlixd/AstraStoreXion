package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLifecycle(t *testing.T) {
	router := newTestRouter(t, "test-token", 1024)
	upload := multipartUpload(t, "/api/v1/files", "hello.txt", "text/plain", []byte("hello"))
	authorize(upload, "test-token")
	created := doJSON[files.File](t, router, upload, http.StatusCreated)
	assert.Equal(t, sha256Hex("hello"), created.Checksum)
	assert.EqualValues(t, 5, created.Size)

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+created.ID+"/status", nil)
	authorize(statusRequest, "test-token")
	status := doJSON[files.File](t, router, statusRequest, http.StatusOK)
	assert.Equal(t, created, status)

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/files?limit=10&offset=0", nil)
	authorize(listRequest, "test-token")
	listed := doJSON[fileListResponse](t, router, listRequest, http.StatusOK)
	assert.Equal(t, 1, listed.Count)
	require.Len(t, listed.Results, 1)
	assert.Equal(t, created.ID, listed.Results[0].ID)

	download := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+created.ID, nil)
	authorize(download, "test-token")
	downloadResponse := httptest.NewRecorder()
	router.ServeHTTP(downloadResponse, download)
	assert.Equal(t, http.StatusOK, downloadResponse.Code)
	assert.Equal(t, "hello", downloadResponse.Body.String())
	assert.Equal(t, "text/plain", downloadResponse.Header().Get("Content-Type"))
	assert.Contains(t, downloadResponse.Header().Get("Content-Disposition"), "hello.txt")

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+created.ID, nil)
	authorize(deleteRequest, "test-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	assert.Equal(t, http.StatusNoContent, deleteResponse.Code)

	secondDelete := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+created.ID, nil)
	authorize(secondDelete, "test-token")
	secondDeleteResponse := httptest.NewRecorder()
	router.ServeHTTP(secondDeleteResponse, secondDelete)
	assert.Equal(t, http.StatusNoContent, secondDeleteResponse.Code)

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+created.ID+"/status", nil)
	authorize(missing, "test-token")
	errorResponse := doJSON[apiErrorResponse](t, router, missing, http.StatusNotFound)
	assert.Equal(t, "not_found", errorResponse.Error.Code)
}

func TestFileRoutesRequireExactServiceToken(t *testing.T) {
	router := newTestRouter(t, "test-token", 1024)

	for name, token := range map[string]string{"missing": "", "wrong": "wrong-token"} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
			if token != "" {
				authorize(request, token)
			}
			response := doJSON[apiErrorResponse](t, router, request, http.StatusUnauthorized)
			assert.Equal(t, "unauthorized", response.Error.Code)
		})
	}
}

func TestUploadRejectsFilesAboveConfiguredLimit(t *testing.T) {
	router := newTestRouter(t, "test-token", 4)
	request := multipartUpload(t, "/api/v1/files", "large.txt", "text/plain", []byte("12345"))
	authorize(request, "test-token")
	response := doJSON[apiErrorResponse](t, router, request, http.StatusRequestEntityTooLarge)
	assert.Equal(t, "file_too_large", response.Error.Code)
}

func TestUploadStartsStreamingBeforeMultipartBodyFinishes(t *testing.T) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	service := &streamProbeService{entered: make(chan struct{})}
	router := newRouter(gateway{service: service, token: "test-token", maxUploadBytes: 1024})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/files", reader)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	authorize(request, "test-token")
	response := httptest.NewRecorder()
	releaseBody := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		defer writer.Close()
		if err := multipartWriter.WriteField("metadata", `{"owner":"blog"}`); err != nil {
			writeDone <- err
			return
		}
		part, err := multipartWriter.CreateFormFile("file", "stream.txt")
		if err != nil {
			writeDone <- err
			return
		}
		<-releaseBody
		if _, err := part.Write([]byte("streamed")); err != nil {
			writeDone <- err
			return
		}
		writeDone <- multipartWriter.Close()
	}()
	handleDone := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(handleDone)
	}()

	streamedBeforeFinish := false
	select {
	case <-service.entered:
		streamedBeforeFinish = true
	case <-time.After(time.Second):
	}
	close(releaseBody)
	require.NoError(t, <-writeDone)
	<-handleDone

	assert.True(t, streamedBeforeFinish, "upload handler buffered the whole multipart request")
	assert.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	assert.Equal(t, []byte("streamed"), service.received)
	assert.Equal(t, map[string]string{"owner": "blog"}, service.metadata)
}

func TestHealthAndReadinessArePublic(t *testing.T) {
	router := newTestRouter(t, "test-token", 1024)

	health := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthResponse := doJSON[statusResponse](t, router, health, http.StatusOK)
	assert.Equal(t, "ok", healthResponse.Status)
	healthz := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	doJSON[statusResponse](t, router, healthz, http.StatusOK)

	ready := httptest.NewRequest(http.MethodGet, "/ready", nil)
	readyResponse := doJSON[statusResponse](t, router, ready, http.StatusOK)
	assert.Equal(t, "ready", readyResponse.Status)
	readyz := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	doJSON[statusResponse](t, router, readyz, http.StatusOK)
}

func TestCapacityRequiresTokenAndReturnsStorageStats(t *testing.T) {
	store, err := files.NewDiskStoreWithPause(t.TempDir(), 90)
	require.NoError(t, err)
	router := newRouter(gateway{service: files.NewService(store), token: "test-token", maxUploadBytes: 1024})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/files/capacity", nil)
	response := doJSON[apiErrorResponse](t, router, request, http.StatusUnauthorized)
	assert.Equal(t, "unauthorized", response.Error.Code)

	authorized := httptest.NewRequest(http.MethodGet, "/api/v1/files/capacity", nil)
	authorize(authorized, "test-token")
	capacity := doJSON[files.Capacity](t, router, authorized, http.StatusOK)
	assert.Greater(t, capacity.TotalBytes, uint64(0))
	assert.Equal(t, 0, capacity.ObjectCount)
	assert.Equal(t, 90, capacity.PauseAtPercent)
}

func TestUploadMapsStoragePausedToInsufficientStorage(t *testing.T) {
	service := &pausedService{}
	router := newRouter(gateway{service: service, token: "test-token", maxUploadBytes: 1024})
	request := multipartUpload(t, "/api/v1/files", "paused.txt", "text/plain", []byte("hello"))
	authorize(request, "test-token")
	response := doJSON[apiErrorResponse](t, router, request, http.StatusInsufficientStorage)
	assert.Equal(t, "storage_paused", response.Error.Code)
}

func TestResumableUploadLifecycle(t *testing.T) {
	router := newTestRouter(t, "test-token", 1024)
	startRequest := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", stringsReader(`{"filename":"large.txt","content_type":"text/plain","size":11,"checksum":"`+sha256Hex("hello world")+`","metadata":{"owner":"blog"}}`))
	startRequest.Header.Set("Content-Type", "application/json")
	authorize(startRequest, "test-token")
	session := doJSON[files.UploadSession](t, router, startRequest, http.StatusCreated)
	assert.Equal(t, files.UploadStatusUploading, session.Status)
	assert.Zero(t, session.ReceivedBytes)

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/"+session.ID, nil)
	authorize(statusRequest, "test-token")
	status := doJSON[files.UploadSession](t, router, statusRequest, http.StatusOK)
	assert.Equal(t, session.ID, status.ID)

	firstChunk := httptest.NewRequest(http.MethodPut, "/api/v1/uploads/"+session.ID, stringsReader("hello "))
	firstChunk.Header.Set("Content-Range", "bytes 0-5/11")
	authorize(firstChunk, "test-token")
	updated := doJSON[files.UploadSession](t, router, firstChunk, http.StatusOK)
	assert.EqualValues(t, 6, updated.ReceivedBytes)

	wrongOffset := httptest.NewRequest(http.MethodPut, "/api/v1/uploads/"+session.ID, stringsReader("world"))
	wrongOffset.Header.Set("Content-Range", "bytes 0-4/11")
	authorize(wrongOffset, "test-token")
	offsetError := doJSON[apiErrorResponse](t, router, wrongOffset, http.StatusConflict)
	assert.Equal(t, "upload_offset_conflict", offsetError.Error.Code)

	incomplete := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/"+session.ID+"/complete", nil)
	authorize(incomplete, "test-token")
	incompleteError := doJSON[apiErrorResponse](t, router, incomplete, http.StatusConflict)
	assert.Equal(t, "upload_incomplete", incompleteError.Error.Code)

	secondChunk := httptest.NewRequest(http.MethodPut, "/api/v1/uploads/"+session.ID, stringsReader("world"))
	secondChunk.Header.Set("Content-Range", "bytes 6-10/11")
	secondChunk.Header.Set("X-Chunk-Checksum", sha256Hex("world"))
	authorize(secondChunk, "test-token")
	done := doJSON[files.UploadSession](t, router, secondChunk, http.StatusOK)
	assert.EqualValues(t, 11, done.ReceivedBytes)

	complete := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/"+session.ID+"/complete", nil)
	authorize(complete, "test-token")
	created := doJSON[files.File](t, router, complete, http.StatusCreated)
	assert.Equal(t, sha256Hex("hello world"), created.Checksum)
	assert.Equal(t, files.StatusAvailable, created.Status)

	completedStatus := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/"+session.ID, nil)
	authorize(completedStatus, "test-token")
	completed := doJSON[files.UploadSession](t, router, completedStatus, http.StatusOK)
	assert.Equal(t, files.UploadStatusCompleted, completed.Status)
	assert.Equal(t, created.ID, completed.FileID)
}

func TestResumableUploadRejectsInvalidChunkChecksumAndRequiresToken(t *testing.T) {
	router := newTestRouter(t, "test-token", 1024)
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", stringsReader(`{"filename":"file.txt","size":4}`))
	doJSON[apiErrorResponse](t, router, unauthorized, http.StatusUnauthorized)

	startRequest := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", stringsReader(`{"filename":"file.txt","size":4}`))
	startRequest.Header.Set("Content-Type", "application/json")
	authorize(startRequest, "test-token")
	session := doJSON[files.UploadSession](t, router, startRequest, http.StatusCreated)

	chunk := httptest.NewRequest(http.MethodPut, "/api/v1/uploads/"+session.ID, stringsReader("data"))
	chunk.Header.Set("Content-Range", "bytes 0-3/4")
	chunk.Header.Set("X-Chunk-Checksum", strings.Repeat("0", 64))
	authorize(chunk, "test-token")
	errorResponse := doJSON[apiErrorResponse](t, router, chunk, http.StatusUnprocessableEntity)
	assert.Equal(t, "upload_checksum_mismatch", errorResponse.Error.Code)

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/uploads/"+session.ID, nil)
	authorize(statusRequest, "test-token")
	status := doJSON[files.UploadSession](t, router, statusRequest, http.StatusOK)
	assert.Zero(t, status.ReceivedBytes)
}

func TestTrashListRestoreAndDownload(t *testing.T) {
	router := newTestRouter(t, "test-token", 1024)
	upload := multipartUpload(t, "/api/v1/files", "restore.txt", "text/plain", []byte("restore me"))
	authorize(upload, "test-token")
	created := doJSON[files.File](t, router, upload, http.StatusCreated)

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+created.ID, nil)
	authorize(deleteRequest, "test-token")
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, deleteRequest)
	assert.Equal(t, http.StatusNoContent, deleteResponse.Code)

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/trash", nil)
	doJSON[apiErrorResponse](t, router, unauthorized, http.StatusUnauthorized)

	trashRequest := httptest.NewRequest(http.MethodGet, "/api/v1/trash?limit=10&offset=0", nil)
	authorize(trashRequest, "test-token")
	trash := doJSON[fileListResponse](t, router, trashRequest, http.StatusOK)
	require.Len(t, trash.Results, 1)
	assert.Equal(t, created.ID, trash.Results[0].ID)
	assert.NotNil(t, trash.Results[0].DeletedAt)

	restoreRequest := httptest.NewRequest(http.MethodPost, "/api/v1/files/"+created.ID+"/restore", nil)
	authorize(restoreRequest, "test-token")
	restored := doJSON[files.File](t, router, restoreRequest, http.StatusOK)
	assert.Equal(t, created.ID, restored.ID)

	download := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+created.ID, nil)
	authorize(download, "test-token")
	downloadResponse := httptest.NewRecorder()
	router.ServeHTTP(downloadResponse, download)
	assert.Equal(t, http.StatusOK, downloadResponse.Code)
	assert.Equal(t, "restore me", downloadResponse.Body.String())

	missingRestore := httptest.NewRequest(http.MethodPost, "/api/v1/files/b8c21d60-e970-4df5-890b-0d2dba93a654/restore", nil)
	authorize(missingRestore, "test-token")
	errorResponse := doJSON[apiErrorResponse](t, router, missingRestore, http.StatusNotFound)
	assert.Equal(t, "not_found", errorResponse.Error.Code)
}

func newTestRouter(t *testing.T, token string, maxUploadBytes int64) http.Handler {
	t.Helper()
	store, err := files.NewDiskStore(t.TempDir())
	require.NoError(t, err)
	return newRouter(gateway{
		service:        files.NewService(store),
		token:          token,
		maxUploadBytes: maxUploadBytes,
	})
}

func stringsReader(value string) io.Reader {
	return strings.NewReader(value)
}

func multipartUpload(t *testing.T, target, filename, contentType string, contents []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(contents)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func authorize(request *http.Request, token string) {
	request.Header.Set("Authorization", "Bearer "+token)
}

func doJSON[T any](t *testing.T, handler http.Handler, request *http.Request, expectedStatus int) T {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, expectedStatus, response.Code, response.Body.String())
	var decoded T
	if expectedStatus != http.StatusNoContent {
		require.NoError(t, json.NewDecoder(response.Body).Decode(&decoded), response.Body.String())
	}
	return decoded
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type streamProbeService struct {
	entered  chan struct{}
	received []byte
	metadata map[string]string
}

func (s *streamProbeService) Upload(_ context.Context, input files.UploadInput) (files.File, error) {
	close(s.entered)
	received, err := io.ReadAll(input.Reader)
	s.received = received
	s.metadata = input.Metadata
	return files.File{ID: "b8c21d60-e970-4df5-890b-0d2dba93a654", Name: input.Name}, err
}

func (*streamProbeService) Download(context.Context, string) (files.File, io.ReadCloser, error) {
	panic("unexpected Download call")
}

func (*streamProbeService) Status(context.Context, string) (files.File, error) {
	panic("unexpected Status call")
}

func (*streamProbeService) List(context.Context, int, int) ([]files.File, error) {
	panic("unexpected List call")
}

func (*streamProbeService) Delete(context.Context, string) error { return nil }
func (*streamProbeService) Ready(context.Context) error          { return nil }
func (*streamProbeService) Capacity(context.Context) (files.Capacity, error) {
	return files.Capacity{}, nil
}

type pausedService struct{}

func (*pausedService) Upload(context.Context, files.UploadInput) (files.File, error) {
	return files.File{}, files.ErrStoragePaused
}
func (*pausedService) Download(context.Context, string) (files.File, io.ReadCloser, error) {
	panic("unexpected Download call")
}
func (*pausedService) Status(context.Context, string) (files.File, error) {
	panic("unexpected Status call")
}
func (*pausedService) List(context.Context, int, int) ([]files.File, error) {
	panic("unexpected List call")
}
func (*pausedService) Delete(context.Context, string) error             { return nil }
func (*pausedService) Ready(context.Context) error                      { return nil }
func (*pausedService) Capacity(context.Context) (files.Capacity, error) { return files.Capacity{}, nil }
