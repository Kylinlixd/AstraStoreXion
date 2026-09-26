package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
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

type MultipartStore interface {
	StartUpload(context.Context, MultipartStartInput) (UploadSession, error)
	GetUpload(context.Context, string) (UploadSession, error)
	AppendUpload(context.Context, string, int64, int64, io.Reader, string) (UploadSession, error)
	CompleteUpload(context.Context, string) (File, error)
	AbortUpload(context.Context, string) error
}

type TrashStore interface {
	ListTrash(context.Context, int, int) ([]File, error)
	Restore(context.Context, string) (File, error)
}

// Options carries the optional DiskStore behaviours. The zero value keeps the
// original single-node defaults: no pause threshold, no owner quota and no
// background expiry of abandoned upload sessions.
type Options struct {
	PauseAtPercent  int
	OwnerQuotaBytes int64
	// UploadTTL expires upload sessions that stopped receiving bytes. Zero
	// disables expiry.
	UploadTTL time.Duration
	// clock is injectable so expiry can be tested without sleeping.
	clock func() time.Time
}

func (o Options) now() time.Time {
	if o.clock != nil {
		return o.clock()
	}
	return time.Now().UTC()
}

type DiskStore struct {
	root            string
	objectsDir      string
	metadataDir     string
	tmpDir          string
	uploadsDir      string
	trashDir        string
	quarantineDir   string
	pauseAtPercent  int
	ownerQuotaBytes int64
	uploadTTL       time.Duration
	clock           func() time.Time
	statFS          func(string, *unix.Statfs_t) error
	mu              sync.RWMutex
	// Per-owner byte accounting, split by record type so a reservation can
	// migrate to a real object without being double counted. The index is
	// rebuilt from disk on first use and updated incrementally afterwards.
	usage      usageIndex
	usageValid bool
	load       sync.Once
	loadErr    error
}

// usageIndex tracks the three things that occupy an owner's quota. Keeping the
// categories separate makes the transitions exact: completing an upload moves
// bytes from uploads to files; deleting moves them from files to trash;
// restoring moves them back; purging drops trash bytes entirely.
//
// Counts are tracked per record, not per owner: the map is keyed by owner, so
// len(files) is the number of owners, not the number of objects.
type usageIndex struct {
	files   map[string]usageEntry
	trash   map[string]usageEntry
	uploads map[string]usageEntry
}

// usageEntry is one owner's contribution to a category.
type usageEntry struct {
	Bytes   int64
	Records int
}

func newUsageIndex() usageIndex {
	return usageIndex{
		files:   make(map[string]usageEntry),
		trash:   make(map[string]usageEntry),
		uploads: make(map[string]usageEntry),
	}
}

// total returns the bytes charged to one owner across all three categories.
func (u usageIndex) total(owner string) int64 {
	return u.files[owner].Bytes + u.trash[owner].Bytes + u.uploads[owner].Bytes
}

func (u usageIndex) bytesOf() int64 {
	var total int64
	for _, entry := range u.files {
		total += entry.Bytes
	}
	return total
}

func (u usageIndex) activeCount() int {
	count := 0
	for _, entry := range u.files {
		count += entry.Records
	}
	return count
}

func (u usageIndex) bytesInTrash() int64 {
	var total int64
	for _, entry := range u.trash {
		total += entry.Bytes
	}
	return total
}

func (u usageIndex) trashedCount() int {
	count := 0
	for _, entry := range u.trash {
		count += entry.Records
	}
	return count
}

func (u usageIndex) bucket(kind usageKind) map[string]usageEntry {
	switch kind {
	case usageTrash:
		return u.trash
	case usageUpload:
		return u.uploads
	default:
		return u.files
	}
}

type usageKind int

const (
	usageFile usageKind = iota
	usageTrash
	usageUpload
)

func NewDiskStore(root string) (*DiskStore, error) {
	return NewDiskStoreWithOptions(root, Options{})
}

func NewDiskStoreWithPause(root string, pauseAtPercent int) (*DiskStore, error) {
	return NewDiskStoreWithOptions(root, Options{PauseAtPercent: pauseAtPercent})
}

func NewDiskStoreWithPauseAndQuota(root string, pauseAtPercent int, ownerQuotaBytes int64) (*DiskStore, error) {
	return NewDiskStoreWithOptions(root, Options{PauseAtPercent: pauseAtPercent, OwnerQuotaBytes: ownerQuotaBytes})
}

// SetUploadTTL configures expiry of abandoned upload sessions. Zero disables it.
func (s *DiskStore) SetUploadTTL(ttl time.Duration) {
	if ttl < 0 {
		ttl = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploadTTL = ttl
}

// SetClock replaces the time source. Intended for tests.
func (s *DiskStore) SetClock(clock func() time.Time) {
	if clock == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock = clock
}

func (s *DiskStore) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now().UTC()
}

func NewDiskStoreWithOptions(root string, options Options) (*DiskStore, error) {
	pauseAtPercent := options.PauseAtPercent
	ownerQuotaBytes := options.OwnerQuotaBytes
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: data directory is empty", ErrInvalidUpload)
	}
	if pauseAtPercent < 0 || pauseAtPercent > 100 {
		return nil, fmt.Errorf("%w: pause threshold must be between 0 and 100", ErrInvalidUpload)
	}
	if ownerQuotaBytes < 0 {
		return nil, fmt.Errorf("%w: owner quota must not be negative", ErrInvalidUpload)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	store := &DiskStore{
		root:            absoluteRoot,
		objectsDir:      filepath.Join(absoluteRoot, "objects"),
		metadataDir:     filepath.Join(absoluteRoot, "metadata"),
		tmpDir:          filepath.Join(absoluteRoot, "tmp"),
		uploadsDir:      filepath.Join(absoluteRoot, "uploads"),
		trashDir:        filepath.Join(absoluteRoot, "trash"),
		quarantineDir:   filepath.Join(absoluteRoot, "quarantine"),
		pauseAtPercent:  pauseAtPercent,
		ownerQuotaBytes: ownerQuotaBytes,
		uploadTTL:       options.UploadTTL,
		clock:           options.clock,
		statFS:          unix.Statfs,
	}
	for _, directory := range []string{store.root, store.objectsDir, store.metadataDir, store.tmpDir, store.uploadsDir, store.trashDir, store.quarantineDir} {
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

	fileID := strings.TrimSpace(input.ID)
	if fileID == "" {
		fileID = uuid.NewString()
	} else if err := validateID(fileID); err != nil {
		return File{}, err
	}
	temporaryPath := filepath.Join(s.tmpDir, fileID+".part")
	objectPath := filepath.Join(s.objectsDir, fileID)
	manifestPath := filepath.Join(s.metadataDir, fileID+".json")
	owner := ownerFromMetadata(input.Metadata)

	s.mu.Lock()
	defer s.mu.Unlock()
	paused, err := s.writesPausedUnlocked()
	if err != nil {
		return File{}, err
	}
	if paused {
		return File{}, ErrStoragePaused
	}
	if err := s.ensureUsageLocked(); err != nil {
		return File{}, err
	}
	if pathExists(objectPath) || pathExists(manifestPath) || pathExists(filepath.Join(s.trashDir, fileID)) {
		return File{}, ErrFileExists
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
	if err := s.enforceQuotaUnlocked(owner, size); err != nil {
		return File{}, err
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
		CreatedAt:   s.now(),
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
	s.moveUsage(owner, usageKind(-1), usageFile, size, 1)
	return cloneFile(stored), nil
}

func (s *DiskStore) StartUpload(ctx context.Context, input MultipartStartInput) (UploadSession, error) {
	if err := ctx.Err(); err != nil {
		return UploadSession{}, err
	}
	if input.Size <= 0 {
		return UploadSession{}, fmt.Errorf("%w: upload size must be positive", ErrInvalidUpload)
	}
	if input.Checksum != "" && !isSHA256(input.Checksum) {
		return UploadSession{}, fmt.Errorf("%w: checksum must be a SHA-256 hex string", ErrInvalidUpload)
	}

	sessionID := uuid.NewString()
	sessionDir := filepath.Join(s.uploadsDir, sessionID)
	dataPath := filepath.Join(sessionDir, "data.part")
	manifestPath := filepath.Join(sessionDir, "manifest.json")
	now := s.now()
	session := UploadSession{
		ID: sessionID, Name: cleanDisplayName(input.Name), ContentType: strings.TrimSpace(input.ContentType),
		Size: input.Size, Checksum: strings.ToLower(strings.TrimSpace(input.Checksum)),
		Status: UploadStatusUploading, CreatedAt: now, UpdatedAt: now,
		Metadata: cloneFile(File{Metadata: input.Metadata}).Metadata,
	}
	if session.ContentType == "" {
		session.ContentType = mime.TypeByExtension(filepath.Ext(session.Name))
	}
	if session.ContentType == "" {
		session.ContentType = "application/octet-stream"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	paused, err := s.writesPausedUnlocked()
	if err != nil {
		return UploadSession{}, err
	}
	if paused {
		return UploadSession{}, ErrStoragePaused
	}
	// Expire abandoned sessions before reserving quota, so a stalled client
	// cannot hold capacity indefinitely.
	if _, err := s.sweepUploadsUnlocked(s.now()); err != nil {
		return UploadSession{}, err
	}
	if err := s.ensureUsageLocked(); err != nil {
		return UploadSession{}, err
	}
	if err := s.enforceQuotaUnlocked(ownerFromMetadata(input.Metadata), input.Size); err != nil {
		return UploadSession{}, err
	}
	if err := os.Mkdir(sessionDir, 0o750); err != nil {
		return UploadSession{}, fmt.Errorf("create upload session: %w", err)
	}
	removeSession := true
	defer func() {
		if removeSession {
			_ = os.RemoveAll(sessionDir)
		}
	}()
	part, err := os.OpenFile(dataPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return UploadSession{}, fmt.Errorf("create upload part: %w", err)
	}
	if err := part.Close(); err != nil {
		return UploadSession{}, fmt.Errorf("close upload part: %w", err)
	}
	if err := writeUploadManifest(manifestPath, session); err != nil {
		return UploadSession{}, err
	}
	if err := syncDirectory(sessionDir); err != nil {
		return UploadSession{}, fmt.Errorf("sync upload session: %w", err)
	}
	s.moveUsage(ownerFromMetadata(session.Metadata), usageKind(-1), usageUpload, session.Size, 1)
	removeSession = false
	return cloneUploadSession(session), nil
}

func (s *DiskStore) GetUpload(ctx context.Context, id string) (UploadSession, error) {
	if err := validateUploadID(id); err != nil {
		return UploadSession{}, err
	}
	if err := ctx.Err(); err != nil {
		return UploadSession{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getUploadUnlocked(id)
}

func (s *DiskStore) AppendUpload(ctx context.Context, id string, offset, chunkSize int64, reader io.Reader, checksum string) (UploadSession, error) {
	if err := validateUploadID(id); err != nil {
		return UploadSession{}, err
	}
	if reader == nil || offset < 0 || chunkSize <= 0 {
		return UploadSession{}, fmt.Errorf("%w: upload chunk is invalid", ErrInvalidUpload)
	}
	if checksum != "" && !isSHA256(checksum) {
		return UploadSession{}, fmt.Errorf("%w: chunk checksum must be a SHA-256 hex string", ErrInvalidUpload)
	}
	if err := ctx.Err(); err != nil {
		return UploadSession{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.getUploadUnlocked(id)
	if err != nil {
		return UploadSession{}, err
	}
	if session.Status != UploadStatusUploading {
		return UploadSession{}, fmt.Errorf("%w: upload session is already completed", ErrUploadOffsetConflict)
	}
	if offset != session.ReceivedBytes {
		return UploadSession{}, fmt.Errorf("%w: expected offset %d, got %d", ErrUploadOffsetConflict, session.ReceivedBytes, offset)
	}
	paused, err := s.writesPausedUnlocked()
	if err != nil {
		return UploadSession{}, err
	}
	if paused {
		return UploadSession{}, ErrStoragePaused
	}

	dataPath := filepath.Join(s.uploadsDir, id, "data.part")
	part, err := os.OpenFile(dataPath, os.O_WRONLY|os.O_APPEND, 0o640)
	if errors.Is(err, os.ErrNotExist) {
		return UploadSession{}, fmt.Errorf("%w: upload part is missing", ErrCorruptMetadata)
	}
	if err != nil {
		return UploadSession{}, fmt.Errorf("open upload part: %w", err)
	}
	defer part.Close()
	info, err := part.Stat()
	if err != nil {
		return UploadSession{}, fmt.Errorf("stat upload part: %w", err)
	}
	if info.Size() != session.ReceivedBytes {
		return UploadSession{}, fmt.Errorf("%w: upload part size does not match manifest", ErrCorruptMetadata)
	}
	initialSize := info.Size()
	hasher := sha256.New()
	remaining := session.Size - session.ReceivedBytes
	if remaining < 0 {
		return UploadSession{}, fmt.Errorf("%w: received bytes exceed declared size", ErrCorruptMetadata)
	}
	if chunkSize > remaining {
		return UploadSession{}, ErrUploadTooLarge
	}
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: reader}, chunkSize+1)
	written, copyErr := io.Copy(io.MultiWriter(part, hasher), limited)
	rollback := func() {
		if err := part.Truncate(initialSize); err != nil {
			log.Printf("rollback upload part %s: %v", id, err)
		}
		_ = part.Sync()
	}
	if copyErr != nil {
		rollback()
		return UploadSession{}, fmt.Errorf("write upload part: %w", copyErr)
	}
	if written > remaining {
		rollback()
		return UploadSession{}, ErrUploadTooLarge
	}
	if written != chunkSize {
		rollback()
		return UploadSession{}, ErrUploadChunkSize
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if checksum != "" && !strings.EqualFold(strings.TrimSpace(checksum), actualChecksum) {
		rollback()
		return UploadSession{}, ErrUploadChecksumMismatch
	}
	if err := part.Sync(); err != nil {
		rollback()
		return UploadSession{}, fmt.Errorf("sync upload part: %w", err)
	}
	session.ReceivedBytes += written
	session.UpdatedAt = s.now()
	manifestPath := filepath.Join(s.uploadsDir, id, "manifest.json")
	if err := writeUploadManifest(manifestPath, session); err != nil {
		rollback()
		session.ReceivedBytes = initialSize
		return UploadSession{}, err
	}
	if err := syncDirectory(filepath.Dir(manifestPath)); err != nil {
		rollback()
		session.ReceivedBytes = initialSize
		return UploadSession{}, fmt.Errorf("sync upload session: %w", err)
	}
	return cloneUploadSession(session), nil
}

func (s *DiskStore) CompleteUpload(ctx context.Context, id string) (File, error) {
	if err := validateUploadID(id); err != nil {
		return File{}, err
	}
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.getUploadUnlocked(id)
	if err != nil {
		return File{}, err
	}
	if session.Status == UploadStatusCompleted && session.FileID != "" {
		return s.getUnlocked(session.FileID)
	}
	if session.ReceivedBytes != session.Size {
		return File{}, ErrUploadIncomplete
	}
	dataPath := filepath.Join(s.uploadsDir, id, "data.part")
	part, err := os.Open(dataPath)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, fmt.Errorf("%w: upload part is missing", ErrCorruptMetadata)
	}
	if err != nil {
		return File{}, fmt.Errorf("open upload part: %w", err)
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(io.Discard, io.TeeReader(&contextReader{ctx: ctx, reader: part}, hasher))
	closeErr := part.Close()
	if copyErr != nil {
		return File{}, fmt.Errorf("read upload part: %w", copyErr)
	}
	if closeErr != nil {
		return File{}, fmt.Errorf("close upload part: %w", closeErr)
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if session.Checksum != "" && !strings.EqualFold(session.Checksum, actualChecksum) {
		return File{}, ErrUploadChecksumMismatch
	}

	fileID := uuid.NewString()
	objectPath := filepath.Join(s.objectsDir, fileID)
	manifestPath := filepath.Join(s.metadataDir, fileID+".json")
	if err := os.Rename(dataPath, objectPath); err != nil {
		return File{}, fmt.Errorf("commit upload object: %w", err)
	}
	removeObject := true
	defer func() {
		if removeObject {
			_ = os.Remove(objectPath)
			_ = os.Remove(manifestPath)
		}
	}()
	if err := syncDirectory(s.objectsDir); err != nil {
		return File{}, fmt.Errorf("sync object directory: %w", err)
	}
	stored := File{
		ID: fileID, Name: session.Name, ContentType: session.ContentType, Size: session.Size,
		Checksum: actualChecksum, Status: StatusAvailable, CreatedAt: session.CreatedAt,
		Metadata: cloneFile(File{Metadata: session.Metadata}).Metadata,
	}
	if err := writeManifest(manifestPath, stored); err != nil {
		return File{}, err
	}
	if err := syncDirectory(s.metadataDir); err != nil {
		return File{}, fmt.Errorf("sync metadata directory: %w", err)
	}
	session.FileID = fileID
	session.Status = UploadStatusCompleted
	session.UpdatedAt = s.now()
	if err := writeUploadManifest(filepath.Join(s.uploadsDir, id, "manifest.json"), session); err != nil {
		return File{}, err
	}
	if err := syncDirectory(filepath.Join(s.uploadsDir, id)); err != nil {
		return File{}, fmt.Errorf("sync upload session: %w", err)
	}
	// The reservation becomes a real object: migrate the bytes instead of
	// charging them a second time.
	s.moveUsage(ownerFromMetadata(session.Metadata), usageUpload, usageFile, session.Size, 1)
	removeObject = false
	_ = os.Remove(dataPath)
	return cloneFile(stored), nil
}

func (s *DiskStore) AbortUpload(ctx context.Context, id string) error {
	if err := validateUploadID(id); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.getUploadUnlocked(id)
	if errors.Is(err, ErrUploadNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if session.Status == UploadStatusCompleted {
		return fmt.Errorf("%w: completed upload cannot be aborted", ErrInvalidUpload)
	}
	if err := os.RemoveAll(filepath.Join(s.uploadsDir, id)); err != nil {
		return fmt.Errorf("remove upload session: %w", err)
	}
	if err := syncDirectory(s.uploadsDir); err != nil {
		return err
	}
	// Release the quota reservation held by the abandoned session.
	s.releaseUsage(ownerFromMetadata(session.Metadata), usageUpload, session.Size)
	return nil
}

func (s *DiskStore) Capacity(ctx context.Context) (Capacity, error) {
	if err := ctx.Err(); err != nil {
		return Capacity{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capacityUnlocked()
}

func (s *DiskStore) Quota(ctx context.Context, owner string) (Quota, error) {
	if err := ctx.Err(); err != nil {
		return Quota{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	owner = normalizeOwner(owner)
	used, err := s.ownerUsageLocked(owner)
	if err != nil {
		return Quota{}, err
	}
	limit := s.ownerQuotaBytes
	available := int64(0)
	if limit > used {
		available = limit - used
	}
	usedPercent := 0.0
	if limit > 0 {
		usedPercent = float64(used) * 100 / float64(limit)
	}
	return Quota{Owner: owner, LimitBytes: limit, UsedBytes: used, AvailableBytes: available, UsedPercent: usedPercent}, nil
}

func (s *DiskStore) enforceQuotaUnlocked(owner string, incoming int64) error {
	if s.ownerQuotaBytes == 0 {
		return nil
	}
	if incoming < 0 {
		return fmt.Errorf("%w: incoming size is negative", ErrInvalidUpload)
	}
	used, err := s.ownerUsageLocked(owner)
	if err != nil {
		return err
	}
	if used > s.ownerQuotaBytes || incoming > s.ownerQuotaBytes-used {
		return fmt.Errorf("%w: owner %q has %d bytes used and %d bytes available", ErrQuotaExceeded, normalizeOwner(owner), used, maxInt64(0, s.ownerQuotaBytes-used))
	}
	return nil
}

// ensureUsageLocked loads the per-owner usage index once per store instance.
// Every mutation updates the index incrementally, so uploads no longer rescan
// every manifest on the hot path. Callers must hold s.mu.
func (s *DiskStore) ensureUsageLocked() error {
	s.load.Do(func() {
		s.loadErr = s.rebuildUsageLocked()
	})
	return s.loadErr
}

// RebuildUsage discards the cached accounting and rescans the store. Run it if
// files are changed outside the service, for example by restore tooling.
func (s *DiskStore) RebuildUsage(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuildUsageLocked()
}

func (s *DiskStore) rebuildUsageLocked() error {
	dataFiles, err := os.ReadDir(s.metadataDir)
	if err != nil {
		return fmt.Errorf("read metadata directory: %w", err)
	}
	trashEntries, err := os.ReadDir(s.trashDir)
	if err != nil {
		return fmt.Errorf("read trash directory: %w", err)
	}
	uploadEntries, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		return fmt.Errorf("read upload directory: %w", err)
	}

	index := newUsageIndex()
	add := func(kind usageKind, owner string, size int64) error {
		if size < 0 {
			return fmt.Errorf("%w: negative object size", ErrCorruptMetadata)
		}
		owner = normalizeOwner(owner)
		bucket := index.bucket(kind)
		current := bucket[owner]
		if current.Bytes > math.MaxInt64-size {
			return fmt.Errorf("%w: owner usage overflow", ErrCorruptMetadata)
		}
		current.Bytes += size
		current.Records++
		bucket[owner] = current
		return nil
	}

	for _, entry := range dataFiles {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateID(id); err != nil {
			return fmt.Errorf("%w: unexpected manifest %s", ErrCorruptMetadata, entry.Name())
		}
		file, err := s.getUnlocked(id)
		if err != nil {
			return err
		}
		if err := add(usageFile, ownerFromMetadata(file.Metadata), file.Size); err != nil {
			return err
		}
	}
	for _, entry := range trashEntries {
		if !entry.IsDir() {
			continue
		}
		file, err := s.getTrashUnlocked(entry.Name())
		if err != nil {
			return err
		}
		if err := add(usageTrash, ownerFromMetadata(file.Metadata), file.Size); err != nil {
			return err
		}
	}
	for _, entry := range uploadEntries {
		if !entry.IsDir() {
			continue
		}
		session, err := s.getUploadUnlocked(entry.Name())
		if err != nil {
			return err
		}
		if session.Status != UploadStatusUploading {
			continue
		}
		// Reserve the declared size, not the received bytes: otherwise a
		// client could declare 1 GiB, never finish, and hold the quota for free.
		if err := add(usageUpload, ownerFromMetadata(session.Metadata), session.Size); err != nil {
			return err
		}
	}
	s.usage = index
	s.usageValid = true
	return nil
}

func (s *DiskStore) ownerUsageLocked(owner string) (int64, error) {
	if err := s.ensureUsageLocked(); err != nil {
		return 0, err
	}
	return s.usage.total(normalizeOwner(owner)), nil
}

// moveUsage transfers bytes and records between categories for one owner. It is
// the only mutation primitive, which keeps the three categories consistent:
// records never appear twice and never disappear silently.
func (s *DiskStore) moveUsage(owner string, from, to usageKind, size int64, records int) {
	if !s.usageValid || size <= 0 {
		return
	}
	owner = normalizeOwner(owner)
	if from >= 0 {
		bucket := s.usage.bucket(from)
		entry := bucket[owner]
		entry.Bytes -= size
		entry.Records -= records
		if entry.Bytes <= 0 {
			delete(bucket, owner)
		} else {
			if entry.Records < 0 {
				entry.Records = 0
			}
			bucket[owner] = entry
		}
	}
	if to >= 0 {
		bucket := s.usage.bucket(to)
		entry := bucket[owner]
		entry.Bytes += size
		entry.Records += records
		bucket[owner] = entry
	}
}

// releaseUsage drops one record from a category entirely.
func (s *DiskStore) releaseUsage(owner string, kind usageKind, size int64) {
	s.moveUsage(owner, kind, usageKind(-1), size, 1)
}

// ownerUsageUnlocked walks the store and returns the quota usage for one owner.
// It remains the authoritative slow path behind RebuildUsage.
func (s *DiskStore) ownerUsageUnlocked(owner string) (int64, error) {
	if err := s.rebuildUsageLocked(); err != nil {
		return 0, err
	}
	return s.usage.total(normalizeOwner(owner)), nil
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
	// Object and trash statistics come from the accounting index instead of two
	// extra directory walks, so a capacity probe no longer scales with the
	// number of stored objects.
	if err := s.ensureUsageLocked(); err != nil {
		return Capacity{}, err
	}
	// The threshold applies to the whole filesystem, not to the data directory:
	// other tenants on the same volume can pause uploads, and that is intended.
	paused := s.pauseAtPercent > 0 && usedPercent >= float64(s.pauseAtPercent)
	return Capacity{
		TotalBytes: total, UsedBytes: used, AvailableBytes: available,
		UsedPercent: usedPercent, ObjectCount: s.usage.activeCount(), ObjectBytes: s.usage.bytesOf(),
		MetadataCount: s.usage.activeCount(), TrashCount: s.usage.trashedCount(), TrashBytes: s.usage.bytesInTrash(),
		PauseAtPercent: s.pauseAtPercent, WritesPaused: paused,
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

// Count returns the number of active objects without loading their manifests.
func (s *DiskStore) Count(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.metadataDir)
	if err != nil {
		return 0, fmt.Errorf("list metadata: %w", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if validateID(strings.TrimSuffix(entry.Name(), ".json")) != nil {
			return 0, fmt.Errorf("%w: unexpected manifest %s", ErrCorruptMetadata, entry.Name())
		}
		count++
	}
	return count, nil
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
	objectPath := filepath.Join(s.objectsDir, id)
	manifestPath := filepath.Join(s.metadataDir, id+".json")
	trashItemDir := filepath.Join(s.trashDir, id)
	trashObjectPath := filepath.Join(trashItemDir, "object")
	trashManifestPath := filepath.Join(trashItemDir, "manifest.json")
	objectExists := pathExists(objectPath)
	manifestExists := pathExists(manifestPath)
	trashExists := pathExists(trashItemDir)
	if !objectExists && !manifestExists {
		if trashExists {
			return nil
		}
		return nil
	}
	if !objectExists || !manifestExists || trashExists {
		return fmt.Errorf("%w: inconsistent delete state for %s", ErrCorruptMetadata, id)
	}
	if err := os.Mkdir(trashItemDir, 0o750); err != nil {
		return fmt.Errorf("create trash entry: %w", err)
	}
	rollbackObject := false
	defer func() {
		if rollbackObject {
			_ = os.Rename(trashObjectPath, objectPath)
			_ = os.Rename(trashManifestPath, manifestPath)
			_ = os.RemoveAll(trashItemDir)
		}
	}()
	if err := os.Rename(objectPath, trashObjectPath); err != nil {
		_ = os.Remove(trashItemDir)
		return fmt.Errorf("move object to trash: %w", err)
	}
	rollbackObject = true
	if err := syncDirectory(s.objectsDir); err != nil {
		return fmt.Errorf("sync object directory: %w", err)
	}
	if err := os.Rename(manifestPath, trashManifestPath); err != nil {
		return fmt.Errorf("move metadata to trash: %w", err)
	}
	if err := syncDirectory(s.metadataDir); err != nil {
		return fmt.Errorf("sync metadata directory: %w", err)
	}
	data, err := os.ReadFile(trashManifestPath)
	if err != nil {
		return fmt.Errorf("read trashed metadata: %w", err)
	}
	var stored File
	if err := json.Unmarshal(data, &stored); err != nil || stored.ID != id {
		return fmt.Errorf("%w: invalid trashed metadata %s", ErrCorruptMetadata, id)
	}
	deletedAt := s.now()
	stored.DeletedAt = &deletedAt
	if err := writeManifest(trashManifestPath, stored); err != nil {
		return err
	}
	if err := syncDirectory(trashItemDir); err != nil {
		return fmt.Errorf("sync trash entry: %w", err)
	}
	rollbackObject = false
	if err := syncDirectory(s.trashDir); err != nil {
		return fmt.Errorf("sync trash directory: %w", err)
	}
	// Trashed bytes still count against the owner, so move them between
	// categories rather than releasing them.
	s.moveUsage(ownerFromMetadata(stored.Metadata), usageFile, usageTrash, stored.Size, 1)
	return nil
}

func (s *DiskStore) ListTrash(ctx context.Context, limit, offset int) ([]File, error) {
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
	entries, err := os.ReadDir(s.trashDir)
	if err != nil {
		return nil, fmt.Errorf("list trash: %w", err)
	}
	trashed := make([]File, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("%w: unexpected trash artifact %s", ErrCorruptMetadata, entry.Name())
		}
		if err := validateID(entry.Name()); err != nil {
			return nil, fmt.Errorf("%w: unexpected trash entry %s", ErrCorruptMetadata, entry.Name())
		}
		file, err := s.getTrashUnlocked(entry.Name())
		if err != nil {
			return nil, err
		}
		trashed = append(trashed, file)
	}
	sort.Slice(trashed, func(i, j int) bool {
		if trashed[i].DeletedAt != nil && trashed[j].DeletedAt != nil && !trashed[i].DeletedAt.Equal(*trashed[j].DeletedAt) {
			return trashed[i].DeletedAt.After(*trashed[j].DeletedAt)
		}
		return trashed[i].ID < trashed[j].ID
	})
	if offset >= len(trashed) {
		return []File{}, nil
	}
	end := offset + limit
	if end > len(trashed) {
		end = len(trashed)
	}
	result := make([]File, 0, end-offset)
	for _, file := range trashed[offset:end] {
		result = append(result, cloneFile(file))
	}
	return result, nil
}

func (s *DiskStore) Restore(ctx context.Context, id string) (File, error) {
	if err := validateID(id); err != nil {
		return File{}, err
	}
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.getTrashUnlocked(id)
	if err != nil {
		return File{}, err
	}
	objectPath := filepath.Join(s.objectsDir, id)
	manifestPath := filepath.Join(s.metadataDir, id+".json")
	if pathExists(objectPath) || pathExists(manifestPath) {
		return File{}, ErrRestoreConflict
	}
	trashItemDir := filepath.Join(s.trashDir, id)
	trashObjectPath := filepath.Join(trashItemDir, "object")
	trashManifestPath := filepath.Join(trashItemDir, "manifest.json")
	file.DeletedAt = nil
	if err := writeManifest(trashManifestPath, file); err != nil {
		return File{}, err
	}
	if err := syncDirectory(trashItemDir); err != nil {
		return File{}, fmt.Errorf("sync trash entry: %w", err)
	}
	if err := os.Rename(trashObjectPath, objectPath); err != nil {
		return File{}, fmt.Errorf("restore object: %w", err)
	}
	rollbackObject := true
	defer func() {
		if rollbackObject {
			_ = os.Rename(objectPath, trashObjectPath)
		}
	}()
	if err := syncDirectory(s.objectsDir); err != nil {
		return File{}, fmt.Errorf("sync object directory: %w", err)
	}
	if err := os.Rename(trashManifestPath, manifestPath); err != nil {
		return File{}, fmt.Errorf("restore metadata: %w", err)
	}
	if err := syncDirectory(s.metadataDir); err != nil {
		return File{}, fmt.Errorf("sync metadata directory: %w", err)
	}
	if err := os.Remove(trashItemDir); err != nil {
		return File{}, fmt.Errorf("remove empty trash entry: %w", err)
	}
	if err := syncDirectory(s.trashDir); err != nil {
		return File{}, fmt.Errorf("sync trash directory: %w", err)
	}
	rollbackObject = false
	// The bytes leave the trash bucket and rejoin the active objects.
	s.moveUsage(ownerFromMetadata(file.Metadata), usageTrash, usageFile, file.Size, 1)
	return cloneFile(file), nil
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
	uploads, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		return fmt.Errorf("read upload directory: %w", err)
	}
	for _, entry := range uploads {
		if !entry.IsDir() || validateUploadID(entry.Name()) != nil {
			return fmt.Errorf("%w: unexpected upload artifact %s", ErrCorruptMetadata, entry.Name())
		}
		sessionDir := filepath.Join(s.uploadsDir, entry.Name())
		children, err := os.ReadDir(sessionDir)
		if err != nil {
			return fmt.Errorf("read upload session: %w", err)
		}
		hasManifest := false
		for _, child := range children {
			if child.Name() == "manifest.json" {
				hasManifest = true
				continue
			}
			if child.Name() != "data.part" {
				return fmt.Errorf("%w: unexpected upload artifact %s/%s", ErrCorruptMetadata, entry.Name(), child.Name())
			}
		}
		if !hasManifest {
			return fmt.Errorf("%w: upload %s has no manifest", ErrCorruptMetadata, entry.Name())
		}
		session, err := s.getUploadUnlocked(entry.Name())
		if err != nil {
			return fmt.Errorf("%w: upload %s: %v", ErrCorruptMetadata, entry.Name(), err)
		}
		if session.Status == UploadStatusCompleted {
			for _, child := range children {
				if child.Name() == "data.part" {
					return fmt.Errorf("%w: completed upload %s still has a part", ErrCorruptMetadata, entry.Name())
				}
			}
		}
	}
	trash, err := os.ReadDir(s.trashDir)
	if err != nil {
		return fmt.Errorf("read trash directory: %w", err)
	}
	for _, entry := range trash {
		if !entry.IsDir() || validateID(entry.Name()) != nil {
			return fmt.Errorf("%w: unexpected trash artifact %s", ErrCorruptMetadata, entry.Name())
		}
		children, err := os.ReadDir(filepath.Join(s.trashDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read trash entry: %w", err)
		}
		hasObject, hasManifest := false, false
		for _, child := range children {
			switch child.Name() {
			case "object":
				hasObject = true
			case "manifest.json":
				hasManifest = true
			default:
				return fmt.Errorf("%w: unexpected trash artifact %s/%s", ErrCorruptMetadata, entry.Name(), child.Name())
			}
		}
		if !hasObject || !hasManifest {
			return fmt.Errorf("%w: incomplete trash entry %s", ErrCorruptMetadata, entry.Name())
		}
		if _, err := s.getTrashUnlocked(entry.Name()); err != nil {
			return err
		}
	}
	if err := s.readyQuarantineUnlocked(); err != nil {
		return err
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

// readyQuarantineUnlocked validates the quarantine area. Recover moves crash
// leftovers there instead of deleting them, so the directory must stay
// trustworthy: only the known groups, and only entries inside them.
func (s *DiskStore) readyQuarantineUnlocked() error {
	groups, err := os.ReadDir(s.quarantineDir)
	if err != nil {
		return fmt.Errorf("read quarantine directory: %w", err)
	}
	for _, group := range groups {
		if !group.IsDir() {
			return fmt.Errorf("%w: unexpected quarantine artifact %s", ErrCorruptMetadata, group.Name())
		}
		switch group.Name() {
		case "partials", "metadata", "uploads":
		default:
			return fmt.Errorf("%w: unexpected quarantine group %s", ErrCorruptMetadata, group.Name())
		}
	}
	return nil
}

func validateID(id string) error {
	if !validFileID.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return nil
}

func (s *DiskStore) getTrashUnlocked(id string) (File, error) {
	manifestPath := filepath.Join(s.trashDir, id, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return File{}, fmt.Errorf("read trash metadata: %w", err)
	}
	var stored File
	if err := json.Unmarshal(data, &stored); err != nil {
		return File{}, fmt.Errorf("%w: trash %s: %v", ErrCorruptMetadata, id, err)
	}
	if stored.ID != id || stored.Status != StatusAvailable || stored.Size < 0 || stored.Checksum == "" || stored.DeletedAt == nil {
		return File{}, fmt.Errorf("%w: invalid trash metadata %s", ErrCorruptMetadata, id)
	}
	objectPath := filepath.Join(s.trashDir, id, "object")
	info, err := os.Stat(objectPath)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, fmt.Errorf("%w: trash object %s is missing", ErrCorruptMetadata, id)
	}
	if err != nil {
		return File{}, fmt.Errorf("stat trash object: %w", err)
	}
	if info.Size() != stored.Size {
		return File{}, fmt.Errorf("%w: trash object %s has unexpected size", ErrCorruptMetadata, id)
	}
	return cloneFile(stored), nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func normalizeOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return "_anonymous"
	}
	return owner
}

func ownerFromMetadata(metadata map[string]string) string {
	if metadata == nil {
		return "_anonymous"
	}
	return normalizeOwner(metadata["owner"])
}

func validateUploadID(id string) error {
	return validateID(id)
}

func (s *DiskStore) getUploadUnlocked(id string) (UploadSession, error) {
	manifestPath := filepath.Join(s.uploadsDir, id, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return UploadSession{}, fmt.Errorf("%w: %s", ErrUploadNotFound, id)
	}
	if err != nil {
		return UploadSession{}, fmt.Errorf("read upload metadata: %w", err)
	}
	var session UploadSession
	if err := json.Unmarshal(data, &session); err != nil {
		return UploadSession{}, fmt.Errorf("%w: upload %s: %v", ErrCorruptMetadata, id, err)
	}
	if session.ID != id || (session.Status != UploadStatusUploading && session.Status != UploadStatusCompleted) || session.Size <= 0 || session.ReceivedBytes < 0 || session.ReceivedBytes > session.Size {
		return UploadSession{}, fmt.Errorf("%w: upload %s", ErrCorruptMetadata, id)
	}
	if session.Status == UploadStatusUploading {
		dataPath := filepath.Join(s.uploadsDir, id, "data.part")
		info, err := os.Stat(dataPath)
		if errors.Is(err, os.ErrNotExist) {
			return UploadSession{}, fmt.Errorf("%w: upload part %s is missing", ErrCorruptMetadata, id)
		}
		if err != nil {
			return UploadSession{}, fmt.Errorf("stat upload part: %w", err)
		}
		if info.Size() != session.ReceivedBytes {
			return UploadSession{}, fmt.Errorf("%w: upload part size does not match manifest", ErrCorruptMetadata)
		}
	}
	if session.Status == UploadStatusCompleted && (session.FileID == "" || validateID(session.FileID) != nil) {
		return UploadSession{}, fmt.Errorf("%w: upload %s has invalid completed file", ErrCorruptMetadata, id)
	}
	return cloneUploadSession(session), nil
}

func writeUploadManifest(path string, session UploadSession) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("encode upload metadata: %w", err)
	}
	temporaryPath := path + ".tmp"
	temporary, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("create upload metadata: %w", err)
	}
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write upload metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync upload metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close upload metadata: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit upload metadata: %w", err)
	}
	removeTemporary = false
	return nil
}

func cloneUploadSession(session UploadSession) UploadSession {
	if session.Metadata == nil {
		return session
	}
	cloned := make(map[string]string, len(session.Metadata))
	for key, value := range session.Metadata {
		cloned[key] = value
	}
	session.Metadata = cloned
	return session
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
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
