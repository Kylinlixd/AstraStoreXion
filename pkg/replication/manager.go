package replication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/files"
)

var ErrInvalidConfig = errors.New("invalid replication configuration")

type Source interface {
	Download(context.Context, string) (files.File, io.ReadCloser, error)
}

type Config struct {
	RemoteURL     string
	Token         string
	JobsDir       string
	Client        *http.Client
	MaxAttempts   int
	RetryInterval time.Duration
}

type Operation string

const (
	OperationUpload  Operation = "upload"
	OperationDelete  Operation = "delete"
	OperationRestore Operation = "restore"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobFailed    JobStatus = "failed"
	JobCompleted JobStatus = "completed"
)

type Job struct {
	ID        string    `json:"job_id"`
	Operation Operation `json:"operation"`
	FileID    string    `json:"file_id"`
	Status    JobStatus `json:"status"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Status struct {
	Enabled   bool `json:"enabled"`
	Pending   int  `json:"pending"`
	Running   int  `json:"running"`
	Failed    int  `json:"failed"`
	Completed int  `json:"completed"`
}

type Manager struct {
	source        Source
	remoteURL     string
	token         string
	jobsDir       string
	client        *http.Client
	maxAttempts   int
	retryInterval time.Duration

	mu   sync.Mutex
	jobs map[string]Job
	wake chan struct{}
	stop context.CancelFunc
	wg   sync.WaitGroup
}

func NewManager(source Source, config Config) (*Manager, error) {
	remoteURL := strings.TrimRight(strings.TrimSpace(config.RemoteURL), "/")
	parsed, err := url.Parse(remoteURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: replica URL must include scheme and host", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, fmt.Errorf("%w: replica token is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.JobsDir) == "" {
		return nil, fmt.Errorf("%w: jobs directory is required", ErrInvalidConfig)
	}
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	retryInterval := config.RetryInterval
	if retryInterval <= 0 {
		retryInterval = time.Second
	}
	if err := os.MkdirAll(config.JobsDir, 0o750); err != nil {
		return nil, fmt.Errorf("create replication jobs directory: %w", err)
	}
	manager := &Manager{
		source: source, remoteURL: remoteURL, token: config.Token, jobsDir: config.JobsDir,
		client: config.Client, maxAttempts: maxAttempts, retryInterval: retryInterval,
		jobs: make(map[string]Job), wake: make(chan struct{}, 1),
	}
	if manager.client == nil {
		manager.client = &http.Client{Timeout: 30 * time.Minute}
	}
	if err := manager.loadJobs(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Start() {
	m.mu.Lock()
	if m.stop != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.stop = cancel
	m.wg.Add(1)
	m.mu.Unlock()
	go m.worker(ctx)
}

func (m *Manager) Close() {
	m.mu.Lock()
	cancel := m.stop
	m.stop = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		m.wg.Wait()
	}
}

func (m *Manager) EnqueueUpload(ctx context.Context, fileID string) (Job, error) {
	return m.enqueue(ctx, OperationUpload, fileID)
}

func (m *Manager) EnqueueDelete(ctx context.Context, fileID string) (Job, error) {
	return m.enqueue(ctx, OperationDelete, fileID)
}

func (m *Manager) EnqueueRestore(ctx context.Context, fileID string) (Job, error) {
	return m.enqueue(ctx, OperationRestore, fileID)
}

func (m *Manager) enqueue(ctx context.Context, operation Operation, fileID string) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	if strings.TrimSpace(fileID) == "" {
		return Job{}, fmt.Errorf("%w: file id is required", ErrInvalidConfig)
	}
	jobID := string(operation) + "-" + fileID
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.jobs[jobID]; ok {
		if existing.Status == JobFailed {
			existing.Status = JobPending
			existing.Attempts = 0
			existing.LastError = ""
			existing.UpdatedAt = time.Now().UTC()
			if err := m.persistJob(existing); err != nil {
				return Job{}, err
			}
			m.jobs[jobID] = existing
			m.signal()
		}
		return existing, nil
	}
	now := time.Now().UTC()
	job := Job{ID: jobID, Operation: operation, FileID: fileID, Status: JobPending, CreatedAt: now, UpdatedAt: now}
	if err := m.persistJob(job); err != nil {
		return Job{}, err
	}
	m.jobs[jobID] = job
	m.signal()
	return job, nil
}

func (m *Manager) Retry(ctx context.Context, jobID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return os.ErrNotExist
	}
	job.Status = JobPending
	job.Attempts = 0
	job.LastError = ""
	job.UpdatedAt = time.Now().UTC()
	if err := m.persistJob(job); err != nil {
		return err
	}
	m.jobs[jobID] = job
	m.signal()
	return nil
}

func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.Before(jobs[j].CreatedAt) })
	return jobs
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{Enabled: true}
	for _, job := range m.jobs {
		switch job.Status {
		case JobPending:
			status.Pending++
		case JobRunning:
			status.Running++
		case JobFailed:
			status.Failed++
		case JobCompleted:
			status.Completed++
		}
	}
	return status
}

func (m *Manager) ProcessPending(ctx context.Context) error {
	job, ok, err := m.claimNext()
	if err != nil || !ok {
		return err
	}
	err = m.execute(ctx, job)
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.jobs[job.ID]
	if err != nil {
		current.Status = JobFailed
		current.Attempts++
		current.LastError = trimError(err)
	} else {
		current.Status = JobCompleted
		current.LastError = ""
	}
	current.UpdatedAt = time.Now().UTC()
	m.jobs[job.ID] = current
	if persistErr := m.persistJob(current); persistErr != nil && err == nil {
		return persistErr
	}
	return err
}

func (m *Manager) claimNext() (Job, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if (job.Status != JobPending && job.Status != JobFailed) || job.Attempts >= m.maxAttempts {
			continue
		}
		job.Status = JobRunning
		job.UpdatedAt = time.Now().UTC()
		m.jobs[job.ID] = job
		if err := m.persistJob(job); err != nil {
			return Job{}, false, err
		}
		return job, true, nil
	}
	return Job{}, false, nil
}

func (m *Manager) execute(ctx context.Context, job Job) error {
	switch job.Operation {
	case OperationUpload:
		return m.replicateUpload(ctx, job.FileID)
	case OperationDelete:
		return m.mutateRemote(ctx, http.MethodDelete, "/api/v1/files/"+url.PathEscape(job.FileID), nil, "", true)
	case OperationRestore:
		return m.mutateRemote(ctx, http.MethodPost, "/api/v1/files/"+url.PathEscape(job.FileID)+"/restore", nil, "", false)
	default:
		return fmt.Errorf("unknown replication operation %q", job.Operation)
	}
}

func (m *Manager) replicateUpload(ctx context.Context, fileID string) error {
	if m.source == nil {
		return errors.New("replication source is unavailable")
	}
	stored, body, err := m.source.Download(ctx, fileID)
	if err != nil {
		return fmt.Errorf("open source object: %w", err)
	}
	multipartBody, contentType := streamMultipart(stored, body)
	defer multipartBody.Close()
	return m.mutateRemote(ctx, http.MethodPost, "/api/v1/files", multipartBody, contentType, false)
}

func (m *Manager) mutateRemote(ctx context.Context, method, path string, body io.ReadCloser, contentType string, notFoundOK bool) error {
	if body != nil {
		defer body.Close()
	}
	request, err := http.NewRequestWithContext(ctx, method, m.remoteURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+m.token)
	request.Header.Set("X-Xion-Replication", "true")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := m.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if notFoundOK && response.StatusCode == http.StatusNotFound {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
	return fmt.Errorf("replica returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.retryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = m.ProcessPending(ctx), m.retryInterval
		case <-m.wake:
			_, _ = m.ProcessPending(ctx), m.retryInterval
		}
	}
}

func (m *Manager) loadJobs() error {
	entries, err := os.ReadDir(m.jobsDir)
	if err != nil {
		return fmt.Errorf("read replication jobs: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.jobsDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read replication job: %w", err)
		}
		var job Job
		if err := json.Unmarshal(data, &job); err != nil || job.ID == "" || job.FileID == "" {
			return fmt.Errorf("%w: invalid replication job %s", ErrInvalidConfig, entry.Name())
		}
		if job.Status == JobRunning {
			job.Status = JobPending
			job.UpdatedAt = time.Now().UTC()
			if err := m.persistJob(job); err != nil {
				return err
			}
		}
		m.jobs[job.ID] = job
	}
	return nil
}

func (m *Manager) persistJob(job Job) error {
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(m.jobsDir, job.ID+".json")
	temporary, err := os.CreateTemp(m.jobsDir, ".job-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o640); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return syncDirectory(m.jobsDir)
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) job(id string) Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

func streamMultipart(stored files.File, source io.ReadCloser) (io.ReadCloser, string) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	go func() {
		defer source.Close()
		part, err := multipartWriter.CreateFormFile("file", stored.Name)
		if err == nil {
			_, err = io.Copy(part, source)
		}
		if err == nil && stored.Metadata != nil {
			var metadataPart io.Writer
			metadataPart, err = multipartWriter.CreateFormField("metadata")
			if err == nil {
				encoded, encodeErr := json.Marshal(stored.Metadata)
				if encodeErr != nil {
					err = encodeErr
				} else {
					_, err = metadataPart.Write(encoded)
				}
			}
		}
		if err == nil {
			err = multipartWriter.Close()
		}
		if err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		_ = writer.Close()
	}()
	return reader, contentType
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
