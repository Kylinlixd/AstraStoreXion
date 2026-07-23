package files

import (
	"errors"
	"io"
	"time"
)

var ErrNotFound = errors.New("文件不存在")

type File struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
	OwnerID     string    `json:"owner_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	DownloadURL string    `json:"download_url,omitempty"`
}

type UploadInput struct {
	Name        string
	ContentType string
	OwnerID     string
	Reader      io.Reader
}

type DownloadOutput struct {
	File    File
	Content io.ReadCloser
}
