package files

import (
	"errors"
	"io"
	"time"
)

var (
	ErrInvalidID              = errors.New("invalid file id")
	ErrInvalidUpload          = errors.New("invalid upload")
	ErrNotFound               = errors.New("file not found")
	ErrCorruptMetadata        = errors.New("corrupt file metadata")
	ErrStoragePaused          = errors.New("storage writes paused")
	ErrUploadNotFound         = errors.New("upload session not found")
	ErrUploadOffsetConflict   = errors.New("upload offset conflict")
	ErrUploadIncomplete       = errors.New("upload is incomplete")
	ErrUploadChecksumMismatch = errors.New("upload checksum mismatch")
	ErrUploadTooLarge         = errors.New("upload exceeds declared size")
)

const StatusAvailable = "available"
const UploadStatusUploading = "uploading"
const UploadStatusCompleted = "completed"

type File struct {
	ID          string            `json:"file_id"`
	Name        string            `json:"filename"`
	ContentType string            `json:"content_type"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type UploadInput struct {
	Name        string
	ContentType string
	Metadata    map[string]string
	Reader      io.Reader
}

type MultipartStartInput struct {
	Name        string
	ContentType string
	Size        int64
	Checksum    string
	Metadata    map[string]string
}

type UploadSession struct {
	ID            string            `json:"upload_id"`
	FileID        string            `json:"file_id,omitempty"`
	Name          string            `json:"filename"`
	ContentType   string            `json:"content_type"`
	Size          int64             `json:"size"`
	Checksum      string            `json:"checksum,omitempty"`
	ReceivedBytes int64             `json:"received_bytes"`
	Status        string            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type Capacity struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	ObjectCount    int     `json:"object_count"`
	ObjectBytes    int64   `json:"object_bytes"`
	MetadataCount  int     `json:"metadata_count"`
	PauseAtPercent int     `json:"pause_at_percent"`
	WritesPaused   bool    `json:"writes_paused"`
}

func cloneFile(file File) File {
	if file.Metadata == nil {
		return file
	}
	source := file.Metadata
	file.Metadata = make(map[string]string, len(source))
	for key, value := range source {
		file.Metadata[key] = value
	}
	return file
}
