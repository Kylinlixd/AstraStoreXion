package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/astrastore/astrastore-xion/pkg/metadata"
	"github.com/astrastore/astrastore-xion/pkg/storage"
)

type IDGenerator func() string

type Option func(*Service)

type Service struct {
	store    storage.StorageNode
	metadata metadata.MetadataService
	nodeID   string
	newID    IDGenerator
}

func NewService(store storage.StorageNode, metadataService metadata.MetadataService, opts ...Option) *Service {
	service := &Service{
		store:    store,
		metadata: metadataService,
		nodeID:   "local",
		newID:    generateID,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func WithIDGenerator(generator IDGenerator) Option {
	return func(s *Service) {
		if generator != nil {
			s.newID = generator
		}
	}
}

func WithNodeID(nodeID string) Option {
	return func(s *Service) {
		if strings.TrimSpace(nodeID) != "" {
			s.nodeID = nodeID
		}
	}
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (File, error) {
	if input.Reader == nil {
		return File{}, fmt.Errorf("文件内容不能为空")
	}

	data, err := io.ReadAll(input.Reader)
	if err != nil {
		return File{}, fmt.Errorf("读取上传内容失败: %w", err)
	}

	id := s.newID()
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])

	if err := s.store.WriteChunk(ctx, id, data); err != nil {
		return File{}, err
	}

	metaInput := metadata.CreateFileMetadataInput{
		FileID:      id,
		Name:        sanitizeName(input.Name),
		ContentType: contentTypeOrDefault(input.ContentType),
		Size:        int64(len(data)),
		Checksum:    checksum,
		OwnerID:     input.OwnerID,
		Status:      metadata.FileStatusReady,
		Chunks: []metadata.ChunkInfo{{
			ChunkID: id,
			NodeID:  s.nodeID,
			Offset:  0,
			Size:    int64(len(data)),
		}},
	}
	if err := s.metadata.CreateFileMetadata(metaInput); err != nil {
		_ = s.store.DeleteChunk(ctx, id)
		return File{}, err
	}

	meta, err := s.metadata.GetMetadata(id)
	if err != nil {
		return File{}, err
	}
	return fileFromMetadata(meta), nil
}

func (s *Service) Download(ctx context.Context, id string) (DownloadOutput, error) {
	meta, err := s.metadata.GetMetadata(id)
	if err != nil {
		return DownloadOutput{}, ErrNotFound
	}
	if meta.Status != metadata.FileStatusReady || len(meta.Chunks) == 0 {
		return DownloadOutput{}, ErrNotFound
	}

	data, err := s.store.ReadChunk(ctx, meta.Chunks[0].ChunkID)
	if err != nil {
		return DownloadOutput{}, err
	}

	return DownloadOutput{
		File:    fileFromMetadata(meta),
		Content: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func (s *Service) Status(ctx context.Context, id string) (File, error) {
	meta, err := s.metadata.GetMetadata(id)
	if err != nil {
		return File{}, ErrNotFound
	}
	return fileFromMetadata(meta), nil
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]File, error) {
	metas, err := s.metadata.ListMetadata(limit, offset)
	if err != nil {
		return nil, err
	}

	files := make([]File, 0, len(metas))
	for _, meta := range metas {
		if meta.Status == metadata.FileStatusReady {
			files = append(files, fileFromMetadata(meta))
		}
	}
	return files, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	meta, err := s.metadata.GetMetadata(id)
	if err != nil {
		return ErrNotFound
	}

	for _, chunk := range meta.Chunks {
		if err := s.store.DeleteChunk(ctx, chunk.ChunkID); err != nil {
			return err
		}
	}

	if err := s.metadata.DeleteMetadata(id); err != nil {
		return ErrNotFound
	}
	return nil
}

func fileFromMetadata(meta *metadata.FileMetadata) File {
	return File{
		ID:          meta.FileID,
		Name:        meta.Name,
		ContentType: meta.ContentType,
		Size:        meta.Size,
		Checksum:    meta.Checksum,
		OwnerID:     meta.OwnerID,
		Status:      string(meta.Status),
		CreatedAt:   meta.CreatedAt,
	}
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	parts := strings.Split(name, "/")
	name = parts[len(parts)-1]
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

func contentTypeOrDefault(contentType string) string {
	if strings.TrimSpace(contentType) == "" {
		return "application/octet-stream"
	}
	return contentType
}

func generateID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "file-fallback"
	}
	return "file-" + hex.EncodeToString(b[:])
}
