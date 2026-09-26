package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RecoveryReport describes what Recover repaired. Every field counts artifacts
// that were moved out of the live directories instead of being treated as
// corruption.
type RecoveryReport struct {
	// PartialObjects counts tmp/*.part files left by an interrupted Put.
	PartialObjects int `json:"partial_objects"`
	// MetadataManifests counts metadata/*.json.tmp files left by an
	// interrupted manifest commit.
	MetadataManifests int `json:"metadata_manifests"`
	// UploadManifests counts uploads/*/manifest.json.tmp files.
	UploadManifests int `json:"upload_manifests"`
	// UploadSessions counts upload session directories removed because they
	// carried no usable manifest.
	UploadSessions int `json:"upload_sessions"`
	// ExpiredUploads counts upload sessions removed by TTL expiry.
	ExpiredUploads int `json:"expired_uploads"`
	// ExpiredTrash counts trash entries removed by retention.
	ExpiredTrash int `json:"expired_trash"`
}

// Recovered reports whether anything was repaired.
func (r RecoveryReport) Recovered() bool {
	return r.PartialObjects+r.MetadataManifests+r.UploadManifests+r.UploadSessions+r.ExpiredUploads+r.ExpiredTrash > 0
}

// Recover quarantines crash leftovers that would otherwise make Ready fail
// forever. A process killed between writing a temporary manifest and renaming
// it leaves metadata/{id}.json.tmp or uploads/{id}/manifest.json.tmp behind;
// Ready treats those as corrupt metadata, which means the service can never
// become ready again without manual cleanup.
//
// Recovery is deliberately conservative: only artifacts that this package
// writes as temporary files are touched, they are moved into quarantine/ rather
// than deleted so the evidence survives, and anything genuinely inconsistent is
// still reported by the following Ready call.
func (s *DiskStore) Recover(ctx context.Context) (RecoveryReport, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryReport{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var report RecoveryReport

	partials, err := os.ReadDir(s.tmpDir)
	if err != nil {
		return report, fmt.Errorf("read temporary directory: %w", err)
	}
	for _, entry := range partials {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		if err := s.quarantinePath(filepath.Join(s.tmpDir, entry.Name()), "partials", entry.Name()); err != nil {
			return report, err
		}
		report.PartialObjects++
	}

	metadata, err := os.ReadDir(s.metadataDir)
	if err != nil {
		return report, fmt.Errorf("read metadata directory: %w", err)
	}
	for _, entry := range metadata {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json.tmp") {
			continue
		}
		if err := s.quarantinePath(filepath.Join(s.metadataDir, entry.Name()), "metadata", entry.Name()); err != nil {
			return report, err
		}
		report.MetadataManifests++
	}

	uploads, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		return report, fmt.Errorf("read upload directory: %w", err)
	}
	for _, entry := range uploads {
		if !entry.IsDir() {
			continue
		}
		sessionDir := filepath.Join(s.uploadsDir, entry.Name())
		children, err := os.ReadDir(sessionDir)
		if err != nil {
			return report, fmt.Errorf("read upload session: %w", err)
		}
		hasManifest := false
		for _, child := range children {
			if child.Name() == "manifest.json" {
				hasManifest = true
				continue
			}
			if strings.HasSuffix(child.Name(), ".json.tmp") {
				if err := s.quarantinePath(
					filepath.Join(sessionDir, child.Name()),
					"uploads",
					entry.Name()+"-"+child.Name(),
				); err != nil {
					return report, err
				}
				report.UploadManifests++
			}
		}
		if hasManifest {
			continue
		}
		// No manifest at all: the session never became usable, so the whole
		// directory is a leftover.
		if err := s.quarantinePath(sessionDir, "uploads", entry.Name()); err != nil {
			return report, err
		}
		report.UploadSessions++
	}

	expiredUploads, err := s.sweepUploadsUnlocked(s.now())
	if err != nil {
		return report, err
	}
	report.ExpiredUploads = expiredUploads

	if report.Recovered() {
		// Artifacts were removed from disk outside the normal mutation paths,
		// so reconcile the accounting with a full rescan.
		if err := s.rebuildUsageLocked(); err != nil {
			return report, err
		}
	}
	return report, nil
}

// sweepUploads removes upload sessions that stopped making progress. Sessions
// reserve their declared size against the owner quota, so an abandoned session
// would otherwise hold quota forever.
func (s *DiskStore) sweepUploadsUnlocked(now time.Time) (int, error) {
	if s.uploadTTL <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		return 0, fmt.Errorf("read upload directory: %w", err)
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if err := validateUploadID(id); err != nil {
			continue
		}
		session, err := s.getUploadUnlocked(id)
		if err != nil {
			if errors.Is(err, ErrUploadNotFound) || errors.Is(err, ErrCorruptMetadata) {
				// A manifest we cannot parse is indistinguishable from an
				// abandoned session; leave it for Recover to quarantine.
				continue
			}
			return removed, err
		}
		if session.Status == UploadStatusCompleted {
			// Completed sessions hold no data and no quota; drop the bookkeeping.
			if err := os.RemoveAll(filepath.Join(s.uploadsDir, id)); err != nil {
				return removed, fmt.Errorf("remove completed upload session: %w", err)
			}
			removed++
			continue
		}
		if now.Sub(session.UpdatedAt) < s.uploadTTL {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.uploadsDir, id)); err != nil {
			return removed, fmt.Errorf("remove expired upload session: %w", err)
		}
		s.releaseUsage(ownerFromMetadata(session.Metadata), usageUpload, session.Size)
		removed++
	}
	if removed > 0 {
		if err := syncDirectory(s.uploadsDir); err != nil {
			return removed, fmt.Errorf("sync upload directory: %w", err)
		}
	}
	return removed, nil
}

// SweepUploads expires abandoned upload sessions and returns how many were
// removed.
func (s *DiskStore) SweepUploads(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sweepUploadsUnlocked(s.now())
}

// PurgeTrash permanently removes trash entries deleted at or before olderThan
// and returns how many entries were destroyed. Passing the zero time removes
// every trashed entry.
func (s *DiskStore) PurgeTrash(ctx context.Context, olderThan time.Time) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.trashDir)
	if err != nil {
		return 0, fmt.Errorf("read trash directory: %w", err)
	}
	purged := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateID(entry.Name()); err != nil {
			return purged, fmt.Errorf("%w: unexpected trash entry %s", ErrCorruptMetadata, entry.Name())
		}
		stored, err := s.getTrashUnlocked(entry.Name())
		if err != nil {
			return purged, err
		}
		deletedAt := stored.CreatedAt
		if stored.DeletedAt != nil {
			deletedAt = *stored.DeletedAt
		}
		if !olderThan.IsZero() && deletedAt.After(olderThan) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.trashDir, entry.Name())); err != nil {
			return purged, fmt.Errorf("purge trash entry: %w", err)
		}
		s.releaseUsage(ownerFromMetadata(stored.Metadata), usageTrash, stored.Size)
		purged++
	}
	if purged > 0 {
		if err := syncDirectory(s.trashDir); err != nil {
			return purged, fmt.Errorf("sync trash directory: %w", err)
		}
	}
	return purged, nil
}

// ListUploads returns the persisted upload sessions, newest first.
func (s *DiskStore) ListUploads(ctx context.Context) ([]UploadSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.uploadsDir)
	if err != nil {
		return nil, fmt.Errorf("read upload directory: %w", err)
	}
	sessions := make([]UploadSession, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || validateUploadID(entry.Name()) != nil {
			continue
		}
		session, err := s.getUploadUnlocked(entry.Name())
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, cloneUploadSession(session))
	}
	sortSessionsByUpdated(sessions)
	return sessions, nil
}

func sortSessionsByUpdated(sessions []UploadSession) {
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0; j-- {
			left, right := sessions[j-1], sessions[j]
			if right.UpdatedAt.After(left.UpdatedAt) || (right.UpdatedAt.Equal(left.UpdatedAt) && right.ID < left.ID) {
				sessions[j-1], sessions[j] = right, left
				continue
			}
			break
		}
	}
}

// quarantinePath moves path into quarantine/<group>/<name>, never overwriting
// an existing entry.
func (s *DiskStore) quarantinePath(path, group, name string) error {
	targetDir := filepath.Join(s.quarantineDir, group)
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return fmt.Errorf("create quarantine directory: %w", err)
	}
	target := filepath.Join(targetDir, filepath.Base(name))
	if pathExists(target) {
		target = filepath.Join(targetDir, fmt.Sprintf("%d-%s", s.now().UnixNano(), filepath.Base(name)))
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat quarantined artifact: %w", err)
	}
	if info.IsDir() {
		if err := copyTree(path, target); err != nil {
			return fmt.Errorf("quarantine directory: %w", err)
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove quarantined directory: %w", err)
		}
	} else {
		if err := os.Rename(path, target); err != nil {
			return fmt.Errorf("quarantine artifact: %w", err)
		}
	}
	if err := syncDirectory(s.quarantineDir); err != nil {
		return fmt.Errorf("sync quarantine directory: %w", err)
	}
	return nil
}

func copyTree(source, target string) error {
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		targetPath := filepath.Join(target, entry.Name())
		if entry.IsDir() {
			if err := copyTree(sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return syncDirectory(target)
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}
