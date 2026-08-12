package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var validFileID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Store interface {
	Put(context.Context, UploadInput) (File, error)
	Open(context.Context, string) (File, io.ReadCloser, error)
	Get(context.Context, string) (File, error)
	List(context.Context, int, int) ([]File, error)
	Delete(context.Context, string) error
	Ready(context.Context) error
}

type DiskStore struct {
	root           string
	objectsDir     string
	metadataDir    string
	tmpDir         string
	pauseAtPercent int
	statFS         func(string, *unix.Statfs_t) error
	mu             sync.RWMutex
}

func NewDiskStore(root string) (*DiskStore, error) {
	return NewDiskStoreWithPause(root, 0)
}

func NewDiskStoreWithPause(root string, pauseAtPercent int) (*DiskStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: data directory is empty", ErrInvalidUpload)
	}
	if pauseAtPercent < 0 || pauseAtPercent > 100 {
		return nil, fmt.Errorf("%w: pause threshold must be between 0 and 100", ErrInvalidUpload)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	store := &DiskStore{
		root:           absoluteRoot,
		objectsDir:     filepath.Join(absoluteRoot, "objects"),
		metadataDir:    filepath.Join(absoluteRoot, "metadata"),
		tmpDir:         filepath.Join(absoluteRoot, "tmp"),
		pauseAtPercent: pauseAtPercent,
		statFS:         unix.Statfs,
	}
	for _, directory := range []string{store.root, store.objectsDir, store.metadataDir, store.tmpDir} {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return nil, fmt.Errorf("create storage directory %s: %w", directory, err)
		}
	}
	return store, nil
}

func (s *DiskStore) Put(ctx context.Context, input UploadInput) (File, error) {
	if input.Reader == nil {
		return File{}, fmt.Errorf("%w: file body is required", ErrInvalidUpload)
	}
	if err := ctx.Err(); err != nil {
		return File{}, err
	}

	fileID := uuid.NewString()
	temporaryPath := filepath.Join(s.tmpDir, fileID+".part")
	objectPath := filepath.Join(s.objectsDir, fileID)
	manifestPath := filepath.Join(s.metadataDir, fileID+".json")

	s.mu.Lock()
	defer s.mu.Unlock()
	paused, err := s.writesPausedUnlocked()
	if err != nil {
		return File{}, err
	}
	if paused {
		return File{}, ErrStoragePaused
	}

	temporary, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return File{}, fmt.Errorf("create temporary object: %w", err)
	}
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(temporary, hasher), &contextReader{ctx: ctx, reader: input.Reader})
	if err != nil {
		return File{}, fmt.Errorf("write object: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return File{}, fmt.Errorf("sync object: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return File{}, fmt.Errorf("close object: %w", err)
	}
	if err := os.Rename(temporaryPath, objectPath); err != nil {
		return File{}, fmt.Errorf("commit object: %w", err)
	}
	removeTemporary = false
	if err := syncDirectory(s.objectsDir); err != nil {
		_ = os.Remove(objectPath)
		return File{}, fmt.Errorf("sync object directory: %w", err)
	}

	name := cleanDisplayName(input.Name)
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	stored := File{
		ID:          fileID,
		Name:        name,
		ContentType: contentType,
		Size:        size,
		Checksum:    hex.EncodeToString(hasher.Sum(nil)),
		Status:      StatusAvailable,
		CreatedAt:   time.Now().UTC(),
		Metadata:    cloneFile(File{Metadata: input.Metadata}).Metadata,
	}
	if err := writeManifest(manifestPath, stored); err != nil {
		_ = os.Remove(objectPath)
		_ = syncDirectory(s.objectsDir)
		return File{}, err
	}
	if err := syncDirectory(s.metadataDir); err != nil {
		_ = os.Remove(manifestPath)
		_ = os.Remove(objectPath)
		_ = syncDirectory(s.metadataDir)
		_ = syncDirectory(s.objectsDir)
		return File{}, fmt.Errorf("sync metadata directory: %w", err)
	}
	return cloneFile(stored), nil
}

func (s *DiskStore) Capacity(ctx context.Context) (Capacity, error) {
	if err := ctx.Err(); err != nil {
		return Capacity{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capacityUnlocked()
}

func (s *DiskStore) capacityUnlocked() (Capacity, error) {
	var stat unix.Statfs_t
	statFS := s.statFS
	if statFS == nil {
		statFS = unix.Statfs
	}
	if err := statFS(s.root, &stat); err != nil {
		return Capacity{}, fmt.Errorf("stat storage filesystem: %w", err)
	}
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	used := total - uint64(stat.Bfree)*uint64(stat.Bsize)
	usedPercent := 0.0
	if total > 0 {
		usedPercent = float64(used) * 100 / float64(total)
	}
	objectCount, objectBytes, err := directoryUsage(s.objectsDir)
	if err != nil {
		return Capacity{}, err
	}
	metadataCount, _, err := directoryUsage(s.metadataDir)
	if err != nil {
		return Capacity{}, err
	}
	paused := s.pauseAtPercent > 0 && usedPercent >= float64(s.pauseAtPercent)
	return Capacity{
		TotalBytes: total, UsedBytes: used, AvailableBytes: available,
		UsedPercent: usedPercent, ObjectCount: objectCount, ObjectBytes: objectBytes,
		MetadataCount: metadataCount, PauseAtPercent: s.pauseAtPercent, WritesPaused: paused,
	}, nil
}

func (s *DiskStore) writesPausedUnlocked() (bool, error) {
	if s.pauseAtPercent == 0 {
		return false, nil
	}
	capacity, err := s.capacityUnlocked()
	if err != nil {
		return false, err
	}
	return capacity.WritesPaused, nil
}

func directoryUsage(path string) (int, int64, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0, fmt.Errorf("read storage directory: %w", err)
	}
	count := 0
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, 0, fmt.Errorf("stat storage entry: %w", err)
		}
		count++
		total += info.Size()
	}
	return count, total, nil
}

func (s *DiskStore) Open(ctx context.Context, id string) (File, io.ReadCloser, error) {
	if err := validateID(id); err != nil {
		return File{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return File{}, nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	stored, err := s.getUnlocked(id)
	if err != nil {
		return File{}, nil, err
	}
	body, err := os.Open(filepath.Join(s.objectsDir, id))
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return File{}, nil, fmt.Errorf("open object: %w", err)
	}
	return cloneFile(stored), body, nil
}

func (s *DiskStore) Get(ctx context.Context, id string) (File, error) {
	if err := validateID(id); err != nil {
		return File{}, err
	}
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getUnlocked(id)
}

func (s *DiskStore) getUnlocked(id string) (File, error) {
	manifestPath := filepath.Join(s.metadataDir, id+".json")
	data, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return File{}, fmt.Errorf("read metadata: %w", err)
	}
	var stored File
	if err := json.Unmarshal(data, &stored); err != nil {
		return File{}, fmt.Errorf("%w: %s: %v", ErrCorruptMetadata, id, err)
	}
	if stored.ID != id || stored.Status != StatusAvailable || stored.Size < 0 || stored.Checksum == "" {
		return File{}, fmt.Errorf("%w: %s", ErrCorruptMetadata, id)
	}
	return cloneFile(stored), nil
}

func (s *DiskStore) List(ctx context.Context, limit, offset int) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.metadataDir)
	if err != nil {
		return nil, fmt.Errorf("list metadata: %w", err)
	}
	stored := make([]File, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateID(id); err != nil {
			return nil, fmt.Errorf("%w: unexpected manifest %s", ErrCorruptMetadata, entry.Name())
		}
		file, err := s.getUnlocked(id)
		if err != nil {
			return nil, err
		}
		stored = append(stored, file)
	}
	sort.Slice(stored, func(i, j int) bool {
		if stored[i].CreatedAt.Equal(stored[j].CreatedAt) {
			return stored[i].ID < stored[j].ID
		}
		return stored[i].CreatedAt.After(stored[j].CreatedAt)
	})
	if offset >= len(stored) {
		return []File{}, nil
	}
	end := offset + limit
	if end > len(stored) {
		end = len(stored)
	}
	result := make([]File, 0, end-offset)
	for _, file := range stored[offset:end] {
		result = append(result, cloneFile(file))
	}
	return result, nil
}

func (s *DiskStore) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, target := range []string{filepath.Join(s.objectsDir, id), filepath.Join(s.metadataDir, id+".json")} {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete file data: %w", err)
		}
	}
	if err := syncDirectory(s.objectsDir); err != nil {
		return fmt.Errorf("sync object directory: %w", err)
	}
	if err := syncDirectory(s.metadataDir); err != nil {
		return fmt.Errorf("sync metadata directory: %w", err)
	}
	return nil
}

func (s *DiskStore) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	probe, err := os.CreateTemp(s.tmpDir, ".ready-*")
	if err != nil {
		return fmt.Errorf("storage is not writable: %w", err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("close readiness probe: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("remove readiness probe: %w", err)
	}
	entries, err := os.ReadDir(s.metadataDir)
	if err != nil {
		return fmt.Errorf("read metadata directory: %w", err)
	}
	manifestIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("%w: unexpected metadata artifact %s", ErrCorruptMetadata, entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateID(id); err != nil {
			return fmt.Errorf("%w: unexpected manifest %s", ErrCorruptMetadata, entry.Name())
		}
		manifestIDs[id] = struct{}{}
		if _, err := s.getUnlocked(id); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(s.objectsDir, id)); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: object %s is missing", ErrCorruptMetadata, id)
		} else if err != nil {
			return fmt.Errorf("stat object: %w", err)
		}
	}
	objects, err := os.ReadDir(s.objectsDir)
	if err != nil {
		return fmt.Errorf("read object directory: %w", err)
	}
	for _, entry := range objects {
		if entry.IsDir() || validateID(entry.Name()) != nil {
			return fmt.Errorf("%w: unexpected object artifact %s", ErrCorruptMetadata, entry.Name())
		}
		if _, ok := manifestIDs[entry.Name()]; !ok {
			return fmt.Errorf("%w: object %s has no manifest", ErrCorruptMetadata, entry.Name())
		}
	}
	partials, err := os.ReadDir(s.tmpDir)
	if err != nil {
		return fmt.Errorf("read temporary directory: %w", err)
	}
	if len(partials) > 0 {
		return fmt.Errorf("%w: unexpected temporary artifact %s", ErrCorruptMetadata, partials[0].Name())
	}
	return nil
}

func validateID(id string) error {
	if !validFileID.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return nil
}

func cleanDisplayName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." {
		return "file"
	}
	return name
}

func writeManifest(path string, stored File) error {
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	temporaryPath := path + ".tmp"
	temporary, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("create metadata: %w", err)
	}
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close metadata: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit metadata: %w", err)
	}
	removeTemporary = false
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
