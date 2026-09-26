package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type xionClient struct {
	config config
	http   *http.Client
}

func (c xionClient) request(ctx context.Context, method, path string, body io.Reader, contentType string, expected ...int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.config.apiURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.config.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.token)
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	for _, status := range expected {
		if response.StatusCode == status {
			return io.ReadAll(response.Body)
		}
	}
	return nil, readXionError(response)
}

func readXionError(response *http.Response) error {
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &envelope) == nil && envelope.Error.Message != "" {
		if envelope.Error.Code == "storage_paused" {
			return fmt.Errorf("上传已暂停：%s", envelope.Error.Message)
		}
		return fmt.Errorf("xion: %s (%s, HTTP %d)", envelope.Error.Message, envelope.Error.Code, response.StatusCode)
	}
	return fmt.Errorf("xion returned HTTP %d", response.StatusCode)
}

func (c xionClient) health(ctx context.Context) (string, error) {
	health, err := c.request(ctx, http.MethodGet, "/healthz", nil, "", http.StatusOK)
	if err != nil {
		return "", err
	}
	ready, err := c.request(ctx, http.MethodGet, "/readyz", nil, "", http.StatusOK)
	if err != nil {
		return "", err
	}
	var healthStatus, readyStatus struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(health, &healthStatus); err != nil {
		return "", fmt.Errorf("parse health response: %w", err)
	}
	if err := json.Unmarshal(ready, &readyStatus); err != nil {
		return "", fmt.Errorf("parse readiness response: %w", err)
	}
	return healthStatus.Status + " (" + readyStatus.Status + ")", nil
}

func (c xionClient) list(ctx context.Context, limit, offset int) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/files?limit=%d&offset=%d", limit, offset)
	return c.request(ctx, http.MethodGet, path, nil, "", http.StatusOK)
}

func (c xionClient) capacity(ctx context.Context) ([]byte, error) {
	return c.request(ctx, http.MethodGet, "/api/v1/files/capacity", nil, "", http.StatusOK)
}

func (c xionClient) info(ctx context.Context, id string) ([]byte, error) {
	return c.request(ctx, http.MethodGet, "/api/v1/files/"+url.PathEscape(id)+"/status", nil, "", http.StatusOK)
}

func (c xionClient) upload(ctx context.Context, path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open upload file: %w", err)
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("create multipart file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("read upload file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart body: %w", err)
	}
	return c.request(ctx, http.MethodPost, "/api/v1/files", &body, writer.FormDataContentType(), http.StatusCreated)
}

func (c xionClient) download(ctx context.Context, id, target string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.apiURL+"/api/v1/files/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.token)
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return readXionError(response)
	}
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(target)+".xionctl-*")
	if err != nil {
		return fmt.Errorf("create temporary download: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := io.Copy(temporary, response.Body); err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync download: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close download: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("commit download: %w", err)
	}
	removeTemporary = false
	return nil
}

func (c xionClient) delete(ctx context.Context, id string) error {
	_, err := c.request(ctx, http.MethodDelete, "/api/v1/files/"+url.PathEscape(id), nil, "", http.StatusNoContent)
	return err
}

// listUploads reports resumable upload sessions, including abandoned ones.
func (c xionClient) listUploads(ctx context.Context) ([]byte, error) {
	return c.request(ctx, http.MethodGet, "/api/v1/uploads", nil, "", http.StatusOK)
}

// sweepUploads expires sessions whose TTL has elapsed.
func (c xionClient) sweepUploads(ctx context.Context) ([]byte, error) {
	return c.request(ctx, http.MethodDelete, "/api/v1/uploads", nil, "", http.StatusOK)
}

// listTrash reports trashed objects.
func (c xionClient) listTrash(ctx context.Context, limit, offset int) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/trash?limit=%d&offset=%d", limit, offset)
	return c.request(ctx, http.MethodGet, path, nil, "", http.StatusOK)
}

// restore brings a trashed object back with its original file id.
func (c xionClient) restore(ctx context.Context, id string) ([]byte, error) {
	return c.request(ctx, http.MethodPost, "/api/v1/files/"+url.PathEscape(id)+"/restore", nil, "", http.StatusOK)
}

// purgeTrash permanently removes trashed objects older than the cutoff. An
// empty cutoff removes everything.
func (c xionClient) purgeTrash(ctx context.Context, olderThan string, all bool) ([]byte, error) {
	payload := map[string]any{"all": all}
	if olderThan != "" {
		payload["older_than"] = olderThan
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode purge request: %w", err)
	}
	return c.request(ctx, http.MethodPost, "/api/v1/trash/purge", bytes.NewReader(body), "application/json", http.StatusOK)
}

func newXionHTTPClient(cfg config) xionClient {
	return xionClient{config: cfg, http: &http.Client{Timeout: cfg.timeout}}
}

func trimJSONSpace(value []byte) []byte {
	return bytes.TrimSpace(value)
}

func isJSON(value []byte) bool {
	return len(trimJSONSpace(value)) > 0 && strings.HasPrefix(string(trimJSONSpace(value)), "{")
}
