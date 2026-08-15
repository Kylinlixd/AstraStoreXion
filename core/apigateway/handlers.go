package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/gorilla/mux"
)

var errFileTooLarge = errors.New("file exceeds configured upload limit")

type fileService interface {
	Upload(context.Context, files.UploadInput) (files.File, error)
	Download(context.Context, string) (files.File, io.ReadCloser, error)
	Status(context.Context, string) (files.File, error)
	List(context.Context, int, int) ([]files.File, error)
	Delete(context.Context, string) error
	Ready(context.Context) error
	Capacity(context.Context) (files.Capacity, error)
}

type multipartService interface {
	StartUpload(context.Context, files.MultipartStartInput) (files.UploadSession, error)
	GetUpload(context.Context, string) (files.UploadSession, error)
	AppendUpload(context.Context, string, int64, int64, io.Reader, string) (files.UploadSession, error)
	CompleteUpload(context.Context, string) (files.File, error)
	AbortUpload(context.Context, string) error
}

type trashService interface {
	ListTrash(context.Context, int, int) ([]files.File, error)
	Restore(context.Context, string) (files.File, error)
}

type gateway struct {
	service        fileService
	token          string
	maxUploadBytes int64
}

type fileListResponse struct {
	Count   int          `json:"count"`
	Results []files.File `json:"results"`
}

type statusResponse struct {
	Status string `json:"status"`
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type uploadStartRequest struct {
	Filename    string            `json:"filename"`
	ContentType string            `json:"content_type"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	Metadata    map[string]string `json:"metadata"`
}

func newRouter(g gateway) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/health", g.health).Methods(http.MethodGet)
	router.HandleFunc("/healthz", g.health).Methods(http.MethodGet)
	router.HandleFunc("/ready", g.ready).Methods(http.MethodGet)
	router.HandleFunc("/readyz", g.ready).Methods(http.MethodGet)

	fileRouter := router.PathPrefix("/api/v1/files").Subrouter()
	fileRouter.Use(g.authenticate)
	fileRouter.HandleFunc("", g.upload).Methods(http.MethodPost)
	fileRouter.HandleFunc("", g.list).Methods(http.MethodGet)
	fileRouter.HandleFunc("/capacity", g.capacity).Methods(http.MethodGet)
	fileRouter.HandleFunc("/{id}/restore", g.restore).Methods(http.MethodPost)
	fileRouter.HandleFunc("/{id}/status", g.status).Methods(http.MethodGet)
	fileRouter.HandleFunc("/{id}", g.download).Methods(http.MethodGet)
	fileRouter.HandleFunc("/{id}", g.delete).Methods(http.MethodDelete)

	trashRouter := router.PathPrefix("/api/v1/trash").Subrouter()
	trashRouter.Use(g.authenticate)
	trashRouter.HandleFunc("", g.listTrash).Methods(http.MethodGet)

	uploadRouter := router.PathPrefix("/api/v1/uploads").Subrouter()
	uploadRouter.Use(g.authenticate)
	uploadRouter.HandleFunc("", g.startUpload).Methods(http.MethodPost)
	uploadRouter.HandleFunc("/{id}/complete", g.completeUpload).Methods(http.MethodPost)
	uploadRouter.HandleFunc("/{id}", g.getUpload).Methods(http.MethodGet)
	uploadRouter.HandleFunc("/{id}", g.appendUpload).Methods(http.MethodPut)
	uploadRouter.HandleFunc("/{id}", g.abortUpload).Methods(http.MethodDelete)
	return router
}

func (g gateway) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		const prefix = "Bearer "
		header := request.Header.Get("Authorization")
		provided := ""
		if strings.HasPrefix(header, prefix) {
			provided = strings.TrimPrefix(header, prefix)
		}
		if len(provided) != len(g.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(g.token)) != 1 {
			writeError(writer, http.StatusUnauthorized, "unauthorized", "missing or invalid service token")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (g gateway) upload(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, g.maxUploadBytes+(1<<20))
	multipartReader, err := request.MultipartReader()
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_multipart", "request must contain a multipart file")
		return
	}

	metadata := map[string]string{}
	var created files.File
	fileSeen := false
	cleanupCreated := func() {
		if created.ID == "" {
			return
		}
		if err := g.service.Delete(request.Context(), created.ID); err != nil {
			log.Printf("clean up rejected upload %s: %v", created.ID, err)
		}
	}

	for {
		part, err := multipartReader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			cleanupCreated()
			writeError(writer, http.StatusRequestEntityTooLarge, "file_too_large", errFileTooLarge.Error())
			return
		}
		if err != nil {
			cleanupCreated()
			writeError(writer, http.StatusBadRequest, "invalid_multipart", "request contains invalid multipart data")
			return
		}
		if fileSeen {
			_ = part.Close()
			cleanupCreated()
			writeError(writer, http.StatusBadRequest, "invalid_multipart", "multipart metadata must precede the file")
			return
		}

		switch part.FormName() {
		case "metadata":
			encoded, err := readMultipartField(part, 64<<10)
			_ = part.Close()
			if err != nil || (encoded != "" && json.Unmarshal([]byte(encoded), &metadata) != nil) {
				writeError(writer, http.StatusBadRequest, "invalid_metadata", "metadata must be a JSON object with string values")
				return
			}
		case "file":
			if part.FileName() == "" {
				_ = part.Close()
				writeError(writer, http.StatusBadRequest, "file_required", "multipart field 'file' is required")
				return
			}
			fileSeen = true
			created, err = g.service.Upload(request.Context(), files.UploadInput{
				Name:        part.FileName(),
				ContentType: part.Header.Get("Content-Type"),
				Metadata:    metadata,
				Reader:      &uploadLimitReader{reader: part, remaining: g.maxUploadBytes},
			})
			closeErr := part.Close()
			if errors.Is(err, errFileTooLarge) {
				writeError(writer, http.StatusRequestEntityTooLarge, "file_too_large", errFileTooLarge.Error())
				return
			}
			if err != nil {
				handleServiceError(writer, err)
				return
			}
			if closeErr != nil {
				cleanupCreated()
				writeError(writer, http.StatusBadRequest, "invalid_multipart", "request contains invalid multipart data")
				return
			}
		default:
			_ = part.Close()
			writeError(writer, http.StatusBadRequest, "invalid_multipart", "request contains an unsupported multipart field")
			return
		}
	}
	if !fileSeen {
		writeError(writer, http.StatusBadRequest, "file_required", "multipart field 'file' is required")
		return
	}
	writeJSON(writer, http.StatusCreated, created)
}

func readMultipartField(reader io.Reader, limit int64) (string, error) {
	value, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(value)) > limit {
		return "", errors.New("multipart field is too large")
	}
	return string(value), nil
}

func (g gateway) download(writer http.ResponseWriter, request *http.Request) {
	stored, body, err := g.service.Download(request.Context(), mux.Vars(request)["id"])
	if err != nil {
		handleServiceError(writer, err)
		return
	}
	defer body.Close()
	writer.Header().Set("Content-Type", stored.ContentType)
	writer.Header().Set("Content-Length", strconv.FormatInt(stored.Size, 10))
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": stored.Name})
	writer.Header().Set("Content-Disposition", disposition)
	writer.Header().Set("ETag", `"sha256-`+stored.Checksum+`"`)
	writer.WriteHeader(http.StatusOK)
	if _, err := io.Copy(writer, body); err != nil {
		log.Printf("stream file %s: %v", stored.ID, err)
	}
}

func (g gateway) status(writer http.ResponseWriter, request *http.Request) {
	stored, err := g.service.Status(request.Context(), mux.Vars(request)["id"])
	if err != nil {
		handleServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, stored)
}

func (g gateway) list(writer http.ResponseWriter, request *http.Request) {
	limit, err := queryInteger(request, "limit", 100)
	if err != nil || limit < 1 || limit > 1000 {
		writeError(writer, http.StatusBadRequest, "invalid_pagination", "limit must be between 1 and 1000")
		return
	}
	offset, err := queryInteger(request, "offset", 0)
	if err != nil || offset < 0 {
		writeError(writer, http.StatusBadRequest, "invalid_pagination", "offset must be zero or greater")
		return
	}
	stored, err := g.service.List(request.Context(), limit, offset)
	if err != nil {
		handleServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, fileListResponse{Count: len(stored), Results: stored})
}

func (g gateway) capacity(writer http.ResponseWriter, request *http.Request) {
	capacity, err := g.service.Capacity(request.Context())
	if err != nil {
		handleServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, capacity)
}

func (g gateway) listTrash(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(trashService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "trash_unavailable", "trash is not enabled")
		return
	}
	limit, err := queryInteger(request, "limit", 100)
	if err != nil || limit < 1 || limit > 1000 {
		writeError(writer, http.StatusBadRequest, "invalid_pagination", "limit must be between 1 and 1000")
		return
	}
	offset, err := queryInteger(request, "offset", 0)
	if err != nil || offset < 0 {
		writeError(writer, http.StatusBadRequest, "invalid_pagination", "offset must be zero or greater")
		return
	}
	stored, err := service.ListTrash(request.Context(), limit, offset)
	if err != nil {
		handleServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, fileListResponse{Count: len(stored), Results: stored})
}

func (g gateway) restore(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(trashService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "trash_unavailable", "trash is not enabled")
		return
	}
	stored, err := service.Restore(request.Context(), mux.Vars(request)["id"])
	if err != nil {
		handleServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, stored)
}

func (g gateway) startUpload(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(multipartService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "resumable_uploads_unavailable", "resumable uploads are not enabled")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	var payload uploadStartRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_upload", "upload session must be valid JSON")
		return
	}
	if payload.Size <= 0 {
		writeError(writer, http.StatusBadRequest, "invalid_upload", "size must be positive")
		return
	}
	if payload.Size > g.maxUploadBytes {
		writeError(writer, http.StatusRequestEntityTooLarge, "file_too_large", errFileTooLarge.Error())
		return
	}
	session, err := service.StartUpload(request.Context(), files.MultipartStartInput{
		Name: payload.Filename, ContentType: payload.ContentType, Size: payload.Size,
		Checksum: payload.Checksum, Metadata: payload.Metadata,
	})
	if err != nil {
		handleUploadError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, session)
}

func (g gateway) getUpload(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(multipartService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "resumable_uploads_unavailable", "resumable uploads are not enabled")
		return
	}
	session, err := service.GetUpload(request.Context(), mux.Vars(request)["id"])
	if err != nil {
		handleUploadError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, session)
}

func (g gateway) appendUpload(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(multipartService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "resumable_uploads_unavailable", "resumable uploads are not enabled")
		return
	}
	offset, end, total, err := parseContentRange(request.Header.Get("Content-Range"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_content_range", err.Error())
		return
	}
	id := mux.Vars(request)["id"]
	session, err := service.GetUpload(request.Context(), id)
	if err != nil {
		handleUploadError(writer, err)
		return
	}
	if total != session.Size {
		writeError(writer, http.StatusBadRequest, "invalid_content_range", "content range total does not match upload size")
		return
	}
	chunkSize := end - offset + 1
	if request.ContentLength != chunkSize {
		writeError(writer, http.StatusBadRequest, "invalid_content_length", "content length must match content range")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, chunkSize)
	updated, err := service.AppendUpload(request.Context(), id, offset, chunkSize, request.Body, request.Header.Get("X-Chunk-Checksum"))
	if err != nil {
		handleUploadError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, updated)
}

func (g gateway) completeUpload(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(multipartService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "resumable_uploads_unavailable", "resumable uploads are not enabled")
		return
	}
	file, err := service.CompleteUpload(request.Context(), mux.Vars(request)["id"])
	if err != nil {
		handleUploadError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, file)
}

func (g gateway) abortUpload(writer http.ResponseWriter, request *http.Request) {
	service, ok := g.service.(multipartService)
	if !ok {
		writeError(writer, http.StatusNotImplemented, "resumable_uploads_unavailable", "resumable uploads are not enabled")
		return
	}
	if err := service.AbortUpload(request.Context(), mux.Vars(request)["id"]); err != nil {
		handleUploadError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (g gateway) delete(writer http.ResponseWriter, request *http.Request) {
	if err := g.service.Delete(request.Context(), mux.Vars(request)["id"]); err != nil {
		handleServiceError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (g gateway) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, statusResponse{Status: "ok"})
}

func (g gateway) ready(writer http.ResponseWriter, request *http.Request) {
	if err := g.service.Ready(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "not_ready", "storage is not ready")
		return
	}
	writeJSON(writer, http.StatusOK, statusResponse{Status: "ready"})
}

func handleServiceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, files.ErrInvalidID), errors.Is(err, files.ErrInvalidUpload):
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, files.ErrRestoreConflict):
		writeError(writer, http.StatusConflict, "file_restore_conflict", "an active file already uses this file id")
	case errors.Is(err, files.ErrStoragePaused):
		writeErrorWithRetry(writer, http.StatusInsufficientStorage, "storage_paused", "存储空间已达到安全阈值，暂时停止上传；已有文件仍可读取，释放空间后会自动恢复。", true)
	case errors.Is(err, files.ErrNotFound):
		writeError(writer, http.StatusNotFound, "not_found", "file does not exist")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(writer, http.StatusRequestTimeout, "request_timeout", "request was cancelled or timed out")
	default:
		log.Printf("file service error: %v", err)
		writeError(writer, http.StatusInternalServerError, "internal_error", "file operation failed")
	}
}

func handleUploadError(writer http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesError), errors.Is(err, files.ErrUploadTooLarge):
		writeError(writer, http.StatusRequestEntityTooLarge, "file_too_large", errFileTooLarge.Error())
	case errors.Is(err, files.ErrInvalidID), errors.Is(err, files.ErrInvalidUpload), errors.Is(err, files.ErrUploadChunkSize):
		writeError(writer, http.StatusBadRequest, "invalid_upload", err.Error())
	case errors.Is(err, files.ErrUploadNotFound):
		writeError(writer, http.StatusNotFound, "upload_not_found", "upload session does not exist")
	case errors.Is(err, files.ErrUploadOffsetConflict):
		writeError(writer, http.StatusConflict, "upload_offset_conflict", err.Error())
	case errors.Is(err, files.ErrUploadIncomplete):
		writeError(writer, http.StatusConflict, "upload_incomplete", "upload session has not received all bytes")
	case errors.Is(err, files.ErrUploadChecksumMismatch):
		writeError(writer, http.StatusUnprocessableEntity, "upload_checksum_mismatch", "upload checksum does not match")
	case errors.Is(err, files.ErrStoragePaused):
		writeErrorWithRetry(writer, http.StatusInsufficientStorage, "storage_paused", "存储空间已达到安全阈值，暂时停止上传；释放空间后会自动恢复。", true)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(writer, http.StatusRequestTimeout, "request_timeout", "request was cancelled or timed out")
	default:
		log.Printf("resumable upload error: %v", err)
		writeError(writer, http.StatusInternalServerError, "internal_error", "upload operation failed")
	}
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeErrorWithRetry(writer, status, code, message, false)
}

func writeErrorWithRetry(writer http.ResponseWriter, status int, code, message string, retryable bool) {
	writeJSON(writer, status, apiErrorResponse{Error: apiError{Code: code, Message: message, Retryable: retryable}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func queryInteger(request *http.Request, name string, defaultValue int) (int, error) {
	value := request.URL.Query().Get(name)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func parseContentRange(value string) (int64, int64, int64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, fmt.Errorf("content range must use bytes start-end/total")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 || parts[1] == "*" {
		return 0, 0, 0, fmt.Errorf("content range must include a total size")
	}
	rangeParts := strings.Split(parts[0], "-")
	if len(rangeParts) != 2 {
		return 0, 0, 0, fmt.Errorf("content range must include start and end")
	}
	start, startErr := strconv.ParseInt(rangeParts[0], 10, 64)
	end, endErr := strconv.ParseInt(rangeParts[1], 10, 64)
	total, totalErr := strconv.ParseInt(parts[1], 10, 64)
	if startErr != nil || endErr != nil || totalErr != nil || start < 0 || end < start || total <= end {
		return 0, 0, 0, fmt.Errorf("content range values are invalid")
	}
	return start, end, total, nil
}

type uploadLimitReader struct {
	reader    io.Reader
	remaining int64
}

func (r *uploadLimitReader) Read(buffer []byte) (int, error) {
	if r.remaining < 0 {
		return 0, errFileTooLarge
	}
	maximumRead := r.remaining + 1
	if int64(len(buffer)) > maximumRead {
		buffer = buffer[:maximumRead]
	}
	read, err := r.reader.Read(buffer)
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		err = errFileTooLarge
	}
	r.remaining -= int64(read)
	if r.remaining < 0 {
		return read, errFileTooLarge
	}
	return read, err
}
