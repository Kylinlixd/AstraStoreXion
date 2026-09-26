package files

import (
	"context"
	"fmt"
	"io"
	"time"
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

// Count returns the number of active objects.
func (s *Service) Count(ctx context.Context) (int, error) {
	provider, ok := s.store.(interface {
		Count(context.Context) (int, error)
	})
	if !ok {
		return 0, fmt.Errorf("object count is unavailable")
	}
	return provider.Count(ctx)
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

// Recover repairs crash leftovers so a restarted process can become ready
// without manual filesystem surgery.
func (s *Service) Recover(ctx context.Context) (RecoveryReport, error) {
	provider, ok := s.store.(interface {
		Recover(context.Context) (RecoveryReport, error)
	})
	if !ok {
		return RecoveryReport{}, nil
	}
	return provider.Recover(ctx)
}

// SweepUploads expires abandoned resumable upload sessions.
func (s *Service) SweepUploads(ctx context.Context) (int, error) {
	provider, ok := s.store.(interface {
		SweepUploads(context.Context) (int, error)
	})
	if !ok {
		return 0, nil
	}
	return provider.SweepUploads(ctx)
}

// ListUploads lists persisted resumable upload sessions.
func (s *Service) ListUploads(ctx context.Context) ([]UploadSession, error) {
	provider, ok := s.store.(interface {
		ListUploads(context.Context) ([]UploadSession, error)
	})
	if !ok {
		return nil, fmt.Errorf("resumable uploads are unavailable")
	}
	return provider.ListUploads(ctx)
}

// PurgeTrash permanently removes trash entries deleted at or before olderThan.
func (s *Service) PurgeTrash(ctx context.Context, olderThan time.Time) (int, error) {
	provider, ok := s.store.(interface {
		PurgeTrash(context.Context, time.Time) (int, error)
	})
	if !ok {
		return 0, fmt.Errorf("trash is unavailable")
	}
	return provider.PurgeTrash(ctx, olderThan)
}
