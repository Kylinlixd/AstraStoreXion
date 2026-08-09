package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/files"
)

const defaultMaxUploadBytes int64 = 50 << 20

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
	store, err := files.NewDiskStore(dataDirectory)
	if err != nil {
		return fmt.Errorf("initialize file store: %w", err)
	}
	maxUploadBytes, err := envInt64("XION_MAX_UPLOAD_BYTES", defaultMaxUploadBytes)
	if err != nil || maxUploadBytes < 1 {
		return fmt.Errorf("XION_MAX_UPLOAD_BYTES must be a positive integer")
	}
	address := envOrDefault("XION_LISTEN_ADDR", "127.0.0.1:8081")
	server := &http.Server{
		Addr:              address,
		Handler:           newRouter(gateway{service: files.NewService(store), token: token, maxUploadBytes: maxUploadBytes}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("AstraStoreXion listening on %s with data directory %s", address, dataDirectory)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
