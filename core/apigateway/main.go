package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/astrastore/astrastore-xion/pkg/replication"
)

const defaultMaxUploadBytes int64 = 1 << 30

// Build metadata, injected with -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	token := os.Getenv("XION_SERVICE_TOKEN")
	if token == "" {
		return errors.New("XION_SERVICE_TOKEN is required")
	}
	dataDirectory := envOrDefault("XION_DATA_DIR", "./data")
	var store *files.DiskStore
	var err error
	maxUploadBytes, err := envInt64("XION_MAX_UPLOAD_BYTES", defaultMaxUploadBytes)
	if err != nil || maxUploadBytes < 1 {
		return fmt.Errorf("XION_MAX_UPLOAD_BYTES must be a positive integer")
	}
	pauseAtPercent, err := envInt("XION_STORAGE_PAUSE_AT_PERCENT", 90)
	if err != nil || pauseAtPercent < 0 || pauseAtPercent > 100 {
		return fmt.Errorf("XION_STORAGE_PAUSE_AT_PERCENT must be between 0 and 100")
	}
	ownerQuotaBytes, err := envInt64("XION_OWNER_QUOTA_BYTES", 0)
	if err != nil || ownerQuotaBytes < 0 {
		return fmt.Errorf("XION_OWNER_QUOTA_BYTES must be zero or a positive integer")
	}
	uploadTTL, err := envDuration("XION_UPLOAD_SESSION_TTL", 0)
	if err != nil || uploadTTL < 0 {
		return fmt.Errorf("XION_UPLOAD_SESSION_TTL must be a non-negative duration such as 24h")
	}
	shutdownTimeout, err := envDuration("XION_SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil || shutdownTimeout <= 0 {
		return fmt.Errorf("XION_SHUTDOWN_TIMEOUT must be a positive duration")
	}
	store, err = files.NewDiskStoreWithOptions(dataDirectory, files.Options{
		PauseAtPercent:  pauseAtPercent,
		OwnerQuotaBytes: ownerQuotaBytes,
		UploadTTL:       uploadTTL,
	})
	if err != nil {
		return fmt.Errorf("initialize file store: %w", err)
	}
	service := files.NewService(store)

	// Repair crash leftovers before accepting traffic. Without this, a process
	// killed between writing a temporary manifest and renaming it would leave
	// the store permanently unready.
	recovery, err := service.Recover(ctx)
	if err != nil {
		return fmt.Errorf("recover storage: %w", err)
	}
	if recovery.Recovered() {
		log.Printf("recovered storage: partial_objects=%d metadata_manifests=%d upload_manifests=%d upload_sessions=%d expired_uploads=%d expired_trash=%d",
			recovery.PartialObjects, recovery.MetadataManifests, recovery.UploadManifests,
			recovery.UploadSessions, recovery.ExpiredUploads, recovery.ExpiredTrash)
	}

	var replicator *replication.Manager
	if replicaURL := strings.TrimSpace(os.Getenv("XION_REPLICA_URL")); replicaURL != "" {
		maxAttempts, parseErr := envInt("XION_REPLICATION_MAX_ATTEMPTS", 10)
		if parseErr != nil || maxAttempts < 1 {
			return fmt.Errorf("XION_REPLICATION_MAX_ATTEMPTS must be a positive integer")
		}
		jobsDir := envOrDefault("XION_REPLICATION_DIR", filepath.Join(dataDirectory, "replication"))
		replicator, err = replication.NewManager(service, replication.Config{
			RemoteURL: replicaURL, Token: os.Getenv("XION_REPLICA_TOKEN"),
			JobsDir: jobsDir, MaxAttempts: maxAttempts,
		})
		if err != nil {
			return fmt.Errorf("initialize replication: %w", err)
		}
		replicator.Start()
		defer replicator.Close()
		log.Printf("Xion replication enabled: %s", replicaURL)
	}
	address := envOrDefault("XION_LISTEN_ADDR", "127.0.0.1:8081")
	server := &http.Server{
		Addr:              address,
		Handler:           newRouter(gateway{service: service, token: token, maxUploadBytes: maxUploadBytes, replicator: replicator}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Minute,
		WriteTimeout:      30 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("AstraStoreXion %s (commit %s) listening on %s with data directory %s", version, commit, address, dataDirectory)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Printf("shutting down: waiting up to %s for in-flight requests", shutdownTimeout)
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown API server: %w", err)
		}
		return nil
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve API: %w", err)
	}
}

func envInt(name string, defaultValue int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(value)
}

func envOrDefault(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

func envInt64(name string, defaultValue int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func envDuration(name string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue, nil
	}
	return time.ParseDuration(value)
}

// Version reports the injected build metadata.
func Version() string {
	return version + " (" + commit + ")"
}
