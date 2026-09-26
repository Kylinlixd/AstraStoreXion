package replication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		case http.MethodGet:
			writer.WriteHeader(http.StatusNotFound)
		case http.MethodPost:
			assert.Equal(t, "/api/v1/files", request.URL.Path)
			assert.Equal(t, "file-1", request.Header.Get("X-Xion-Replication-File-ID"))
			_, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			require.NoError(t, err)
			reader := mustMultipartReader(t, request, params["boundary"])
			seenFile := false
			for {
				part, err := reader.NextPart()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
				contents, err := io.ReadAll(part)
				require.NoError(t, err)
				if part.FormName() == "file" {
					seenFile = true
					assert.Equal(t, "replicated bytes", string(contents))
				}
			}
			assert.True(t, seenFile)
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
		if request.Method == http.MethodGet {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	jobsDir := t.TempDir()
	config := Config{RemoteURL: server.URL, Token: "secret", JobsDir: jobsDir, MaxAttempts: 3, BackoffBase: -1}
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

func TestReplicationUploadSkipsAlreadySynchronizedStableFile(t *testing.T) {
	var postCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			assert.Equal(t, "/api/v1/files/file-4/status", request.URL.Path)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"file_id":"file-4","filename":"four.txt","size":4,"checksum":"` + sha256Hex("four") + `","status":"available"}`))
			return
		}
		postCalls.Add(1)
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	manager, err := NewManager(fakeSource{file: files.File{ID: "file-4", Name: "four.txt", Size: 4, Checksum: sha256Hex("four")}, body: "four"}, Config{
		RemoteURL: server.URL, Token: "secret", JobsDir: t.TempDir(),
	})
	require.NoError(t, err)
	defer manager.Close()
	_, err = manager.EnqueueUpload(context.Background(), "file-4")
	require.NoError(t, err)
	require.NoError(t, manager.ProcessPending(context.Background()))
	assert.Zero(t, postCalls.Load())
}

func mustMultipartReader(t *testing.T, request *http.Request, mediaType string) *multipart.Reader {
	t.Helper()
	return multipart.NewReader(request.Body, mediaType)
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func TestReplicationConfigRejectsMissingRemote(t *testing.T) {
	_, err := NewManager(fakeSource{}, Config{JobsDir: t.TempDir()})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
}

// TestReplicationFailureSchedulesBackoff pins the fix for the retry storm: a
// failed job must not be claimable again until its backoff window elapses.
func TestReplicationFailureSchedulesBackoff(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		attempts.Add(1)
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	manager, err := NewManager(fakeSource{file: files.File{ID: "file-9", Name: "nine.txt", Size: 4}, body: "nine"}, Config{
		RemoteURL: server.URL, Token: "secret", JobsDir: t.TempDir(), MaxAttempts: 5,
		BackoffBase: time.Hour, BackoffMax: time.Hour,
	})
	require.NoError(t, err)
	defer manager.Close()

	_, err = manager.EnqueueUpload(context.Background(), "file-9")
	require.NoError(t, err)
	require.Error(t, manager.ProcessPending(context.Background()))
	assert.Equal(t, int32(1), attempts.Load())

	job := manager.List()[0]
	assert.Equal(t, JobFailed, job.Status)
	assert.False(t, job.NextAttemptAt.IsZero(), "a failed job records when it may run again")
	assert.WithinDuration(t, time.Now().UTC().Add(time.Hour), job.NextAttemptAt, time.Minute)

	// The window has not elapsed, so no further attempt is made.
	require.NoError(t, manager.ProcessPending(context.Background()))
	assert.Equal(t, int32(1), attempts.Load(), "retry must wait for the backoff window")

	// A manual retry clears the window and runs immediately.
	require.NoError(t, manager.Retry(context.Background(), job.ID))
	require.Error(t, manager.ProcessPending(context.Background()))
	assert.Equal(t, int32(2), attempts.Load())
}
