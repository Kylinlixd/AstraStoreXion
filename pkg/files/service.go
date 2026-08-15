package files

import (
	"context"
	"fmt"
	"io"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (File, error) {
	return s.store.Put(ctx, input)
}

func (s *Service) Download(ctx context.Context, id string) (File, io.ReadCloser, error) {
	return s.store.Open(ctx, id)
}

func (s *Service) Status(ctx context.Context, id string) (File, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]File, error) {
	return s.store.List(ctx, limit, offset)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

func (s *Service) Ready(ctx context.Context) error {
	return s.store.Ready(ctx)
}

func (s *Service) Capacity(ctx context.Context) (Capacity, error) {
	provider, ok := s.store.(interface {
		Capacity(context.Context) (Capacity, error)
	})
	if !ok {
		return Capacity{}, fmt.Errorf("storage capacity is unavailable")
	}
	return provider.Capacity(ctx)
}

func (s *Service) StartUpload(ctx context.Context, input MultipartStartInput) (UploadSession, error) {
	provider, ok := s.store.(interface {
		StartUpload(context.Context, MultipartStartInput) (UploadSession, error)
	})
	if !ok {
		return UploadSession{}, fmt.Errorf("resumable uploads are unavailable")
	}
	return provider.StartUpload(ctx, input)
}

func (s *Service) GetUpload(ctx context.Context, id string) (UploadSession, error) {
	provider, ok := s.store.(interface {
		GetUpload(context.Context, string) (UploadSession, error)
	})
	if !ok {
		return UploadSession{}, fmt.Errorf("resumable uploads are unavailable")
	}
	return provider.GetUpload(ctx, id)
}

func (s *Service) AppendUpload(ctx context.Context, id string, offset, chunkSize int64, reader io.Reader, checksum string) (UploadSession, error) {
	provider, ok := s.store.(interface {
		AppendUpload(context.Context, string, int64, int64, io.Reader, string) (UploadSession, error)
	})
	if !ok {
		return UploadSession{}, fmt.Errorf("resumable uploads are unavailable")
	}
	return provider.AppendUpload(ctx, id, offset, chunkSize, reader, checksum)
}

func (s *Service) CompleteUpload(ctx context.Context, id string) (File, error) {
	provider, ok := s.store.(interface {
		CompleteUpload(context.Context, string) (File, error)
	})
	if !ok {
		return File{}, fmt.Errorf("resumable uploads are unavailable")
	}
	return provider.CompleteUpload(ctx, id)
}

func (s *Service) AbortUpload(ctx context.Context, id string) error {
	provider, ok := s.store.(interface {
		AbortUpload(context.Context, string) error
	})
	if !ok {
		return fmt.Errorf("resumable uploads are unavailable")
	}
	return provider.AbortUpload(ctx, id)
}

func (s *Service) ListTrash(ctx context.Context, limit, offset int) ([]File, error) {
	provider, ok := s.store.(interface {
		ListTrash(context.Context, int, int) ([]File, error)
	})
	if !ok {
		return nil, fmt.Errorf("trash is unavailable")
	}
	return provider.ListTrash(ctx, limit, offset)
}

func (s *Service) Restore(ctx context.Context, id string) (File, error) {
	provider, ok := s.store.(interface {
		Restore(context.Context, string) (File, error)
	})
	if !ok {
		return File{}, fmt.Errorf("trash is unavailable")
	}
	return provider.Restore(ctx, id)
}

func (s *Service) Quota(ctx context.Context, owner string) (Quota, error) {
	provider, ok := s.store.(interface {
		Quota(context.Context, string) (Quota, error)
	})
	if !ok {
		return Quota{}, fmt.Errorf("owner quota is unavailable")
	}
	return provider.Quota(ctx, owner)
}
