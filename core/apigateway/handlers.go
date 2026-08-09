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
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
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
	fileRouter.HandleFunc("/{id}/status", g.status).Methods(http.MethodGet)
	fileRouter.HandleFunc("/{id}", g.download).Methods(http.MethodGet)
	fileRouter.HandleFunc("/{id}", g.delete).Methods(http.MethodDelete)
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
	case errors.Is(err, files.ErrNotFound):
		writeError(writer, http.StatusNotFound, "not_found", "file does not exist")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(writer, http.StatusRequestTimeout, "request_timeout", "request was cancelled or timed out")
	default:
		log.Printf("file service error: %v", err)
		writeError(writer, http.StatusInternalServerError, "internal_error", "file operation failed")
	}
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, apiErrorResponse{Error: apiError{Code: code, Message: message}})
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

func ensureFileService() *files.Service {
	if fileService == nil {
		initFileService("data/files")
	}
	return fileService
}

func parsePositiveInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
