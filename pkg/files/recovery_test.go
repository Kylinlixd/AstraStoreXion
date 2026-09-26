package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClock is a hand-driven clock so expiry can be tested without sleeping.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

// newRecoverableStore builds a store whose clock the test controls.
func newRecoverableStore(t *testing.T, options Options) (*DiskStore, string, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	options.clock = clock.Now
	root := t.TempDir()
	store, err := NewDiskStoreWithOptions(root, options)
	require.NoError(t, err)
	return store, root, clock
}

// TestDiskStoreRecoverUnblocksReadyAfterCrash reproduces the failure that made
// a restarted process permanently unready: a manifest was written to its
// temporary name but the process died before the rename.
func TestDiskStoreRecoverUnblocksReadyAfterCrash(t *testing.T) {
	const orphanID = "b8c21d60-e970-4df5-890b-0d2dba93a654"
	store, root, _ := newRecoverableStore(t, Options{})
	ctx := context.Background()

	saved, err := store.Put(ctx, UploadInput{Name: "keep.txt", Reader: strings.NewReader("keep me")})
	require.NoError(t, err)

	// Crash leftovers, exactly where the writers put them.
	require.NoError(t, os.WriteFile(filepath.Join(root, "metadata", orphanID+".json.tmp"), []byte("{}"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tmp", orphanID+".part"), []byte("half"), 0o640))
	sessionDir := filepath.Join(root, "uploads", "11111111-2222-3333-4444-555555555555")
	require.NoError(t, os.MkdirAll(sessionDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "manifest.json.tmp"), []byte("{}"), 0o640))

	// Before recovery the store is unready, which is what blocked restarts.
	require.ErrorIs(t, store.Ready(ctx), ErrCorruptMetadata)

	report, err := store.Recover(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, report.PartialObjects)
	assert.Equal(t, 1, report.MetadataManifests)
	assert.Equal(t, 1, report.UploadManifests)
	assert.True(t, report.Recovered())

	// Recovery must restore readiness without hiding real data.
	require.NoError(t, store.Ready(ctx))
	got, err := store.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.Checksum, got.Checksum)

	// Evidence is preserved, not deleted.
	quarantined, err := os.ReadDir(filepath.Join(root, "quarantine", "metadata"))
	require.NoError(t, err)
	require.Len(t, quarantined, 1)
	assert.Equal(t, orphanID+".json.tmp", quarantined[0].Name())
}

// TestDiskStoreRecoverQuarantinesUnusableSession covers a session directory
// whose manifest never landed: the whole directory is a leftover.
func TestDiskStoreRecoverQuarantinesUnusableSession(t *testing.T) {
	const uploadID = "11111111-2222-3333-4444-555555555555"
	store, root, _ := newRecoverableStore(t, Options{})
	ctx := context.Background()

	sessionDir := filepath.Join(root, "uploads", uploadID)
	require.NoError(t, os.MkdirAll(sessionDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "data.part"), []byte("partial"), 0o640))
	require.ErrorIs(t, store.Ready(ctx), ErrCorruptMetadata)

	report, err := store.Recover(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, report.UploadSessions)
	require.NoError(t, store.Ready(ctx))
}

// TestDiskStoreRecoverStillReportsRealCorruption is the guard rail: recovery
// must not turn genuine inconsistency into a green light.
func TestDiskStoreRecoverStillReportsRealCorruption(t *testing.T) {
	const orphanID = "b8c21d60-e970-4df5-890b-0d2dba93a654"
	store, root, _ := newRecoverableStore(t, Options{})
	ctx := context.Background()

	// An object with no manifest is real corruption, not a temp artifact.
	require.NoError(t, os.WriteFile(filepath.Join(root, "objects", orphanID), []byte("orphan"), 0o640))

	_, err := store.Recover(ctx)
	require.NoError(t, err)
	assert.ErrorIs(t, store.Ready(ctx), ErrCorruptMetadata)
}

// TestDiskStoreRecoverIsIdempotent makes sure a second pass is harmless.
func TestDiskStoreRecoverIsIdempotent(t *testing.T) {
	store, root, _ := newRecoverableStore(t, Options{})
	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(root, "tmp", "b8c21d60-e970-4df5-890b-0d2dba93a654.part"), []byte("x"), 0o640))

	first, err := store.Recover(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, first.PartialObjects)

	second, err := store.Recover(ctx)
	require.NoError(t, err)
	assert.False(t, second.Recovered())
}

// TestDiskStoreSweepUploadsExpiresAbandonedSessions verifies that a client that
// declares a large upload and vanishes stops holding quota.
func TestDiskStoreSweepUploadsExpiresAbandonedSessions(t *testing.T) {
	store, _, now := newRecoverableStore(t, Options{UploadTTL: time.Hour})
	ctx := context.Background()

	stale, err := store.StartUpload(ctx, MultipartStartInput{
		Name: "big.bin", Size: 4096, Metadata: map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)
	_, err = store.AppendUpload(ctx, stale.ID, 0, 8, strings.NewReader("12345678"), "")
	require.NoError(t, err)

	quota, err := store.Quota(ctx, "blog")
	require.NoError(t, err)
	assert.EqualValues(t, 4096, quota.UsedBytes)

	// Still inside the TTL: nothing is removed.
	removed, err := store.SweepUploads(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)

	now.Advance(2 * time.Hour)
	removed, err = store.SweepUploads(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	quota, err = store.Quota(ctx, "blog")
	require.NoError(t, err)
	assert.EqualValues(t, 0, quota.UsedBytes)
	_, err = store.GetUpload(ctx, stale.ID)
	assert.ErrorIs(t, err, ErrUploadNotFound)
}

// TestDiskStoreStartUploadSweepsExpiredSessions proves expiry happens without a
// maintenance call, so quota is reclaimed on the next client attempt.
func TestDiskStoreStartUploadSweepsExpiredSessions(t *testing.T) {
	store, _, now := newRecoverableStore(t, Options{UploadTTL: time.Hour})
	ctx := context.Background()

	first, err := store.StartUpload(ctx, MultipartStartInput{Name: "one.bin", Size: 2048})
	require.NoError(t, err)
	now.Advance(3 * time.Hour)

	_, err = store.StartUpload(ctx, MultipartStartInput{Name: "two.bin", Size: 2048})
	require.NoError(t, err)
	_, err = store.GetUpload(ctx, first.ID)
	assert.ErrorIs(t, err, ErrUploadNotFound)
}

// TestDiskStoreSweepUploadsDropsCompletedSessions keeps completed sessions from
// accumulating forever.
func TestDiskStoreSweepUploadsDropsCompletedSessions(t *testing.T) {
	store, _, _ := newRecoverableStore(t, Options{UploadTTL: time.Hour})
	ctx := context.Background()

	session, err := store.StartUpload(ctx, MultipartStartInput{Name: "done.bin", Size: 4})
	require.NoError(t, err)
	_, err = store.AppendUpload(ctx, session.ID, 0, 4, strings.NewReader("done"), "")
	require.NoError(t, err)
	stored, err := store.CompleteUpload(ctx, session.ID)
	require.NoError(t, err)

	removed, err := store.SweepUploads(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	_, err = store.GetUpload(ctx, session.ID)
	assert.ErrorIs(t, err, ErrUploadNotFound)

	// The committed object is untouched.
	got, err := store.Get(ctx, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, stored.Checksum, got.Checksum)
}

func TestDiskStorePurgeTrashRemovesOnlyOldEntries(t *testing.T) {
	store, _, now := newRecoverableStore(t, Options{})
	ctx := context.Background()

	old, err := store.Put(ctx, UploadInput{Name: "old.txt", Reader: strings.NewReader("old")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, old.ID))

	now.Advance(48 * time.Hour)

	recent, err := store.Put(ctx, UploadInput{Name: "recent.txt", Reader: strings.NewReader("recent")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, recent.ID))

	// Cutoff 24h ago keeps the entry deleted an hour ago.
	purged, err := store.PurgeTrash(ctx, now.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, purged)

	remaining, err := store.ListTrash(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, recent.ID, remaining[0].ID)

	// Purge everything.
	purged, err = store.PurgeTrash(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, purged)
	remaining, err = store.ListTrash(ctx, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, remaining)
	require.NoError(t, store.Ready(ctx))
}

func TestDiskStorePurgeTrashReleasesQuota(t *testing.T) {
	store, _, _ := newRecoverableStore(t, Options{OwnerQuotaBytes: 10})
	ctx := context.Background()

	saved, err := store.Put(ctx, UploadInput{
		Name: "quota.txt", Reader: strings.NewReader("0123456789"), Metadata: map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, saved.ID))

	// Trashed bytes still count against the quota.
	quota, err := store.Quota(ctx, "blog")
	require.NoError(t, err)
	assert.EqualValues(t, 10, quota.UsedBytes)

	_, err = store.Put(ctx, UploadInput{
		Name: "blocked.txt", Reader: strings.NewReader("x"), Metadata: map[string]string{"owner": "blog"},
	})
	assert.ErrorIs(t, err, ErrQuotaExceeded)

	purged, err := store.PurgeTrash(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, purged)

	quota, err = store.Quota(ctx, "blog")
	require.NoError(t, err)
	assert.EqualValues(t, 0, quota.UsedBytes)
	_, err = store.Put(ctx, UploadInput{
		Name: "allowed.txt", Reader: strings.NewReader("x"), Metadata: map[string]string{"owner": "blog"},
	})
	require.NoError(t, err)
}

// TestDiskStoreUsageIndexTracksMutations checks that the cached quota index
// stays correct across upload, resumable upload, complete, abort and restore.
func TestDiskStoreUsageIndexTracksMutations(t *testing.T) {
	store, _, _ := newRecoverableStore(t, Options{OwnerQuotaBytes: 1 << 20})
	ctx := context.Background()
	owner := map[string]string{"owner": "blog"}

	used := func() int64 {
		quota, err := store.Quota(ctx, "blog")
		require.NoError(t, err)
		return quota.UsedBytes
	}

	assert.EqualValues(t, 0, used())

	uploaded, err := store.Put(ctx, UploadInput{Name: "a.txt", Reader: strings.NewReader("12345"), Metadata: owner})
	require.NoError(t, err)
	assert.EqualValues(t, 5, used())

	session, err := store.StartUpload(ctx, MultipartStartInput{Name: "b.bin", Size: 100, Metadata: owner})
	require.NoError(t, err)
	assert.EqualValues(t, 105, used(), "declared size must be reserved")

	_, err = store.AppendUpload(ctx, session.ID, 0, 100, strings.NewReader(strings.Repeat("b", 100)), "")
	require.NoError(t, err)
	completed, err := store.CompleteUpload(ctx, session.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 105, used(), "reservation becomes a real object")

	aborted, err := store.StartUpload(ctx, MultipartStartInput{Name: "c.bin", Size: 50, Metadata: owner})
	require.NoError(t, err)
	assert.EqualValues(t, 155, used())
	require.NoError(t, store.AbortUpload(ctx, aborted.ID))
	assert.EqualValues(t, 105, used(), "abort releases the reservation")

	require.NoError(t, store.Delete(ctx, uploaded.ID))
	assert.EqualValues(t, 105, used(), "trash still counts")

	_, err = store.Restore(ctx, uploaded.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 105, used())

	require.NoError(t, store.Delete(ctx, completed.ID))
	_, err = store.PurgeTrash(ctx, time.Time{})
	require.NoError(t, err)
	assert.EqualValues(t, 5, used(), "purging the completed object leaves the restored one")

	// Deleting the restored object and purging returns the owner to zero.
	require.NoError(t, store.Delete(ctx, uploaded.ID))
	_, err = store.PurgeTrash(ctx, time.Time{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, used())

	require.NoError(t, store.RebuildUsage(ctx))
	assert.EqualValues(t, 0, used())
}

// TestDiskStoreCountReportsActiveObjects covers the pagination total.
func TestDiskStoreCountReportsActiveObjects(t *testing.T) {
	store, _, _ := newRecoverableStore(t, Options{})
	ctx := context.Background()

	for _, name := range []string{"one", "two", "three"} {
		_, err := store.Put(ctx, UploadInput{Name: name + ".txt", Reader: strings.NewReader(name)})
		require.NoError(t, err)
	}
	trashed, err := store.Put(ctx, UploadInput{Name: "gone.txt", Reader: strings.NewReader("gone")})
	require.NoError(t, err)
	require.NoError(t, store.Delete(ctx, trashed.ID))

	count, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "trashed objects are not active")
}

func TestDiskStoreReadyRejectsUnknownQuarantineGroup(t *testing.T) {
	store, root, _ := newRecoverableStore(t, Options{})
	require.NoError(t, os.MkdirAll(filepath.Join(root, "quarantine", "surprise"), 0o750))
	assert.ErrorIs(t, store.Ready(context.Background()), ErrCorruptMetadata)

	require.NoError(t, os.RemoveAll(filepath.Join(root, "quarantine", "surprise")))
	require.NoError(t, os.WriteFile(filepath.Join(root, "quarantine", "loose.txt"), []byte("x"), 0o640))
	assert.ErrorIs(t, store.Ready(context.Background()), ErrCorruptMetadata)
}

// TestDiskStoreRecoverWithTTLIsReported keeps the two repair paths observable.
func TestDiskStoreRecoverWithTTLIsReported(t *testing.T) {
	store, _, now := newRecoverableStore(t, Options{UploadTTL: time.Minute})
	ctx := context.Background()

	session, err := store.StartUpload(ctx, MultipartStartInput{Name: "stale.bin", Size: 128})
	require.NoError(t, err)

	// Inside the TTL the session survives.
	report, err := store.Recover(ctx)
	require.NoError(t, err)
	assert.Zero(t, report.ExpiredUploads)

	now.Advance(2 * time.Minute)
	report, err = store.Recover(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, report.ExpiredUploads)

	sessions, err := store.ListUploads(ctx)
	require.NoError(t, err)
	assert.Empty(t, sessions)
	_, err = store.GetUpload(ctx, session.ID)
	assert.ErrorIs(t, err, ErrUploadNotFound)
}

// TestDiskStoreCapacityCountsEveryRecord guards the accounting against
// aggregating records per owner: the index is keyed by owner, so counting map
// keys reports the number of owners instead of the number of objects.
func TestDiskStoreCapacityCountsEveryRecord(t *testing.T) {
	store, _, _ := newRecoverableStore(t, Options{})
	ctx := context.Background()

	// Three objects under one owner plus two under another, plus one default.
	for index := 0; index < 3; index++ {
		_, err := store.Put(ctx, UploadInput{
			Name: "single-owner.txt", Reader: strings.NewReader("aaa"),
			Metadata: map[string]string{"owner": "blog"},
		})
		require.NoError(t, err)
	}
	for index := 0; index < 2; index++ {
		_, err := store.Put(ctx, UploadInput{
			Name: "other-owner.txt", Reader: strings.NewReader("bb"),
			Metadata: map[string]string{"owner": "media"},
		})
		require.NoError(t, err)
	}
	trashed, err := store.Put(ctx, UploadInput{Name: "anons.txt", Reader: strings.NewReader("c")})
	require.NoError(t, err)

	capacity, err := store.Capacity(ctx)
	require.NoError(t, err)
	assert.Equal(t, 6, capacity.ObjectCount, "one owner holding three objects is still three objects")
	assert.EqualValues(t, 3*3+2*2+1, capacity.ObjectBytes)

	// Trashing a record moves it, it does not duplicate or drop it.
	require.NoError(t, store.Delete(ctx, trashed.ID))
	capacity, err = store.Capacity(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, capacity.ObjectCount)
	assert.Equal(t, 1, capacity.TrashCount)
	assert.EqualValues(t, 1, capacity.TrashBytes)
	assert.EqualValues(t, 3*3+2*2, capacity.ObjectBytes)

	// Restoring returns it to the active side.
	_, err = store.Restore(ctx, trashed.ID)
	require.NoError(t, err)
	capacity, err = store.Capacity(ctx)
	require.NoError(t, err)
	assert.Equal(t, 6, capacity.ObjectCount)
	assert.Equal(t, 0, capacity.TrashCount)

	// Purging drops the record only where it was.
	require.NoError(t, store.Delete(ctx, trashed.ID))
	purged, err := store.PurgeTrash(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, purged)
	capacity, err = store.Capacity(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, capacity.ObjectCount)
	assert.Equal(t, 0, capacity.TrashCount)

	// A rebuild must agree with the incremental index.
	require.NoError(t, store.RebuildUsage(ctx))
	rebuilt, err := store.Capacity(ctx)
	require.NoError(t, err)
	assert.Equal(t, capacity.ObjectCount, rebuilt.ObjectCount)
	assert.Equal(t, capacity.ObjectBytes, rebuilt.ObjectBytes)
	assert.Equal(t, capacity.TrashCount, rebuilt.TrashCount)
}
