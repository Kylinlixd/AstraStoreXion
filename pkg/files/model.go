package files

import (
	"errors"
	"io"
	"time"
)

var (
	ErrInvalidID       = errors.New("invalid file id")
	ErrInvalidUpload   = errors.New("invalid upload")
	ErrNotFound        = errors.New("file not found")
	ErrCorruptMetadata = errors.New("corrupt file metadata")
)

const StatusAvailable = "available"

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
