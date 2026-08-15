package replication

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	file files.File
	body string
}

func (s fakeSource) Download(context.Context, string) (files.File, io.ReadCloser, error) {
	return s.file, io.NopCloser(strings.NewReader(s.body)), nil
}

func TestReplicationUploadAndDeleteJobs(t *testing.T) {
	var uploaded atomic.Bool
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer replica-secret", request.Header.Get("Authorization"))
		assert.Equal(t, "true", request.Header.Get("X-Xion-Replication"))
		switch request.Method {
		case http.MethodPost:
			assert.Equal(t, "/api/v1/files", request.URL.Path)
			_, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			require.NoError(t, err)
			reader := mustMultipartReader(t, request, params["boundary"])
			part, err := reader.NextPart()
			require.NoError(t, err)
			assert.Equal(t, "file", part.FormName())
			contents, err := io.ReadAll(part)
			require.NoError(t, err)
			assert.Equal(t, "replicated bytes", string(contents))
			uploaded.Store(true)
			writer.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			assert.Equal(t, "/api/v1/files/file-1", request.URL.Path)
			deleted.Store(true)
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	manager, err := NewManager(fakeSource{file: files.File{ID: "file-1", Name: "note.txt", ContentType: "text/plain", Size: 16}, body: "replicated bytes"}, Config{
		RemoteURL: server.URL, Token: "replica-secret", JobsDir: t.TempDir(), MaxAttempts: 3,
	})
	require.NoError(t, err)
	defer manager.Close()

	_, err = manager.EnqueueUpload(context.Background(), "file-1")
	require.NoError(t, err)
	require.NoError(t, manager.ProcessPending(context.Background()))
	status := manager.Status()
	assert.Equal(t, 1, status.Completed)
	assert.True(t, uploaded.Load())

	_, err = manager.EnqueueDelete(context.Background(), "file-1")
	require.NoError(t, err)
	require.NoError(t, manager.ProcessPending(context.Background()))
	status = manager.Status()
	assert.Equal(t, 2, status.Completed)
	assert.True(t, deleted.Load())
}

func TestReplicationReloadsJobsAndRetriesFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	jobsDir := t.TempDir()
	config := Config{RemoteURL: server.URL, Token: "secret", JobsDir: jobsDir, MaxAttempts: 3}
	first, err := NewManager(fakeSource{file: files.File{ID: "file-2", Name: "two.txt", Size: 3}, body: "two"}, config)
	require.NoError(t, err)
	_, err = first.EnqueueUpload(context.Background(), "file-2")
	require.NoError(t, err)
	first.Close()

	second, err := NewManager(fakeSource{file: files.File{ID: "file-2", Name: "two.txt", Size: 3}, body: "two"}, config)
	require.NoError(t, err)
	defer second.Close()
	jobs := second.List()
	require.Len(t, jobs, 1)
	assert.Equal(t, JobPending, jobs[0].Status)

	err = second.ProcessPending(context.Background())
	assert.Error(t, err)
	assert.Equal(t, 1, second.Status().Failed)
	require.NoError(t, second.ProcessPending(context.Background()))
	assert.Equal(t, 1, second.Status().Completed)
	assert.Equal(t, int32(2), attempts.Load())
}

func TestReplicationRetryAndRestoreJob(t *testing.T) {
	var restore atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/api/v1/files/file-3/restore", request.URL.Path)
		restore.Store(true)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	manager, err := NewManager(fakeSource{}, Config{RemoteURL: server.URL, Token: "secret", JobsDir: t.TempDir()})
	require.NoError(t, err)
	defer manager.Close()
	job, err := manager.EnqueueRestore(context.Background(), "file-3")
	require.NoError(t, err)
	require.NoError(t, manager.ProcessPending(context.Background()))
	assert.True(t, restore.Load())
	assert.Equal(t, JobCompleted, manager.job(job.ID).Status)

	require.NoError(t, manager.Retry(context.Background(), job.ID))
	assert.Equal(t, JobPending, manager.job(job.ID).Status)
}

func mustMultipartReader(t *testing.T, request *http.Request, mediaType string) *multipart.Reader {
	t.Helper()
	return multipart.NewReader(request.Body, mediaType)
}

func TestReplicationConfigRejectsMissingRemote(t *testing.T) {
	_, err := NewManager(fakeSource{}, Config{JobsDir: t.TempDir()})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
}
