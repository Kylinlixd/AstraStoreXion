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
