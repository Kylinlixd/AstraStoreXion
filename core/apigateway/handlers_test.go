package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestFileHandlersPersistUploadedContent(t *testing.T) {
	initAuthSystem()
	initFileService(t.TempDir())

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/auth/login", login).Methods("POST")
	router.HandleFunc("/api/v1/files", uploadFile).Methods("POST")
	router.HandleFunc("/api/v1/files", listFiles).Methods("GET")
	router.HandleFunc("/api/v1/files/{id}", downloadFile).Methods("GET")
	router.HandleFunc("/api/v1/files/{id}", deleteFile).Methods("DELETE")
	router.HandleFunc("/api/v1/files/{id}/status", getFileStatus).Methods("GET")

	token := loginForTest(t, router)
	fileID := uploadForTest(t, router, token, "hello.txt", []byte("hello world"))

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+fileID+"/status", nil)
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusResp := httptest.NewRecorder()
	router.ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", statusResp.Code, statusResp.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list code = %d, body = %s", listResp.Code, listResp.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+fileID, nil)
	downloadReq.Header.Set("Authorization", "Bearer "+token)
	downloadResp := httptest.NewRecorder()
	router.ServeHTTP(downloadResp, downloadReq)
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("download code = %d, body = %s", downloadResp.Code, downloadResp.Body.String())
	}
	if downloadResp.Body.String() != "hello world" {
		t.Fatalf("download body = %q, want hello world", downloadResp.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+fileID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteResp := httptest.NewRecorder()
	router.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete code = %d, body = %s", deleteResp.Code, deleteResp.Body.String())
	}

	downloadAgainResp := httptest.NewRecorder()
	router.ServeHTTP(downloadAgainResp, downloadReq)
	if downloadAgainResp.Code != http.StatusNotFound {
		t.Fatalf("download after delete code = %d, want %d", downloadAgainResp.Code, http.StatusNotFound)
	}
}

func loginForTest(t *testing.T, router http.Handler) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("login code = %d, body = %s", resp.Code, resp.Body.String())
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response error = %v", err)
	}
	return body.Token
}

func uploadForTest(t *testing.T, router http.Handler, token, name string, content []byte) string {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(content)); err != nil {
		t.Fatalf("copy upload body error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("upload code = %d, body = %s", resp.Code, resp.Body.String())
	}

	var response struct {
		FileID string `json:"file_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode upload response error = %v", err)
	}
	return response.FileID
}
