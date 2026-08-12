package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testToken = "test-service-token"

func TestLoadConfigUsesCLIOverEnvironmentAndEnvFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "astrastore-xion.env")
	if err := os.WriteFile(envPath, []byte("XION_LISTEN_ADDR=env-file:8081\nXION_SERVICE_TOKEN=file-token\nXION_TIMEOUT=7s\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(envPath, map[string]string{
		"XION_API_URL":       "http://process:8081",
		"XION_SERVICE_TOKEN": "process-token",
		"XION_TIMEOUT":       "9s",
	}, configOverrides{apiURL: "http://cli:8081", token: "cli-token"})
	if err != nil {
		t.Fatal(err)
	}
	if config.apiURL != "http://cli:8081" || config.token != "cli-token" || config.timeout.String() != "9s" {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestRunHealthChecksBothEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("authorization = %q", got)
		}
		if r.URL.Path == "/healthz" {
			writeTestJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if r.URL.Path == "/readyz" {
			writeTestJSON(w, http.StatusOK, map[string]string{"status": "ready"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--api", server.URL, "--token", testToken, "health"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "ok (ready)" {
		t.Fatalf("stdout = %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunListAndInfoPrintJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/files" {
			writeTestJSON(w, http.StatusOK, map[string]any{"count": 1, "results": []map[string]any{{"file_id": "file-1", "filename": "hello.txt", "size": 5, "status": "available"}}})
			return
		}
		if r.URL.Path == "/api/v1/files/file-1/status" {
			writeTestJSON(w, http.StatusOK, map[string]any{"file_id": "file-1", "filename": "hello.txt", "size": 5, "status": "available"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	for _, args := range [][]string{{"--api", server.URL, "--token", testToken, "list"}, {"--api", server.URL, "--token", testToken, "info", "file-1"}} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
			t.Fatalf("output is not JSON: %v; output=%q", err, stdout.String())
		}
	}
}

func TestRunCapacityPrintsAuthenticatedStorageStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files/capacity" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Fatalf("authorization = %q", got)
		}
		writeTestJSON(w, http.StatusOK, map[string]any{
			"total_bytes": 1000, "used_bytes": 900, "available_bytes": 100,
			"used_percent": 90, "pause_at_percent": 90, "writes_paused": true,
		})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--api", server.URL, "--token", testToken, "capacity"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["writes_paused"] != true || decoded["used_percent"] != float64(90) {
		t.Fatalf("capacity output = %s", stdout.String())
	}
}

func TestRunUploadShowsFriendlyStoragePausedMessage(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(fixture, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(w, http.StatusInsufficientStorage, map[string]any{"error": map[string]any{
			"code": "storage_paused", "message": "存储空间已达到安全阈值，暂时停止上传；释放空间后会自动恢复。",
		}})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run([]string{"--api", server.URL, "--token", testToken, "upload", fixture}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "上传已暂停") || !strings.Contains(err.Error(), "自动恢复") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunUploadUsesMultipartFile(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(fixture, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/files" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() != "file" || part.FileName() != "hello.txt" || string(body) != "hello" {
			t.Errorf("unexpected multipart file: name=%q filename=%q body=%q", part.FormName(), part.FileName(), body)
		}
		writeTestJSON(w, http.StatusCreated, map[string]any{"file_id": "file-1", "filename": "hello.txt", "size": 5, "status": "available"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--api", server.URL, "--token", testToken, "upload", fixture}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "file-1") {
		t.Fatalf("upload output = %q", stdout.String())
	}
}

func TestRunDownloadWritesAtomically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files/file-1" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "downloaded")
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "nested", "download.txt")

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--api", server.URL, "--token", testToken, "download", "file-1", target}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "downloaded" {
		t.Fatalf("downloaded data = %q", data)
	}
}

func TestRunDeleteRequiresExplicitConfirmation(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run([]string{"--api", server.URL, "--token", testToken, "delete", "file-1"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("delete request was sent without confirmation")
	}

	stdout.Reset()
	if err := run([]string{"--api", server.URL, "--token", testToken, "delete", "file-1", "--yes"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
}

func TestRunLogsUsesSafeJournalctlArguments(t *testing.T) {
	var gotArgs []string
	fakeRunner := func(_ context.Context, stdout, _ io.Writer, args []string) error {
		gotArgs = append([]string(nil), args...)
		_, _ = io.WriteString(stdout, "Aug 13 05:00:00 host astrastore-xion[1]: ready\n")
		return nil
	}
	var stdout, stderr bytes.Buffer
	if err := runWithJournalRunner([]string{"logs", "--lines", "12", "--since", "1h", "--follow"}, &stdout, &stderr, fakeRunner); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(gotArgs, " "); got != "-u astrastore-xion.service --no-pager --output=cat -n 12 --since 1h -f" {
		t.Fatalf("journalctl args = %q", got)
	}
	if !strings.Contains(stdout.String(), "ready") {
		t.Fatalf("logs output = %q", stdout.String())
	}
}

func TestRunLogsDefaultsToLastHundredLines(t *testing.T) {
	var gotArgs []string
	fakeRunner := func(_ context.Context, _, _ io.Writer, args []string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	var stdout, stderr bytes.Buffer
	if err := runWithJournalRunner([]string{"logs"}, &stdout, &stderr, fakeRunner); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(gotArgs, " "); got != "-u astrastore-xion.service --no-pager --output=cat -n 100" {
		t.Fatalf("journalctl args = %q", got)
	}
}

func TestRunLogsPropagatesJournalError(t *testing.T) {
	wanted := errors.New("journal permission denied")
	fakeRunner := func(_ context.Context, _, _ io.Writer, _ []string) error { return wanted }
	var stdout, stderr bytes.Buffer
	if err := runWithJournalRunner([]string{"logs", "--lines", "5"}, &stdout, &stderr, fakeRunner); !errors.Is(err, wanted) {
		t.Fatalf("error = %v", err)
	}
}

func writeTestJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func TestMultipartContentTypeHelper(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("hello"))
	_ = writer.Close()
	if body.Len() == 0 {
		t.Fatal("multipart body is empty")
	}
}
