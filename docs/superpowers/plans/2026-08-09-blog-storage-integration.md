# Blog Storage Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver and deploy a persistent single-node AstraStoreXion service, connect it to the existing Django/Vue blog without breaking legacy media, and verify generated image/document uploads byte-for-byte.

**Architecture:** AstraStoreXion runs as a loopback-only Go service with atomic files and JSON manifests. Django remains the public/authenticated API and chooses a local or Xion storage adapter per database record; Vue only talks to Django and gains upload progress plus an embedded tutorial.

**Tech Stack:** Go 1.23+, Gorilla Mux, Python 3.12/requests, Django 5.1/DRF, Vue 3/Vite/Jest, systemd, Nginx.

---

## File map

### `/Users/leexd/AstraStoreXion`

- Modify `go.mod`, `go.sum`, `pkg/monitor/prometheus.go`: restore the Go build baseline.
- Create `pkg/files/model.go`, `pkg/files/store.go`, `pkg/files/service.go`: persistent atomic file lifecycle.
- Create `pkg/files/store_test.go`, `pkg/files/service_test.go`: persistence, checksum, traversal and compensation tests.
- Replace `core/apigateway/main.go`, `core/apigateway/handlers.go`: dependency-injected service-token HTTP API.
- Create `core/apigateway/handlers_test.go`: HTTP lifecycle, authorization and limits.
- Modify `client/python/astrastore_xion/{config,models,client}.py`: production SDK contract.
- Create `client/python/tests/test_client.py`: SDK request and streaming behavior.
- Create `deploy/systemd/astrastore-xion.service`, `deploy/systemd/astrastore-xion.env.example`: production unit and non-secret config template.
- Create `scripts/smoke-test.sh`: local/production byte-preserving lifecycle test.
- Modify `Makefile`, `README.md`, `docs/客户端使用文档.md`, `docs/operations/README.md`: build, tutorial, deployment and rollback.
- Create `test/fixtures/generated/*`: PNG, PDF, DOCX, TXT fixtures and checksum manifest.

### `/Users/leexd/blog_li`

- Modify `apps/upload/models.py`: storage backend/key/checksum/content type fields and stable API URL.
- Create `apps/upload/storage_backends.py`: local/Xion adapter boundary.
- Create `apps/upload/checks.py`: startup configuration checks.
- Create `apps/upload/migrations/0003_uploadfile_storage_fields.py`: legacy-safe schema migration.
- Modify `apps/upload/serializers.py`, `apps/upload/views.py`, `apps/upload/urls.py`: upload/download/delete through adapters.
- Replace `apps/upload/tests.py`: local/Xion routing, compensation, permissions and failure tests.
- Modify `blog/settings.py`, `docs/deployment.md`, `.env.example`: Xion feature flag and runbook.

### `/Users/leexd/myblog-admin`

- Modify `src/api/file.js`: upload progress callback and storage fields.
- Create `src/views/files/FileTutorialDrawer.vue`: five-step operator tutorial.
- Modify `src/views/files/FileList.vue`: responsive workbench, drag/drop progress and health badge.
- Modify `src/api/__tests__/file.spec.js`, `src/views/files/__tests__/FileList.spec.js`.
- Create `src/views/files/__tests__/FileTutorialDrawer.spec.js`.
- Modify `docs/DEV_GUIDE.md`, `docs/DEPLOY.md`.

### Task 1: Restore the Go build baseline

**Files:**
- Modify: `/Users/leexd/AstraStoreXion/pkg/monitor/prometheus.go`
- Create: `/Users/leexd/AstraStoreXion/pkg/monitor/prometheus_test.go`
- Modify: `/Users/leexd/AstraStoreXion/go.mod`
- Modify: `/Users/leexd/AstraStoreXion/go.sum`

- [ ] **Step 1: Write the failing interface regression test**

```go
func TestPrometheusMonitorUpdatesUnlabelledMetrics(t *testing.T) {
    mon := NewPrometheusMonitor(MonitorConfig{})
    require.NoError(t, mon.RegisterMetric(MetricDefinition{Name: "xion_test_total", Help: "test", Type: CounterMetric}))
    require.NoError(t, mon.RegisterMetric(MetricDefinition{Name: "xion_test_gauge", Help: "test", Type: GaugeMetric}))
    require.NoError(t, mon.RegisterMetric(MetricDefinition{Name: "xion_test_seconds", Help: "test", Type: HistogramMetric}))
    require.NoError(t, mon.IncCounter("xion_test_total", 1, nil))
    require.NoError(t, mon.SetGauge("xion_test_gauge", 2, nil))
    require.NoError(t, mon.Observe("xion_test_seconds", 0.25, nil))
}
```

- [ ] **Step 2: Prove the current code fails**

Run: `go test ./pkg/monitor -run TestPrometheusMonitorUpdatesUnlabelledMetrics -count=1`

Expected: compilation fails because pointers to Prometheus interfaces do not expose `Add`, `Set`, or `Observe`.

- [ ] **Step 3: Store and assert Prometheus interfaces directly**

Use `metric.(prometheus.Counter)`, `metric.(prometheus.Gauge)`, `metric.(prometheus.Histogram)`, and `metric.(prometheus.Summary)`. Run `go mod tidy` to record the already imported gRPC dependency.

- [ ] **Step 4: Verify the repaired baseline**

Run: `go test ./pkg/monitor -count=1 && go test ./... && go vet ./...`

Expected: all packages compile and all existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum pkg/monitor/prometheus.go pkg/monitor/prometheus_test.go
git commit -m "fix: restore Go build baseline"
```

### Task 2: Build the persistent atomic file service

**Files:**
- Create: `/Users/leexd/AstraStoreXion/pkg/files/model.go`
- Create: `/Users/leexd/AstraStoreXion/pkg/files/store.go`
- Create: `/Users/leexd/AstraStoreXion/pkg/files/service.go`
- Create: `/Users/leexd/AstraStoreXion/pkg/files/store_test.go`
- Create: `/Users/leexd/AstraStoreXion/pkg/files/service_test.go`

- [ ] **Step 1: Write failing persistence and safety tests**

```go
func TestDiskStorePersistsAcrossRestart(t *testing.T) {
    root := t.TempDir()
    first, err := NewDiskStore(root)
    require.NoError(t, err)
    saved, err := first.Put(context.Background(), UploadInput{
        Name: "hello.txt", ContentType: "text/plain", Reader: strings.NewReader("hello"),
    })
    require.NoError(t, err)

    second, err := NewDiskStore(root)
    require.NoError(t, err)
    got, body, err := second.Open(context.Background(), saved.ID)
    require.NoError(t, err)
    defer body.Close()
    bytes, err := io.ReadAll(body)
    require.NoError(t, err)
    assert.Equal(t, saved.Checksum, got.Checksum)
    assert.Equal(t, []byte("hello"), bytes)
}

func TestDiskStoreRejectsUnsafeID(t *testing.T) {
    store, err := NewDiskStore(t.TempDir())
    require.NoError(t, err)
    _, _, err = store.Open(context.Background(), "../outside")
    require.ErrorIs(t, err, ErrInvalidID)
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./pkg/files -run 'TestDiskStore(PersistsAcrossRestart|RejectsUnsafeID)' -count=1`

Expected: package or symbols do not exist.

- [ ] **Step 3: Implement the smallest persistent store**

Define these contracts:

```go
type File struct {
    ID string `json:"file_id"`
    Name string `json:"filename"`
    ContentType string `json:"content_type"`
    Size int64 `json:"size"`
    Checksum string `json:"checksum"`
    CreatedAt time.Time `json:"created_at"`
    Metadata map[string]string `json:"metadata,omitempty"`
}

type UploadInput struct {
    Name string
    ContentType string
    Metadata map[string]string
    Reader io.Reader
}

type Store interface {
    Put(context.Context, UploadInput) (File, error)
    Open(context.Context, string) (File, io.ReadCloser, error)
    Get(context.Context, string) (File, error)
    List(context.Context, int, int) ([]File, error)
    Delete(context.Context, string) error
    Ready(context.Context) error
}
```

`DiskStore.Put` writes to `<root>/tmp`, calculates SHA-256, syncs, renames into `<root>/objects/<id>`, then atomically writes `<root>/metadata/<id>.json`. If manifest creation fails, remove the object. IDs must match `^[a-f0-9-]{36}$`; list sort is `CreatedAt DESC, ID ASC`; returned maps are cloned.

- [ ] **Step 4: Add lifecycle, rollback, missing-object, list-order and idempotent-delete tests**

Run: `go test ./pkg/files -count=1 -race`

Expected: all store tests pass and no race is reported.

- [ ] **Step 5: Commit**

```bash
git add pkg/files
git commit -m "feat: add persistent atomic file service"
```

### Task 3: Replace the mock gateway with the service-token HTTP API

**Files:**
- Modify: `/Users/leexd/AstraStoreXion/core/apigateway/main.go`
- Modify: `/Users/leexd/AstraStoreXion/core/apigateway/handlers.go`
- Create: `/Users/leexd/AstraStoreXion/core/apigateway/handlers_test.go`

- [ ] **Step 1: Write a failing authenticated lifecycle test**

```go
func TestFileLifecycle(t *testing.T) {
    router := newTestRouter(t, "test-token", 1024)
    upload := multipartUpload(t, "/api/v1/files", "hello.txt", "text/plain", []byte("hello"))
    upload.Header.Set("Authorization", "Bearer test-token")
    created := doJSON[files.File](t, router, upload, http.StatusCreated)
    assert.Equal(t, sha256Hex("hello"), created.Checksum)

    download := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+created.ID, nil)
    download.Header.Set("Authorization", "Bearer test-token")
    response := httptest.NewRecorder()
    router.ServeHTTP(response, download)
    assert.Equal(t, http.StatusOK, response.Code)
    assert.Equal(t, "hello", response.Body.String())
}
```

- [ ] **Step 2: Verify RED against the mock handlers**

Run: `go test ./core/apigateway -run TestFileLifecycle -count=1`

Expected: upload is not persisted and download returns simulated Chinese text.

- [ ] **Step 3: Implement dependency-injected routes**

Create `gateway{store files.Store, token string, maxUploadBytes int64}` and `newRouter(gateway)`. Register health, readiness, upload, download, status, list and delete. Validate Bearer tokens with `subtle.ConstantTimeCompare`, enforce `http.MaxBytesReader`, use safe RFC 5987 content disposition, and emit:

```json
{"error":{"code":"unauthorized","message":"missing or invalid service token"}}
```

`main` reads `XION_LISTEN_ADDR`, `XION_DATA_DIR`, `XION_SERVICE_TOKEN`, and `XION_MAX_UPLOAD_BYTES`; empty service token terminates startup.

- [ ] **Step 4: Add authorization, 413, status, list, delete, 404 and readiness tests**

Run: `go test ./core/apigateway -count=1 -race && go test ./... && go vet ./...`

Expected: the full repository passes.

- [ ] **Step 5: Commit**

```bash
git add core/apigateway
git commit -m "feat: serve the real file lifecycle API"
```

### Task 4: Make the Python SDK the production bridge

**Files:**
- Modify: `/Users/leexd/AstraStoreXion/client/python/astrastore_xion/config.py`
- Modify: `/Users/leexd/AstraStoreXion/client/python/astrastore_xion/models.py`
- Modify: `/Users/leexd/AstraStoreXion/client/python/astrastore_xion/client.py`
- Create: `/Users/leexd/AstraStoreXion/client/python/tests/test_client.py`
- Modify: `/Users/leexd/AstraStoreXion/client/python/setup.py`

- [ ] **Step 1: Write failing SDK contract tests**

```python
def test_upload_sends_service_token_and_maps_real_fields(requests_mock):
    requests_mock.post(
        "http://xion/api/v1/files",
        json={"file_id": "abc", "filename": "a.txt", "size": 1,
              "content_type": "text/plain", "checksum": "00", "created_at": "2026-08-09T00:00:00Z"},
        status_code=201,
    )
    client = XionClient(XionConfig(api_gateway="http://xion", service_token="secret"))
    result = client.upload_file(io.BytesIO(b"a"), "a.txt")
    assert result.file_id == "abc"
    assert requests_mock.last_request.headers["Authorization"] == "Bearer secret"

def test_upload_is_not_retried(requests_mock):
    requests_mock.post("http://xion/api/v1/files", status_code=503)
    client = XionClient(XionConfig(api_gateway="http://xion", service_token="secret", max_retries=3))
    with pytest.raises(XionHTTPError):
        client.upload_file(io.BytesIO(b"a"), "a.txt")
    assert requests_mock.call_count == 1
```

- [ ] **Step 2: Verify RED**

Run: `python -m pytest client/python/tests/test_client.py -q`

Expected: config lacks `service_token`, models do not map real fields, and errors are unstructured.

- [ ] **Step 3: Implement headers, response models and safe retries**

Add `XionError`, `XionHTTPError(status_code, code, message)`, and `XionUnavailableError`. Add auth headers to every request. Stream downloads with `iter_content`; retry GET/DELETE on connection errors and 502/503/504 with bounded backoff; never retry POST upload. Add `list_files` and `health`.

- [ ] **Step 4: Verify SDK**

Run: `python -m pytest client/python/tests -q && python -m compileall -q client/python/astrastore_xion`

Expected: all SDK tests pass.

- [ ] **Step 5: Commit**

```bash
git add client/python
git commit -m "feat: harden the Python storage client"
```

### Task 5: Add the Django dual-storage boundary

**Files:**
- Clone: `/Users/leexd/blog_li`
- Modify: `/Users/leexd/blog_li/apps/upload/models.py`
- Create: `/Users/leexd/blog_li/apps/upload/storage_backends.py`
- Create: `/Users/leexd/blog_li/apps/upload/checks.py`
- Create: `/Users/leexd/blog_li/apps/upload/migrations/0003_uploadfile_storage_fields.py`
- Modify: `/Users/leexd/blog_li/blog/settings.py`
- Modify: `/Users/leexd/blog_li/apps/upload/apps.py`
- Replace: `/Users/leexd/blog_li/apps/upload/tests.py`

- [ ] **Step 1: Clone and branch the backend**

Run: `git clone https://github.com/Kylinlixd/blog_li.git /Users/leexd/blog_li && git -C /Users/leexd/blog_li switch -c codex/xion-storage-integration`

Expected: clean branch based on the current remote main.

- [ ] **Step 2: Write failing adapter/model tests**

```python
class StorageBackendTests(TestCase):
    @override_settings(XION_STORAGE_ENABLED=True, XION_BASE_URL="http://xion", XION_SERVICE_TOKEN="token")
    @patch("apps.upload.storage_backends.XionClient")
    def test_xion_backend_returns_stable_blog_url(self, client_type):
        client_type.return_value.upload_file.return_value = SimpleNamespace(
            file_id="xion-id", checksum="abc", size=3, content_type="text/plain"
        )
        stored = get_storage_backend().save(SimpleUploadedFile("a.txt", b"abc", content_type="text/plain"))
        self.assertEqual(stored.storage_key, "xion-id")
        self.assertEqual(stored.checksum, "abc")

    def test_legacy_rows_default_to_local(self):
        field = UploadFile._meta.get_field("storage_backend")
        self.assertEqual(field.default, "local")
```

- [ ] **Step 3: Verify RED**

Run: `/Users/leexd/blog_li/.venv/bin/python manage.py test apps.upload -v 2`

Expected: storage fields and adapter module are absent.

- [ ] **Step 4: Implement the model, migration, adapters and system check**

Define `StoredObject(storage_backend, storage_key, size, checksum, content_type)` and `StorageBackend.save/open/delete`. `LocalStorageBackend` contains existing safe filesystem behavior. `XionStorageBackend` imports the checked-out SDK during development and installed package in production. A Django check emits `upload.E001` when Xion is enabled without URL/token.

The migration adds nullable key/checksum/content type plus `storage_backend=models.CharField(max_length=16, default="local")`; it does not rewrite existing `file_url` values.

- [ ] **Step 5: Verify model and configuration**

Run: `python manage.py makemigrations --check --dry-run && python manage.py test apps.upload -v 2 && python manage.py check`

Expected: no missing migrations; adapter tests and system checks pass.

- [ ] **Step 6: Commit**

```bash
git add apps/upload blog/settings.py
git commit -m "feat: add dual file storage adapters"
```

### Task 6: Route Django upload, download and delete through adapters

**Files:**
- Modify: `/Users/leexd/blog_li/apps/upload/serializers.py`
- Modify: `/Users/leexd/blog_li/apps/upload/views.py`
- Modify: `/Users/leexd/blog_li/apps/upload/urls.py`
- Modify: `/Users/leexd/blog_li/apps/upload/tests.py`

- [ ] **Step 1: Write failing integration/compensation tests**

```python
@patch("apps.upload.views.get_storage_backend")
def test_database_failure_deletes_new_xion_object(self, backend_factory):
    backend = backend_factory.return_value
    backend.save.return_value = StoredObject("xion", "key-1", 3, "abc", "text/plain")
    with patch("apps.upload.views.UploadFile.objects.create", side_effect=DatabaseError("down")):
        response = self.client.post("/api/upload/upload/", {"file": self.sample, "file_type": "document"})
    self.assertEqual(response.status_code, 500)
    backend.delete.assert_called_once_with("key-1")

@patch("apps.upload.views.backend_for_file")
def test_delete_failure_keeps_database_row(self, backend_factory):
    backend_factory.return_value.delete.side_effect = StorageUnavailable("down")
    response = self.client.delete(f"/api/upload/files/{self.file.id}/")
    self.assertEqual(response.status_code, 503)
    self.assertTrue(UploadFile.objects.filter(id=self.file.id).exists())
```

- [ ] **Step 2: Verify RED**

Run: `python manage.py test apps.upload -v 2`

Expected: current views directly write/delete `MEDIA_ROOT`, so routing and compensation assertions fail.

- [ ] **Step 3: Implement transactional views and stable download URL**

The upload view validates the stream, calls `backend.save`, then creates the row inside `transaction.atomic`; on database failure it calls `backend.delete`. `file_url` becomes `/api/upload/public/<id>/` for public files and `/api/upload/files/<id>/download/` for private files. The existing `download` action and a public GET endpoint stream from `backend.open` with `FileResponse`.

Destroy calls backend delete before `perform_destroy`; `StorageUnavailable` maps to 503 and preserves the row; missing physical data is treated as deleted. Serializers expose `storage_backend`, `checksum`, and `content_type` read-only.

- [ ] **Step 4: Run focused and full backend verification**

Run: `python manage.py check && python manage.py makemigrations --check --dry-run && python manage.py test apps.upload -v 2 && python manage.py test`

Expected: all backend tests pass.

- [ ] **Step 5: Commit**

```bash
git add apps/upload
git commit -m "feat: route blog files through storage adapters"
```

### Task 7: Upgrade the Vue file workbench and tutorial

**Files:**
- Clone: `/Users/leexd/myblog-admin`
- Modify: `/Users/leexd/myblog-admin/src/api/file.js`
- Create: `/Users/leexd/myblog-admin/src/views/files/FileTutorialDrawer.vue`
- Modify: `/Users/leexd/myblog-admin/src/views/files/FileList.vue`
- Modify: `/Users/leexd/myblog-admin/src/api/__tests__/file.spec.js`
- Modify: `/Users/leexd/myblog-admin/src/views/files/__tests__/FileList.spec.js`
- Create: `/Users/leexd/myblog-admin/src/views/files/__tests__/FileTutorialDrawer.spec.js`

- [ ] **Step 1: Clone and branch the frontend**

Run: `git clone https://github.com/Kylinlixd/myblog-admin.git /Users/leexd/myblog-admin && git -C /Users/leexd/myblog-admin switch -c codex/xion-storage-workbench`

Expected: clean branch based on remote main.

- [ ] **Step 2: Write failing API progress and tutorial tests**

```javascript
it('forwards upload progress and normalizes storage fields', async () => {
  jest.spyOn(request, 'post').mockImplementation((_url, _data, options) => {
    options.onUploadProgress({ loaded: 5, total: 10 })
    return Promise.resolve({ data: { id: 1, file_type: 'document', file_size: 10,
      file_url: '/api/upload/public/1/', storage_backend: 'xion', checksum: 'abc' } })
  })
  const progress = jest.fn()
  const result = await uploadFile({ file: new File(['x'], 'a.pdf'), file_type: 'document', onProgress: progress })
  expect(progress).toHaveBeenCalledWith(50)
  expect(result.storage_backend).toBe('xion')
})

it('documents the complete operator workflow', () => {
  const wrapper = mount(FileTutorialDrawer, { props: { open: true } })
  for (const text of ['上传文件', '复制链接', '插入文章', '下载与删除', '常见问题']) {
    expect(wrapper.text()).toContain(text)
  }
})
```

- [ ] **Step 3: Verify RED**

Run: `npm test -- --runInBand src/api/__tests__/file.spec.js src/views/files/__tests__/FileTutorialDrawer.spec.js`

Expected: progress is ignored and the tutorial component is absent.

- [ ] **Step 4: Implement API progress and tutorial**

Pass Axios `onUploadProgress` and calculate `Math.round(loaded * 100 / total)`. The tutorial drawer contains five concise steps, accepted types and maximum sizes, plus retry guidance; it emits `update:open` and stores only a dismissed-hint boolean in local storage.

- [ ] **Step 5: Implement the responsive workbench**

Add storage badge, tutorial button, drag/drop copy, current upload name/progress, MIME-aware type inference (`pdf/doc/docx/xls/xlsx` → `document`), a 50MiB client limit matching Nginx/Django/Xion, and mobile cards below 768px. Keep existing batch delete, search and async states.

- [ ] **Step 6: Verify focused tests, lint and build**

Run: `npm test -- --runInBand src/api/__tests__/file.spec.js src/views/files/__tests__/FileList.spec.js src/views/files/__tests__/FileTutorialDrawer.spec.js && npm run lint && npm run build`

Expected: focused tests pass; lint and production build exit zero.

- [ ] **Step 7: Commit**

```bash
git add src/api/file.js src/views/files docs
git commit -m "feat: add the storage file workbench tutorial"
```

### Task 8: Add systemd deployment, fixtures and documentation

**Files:**
- Create: `/Users/leexd/AstraStoreXion/deploy/systemd/astrastore-xion.service`
- Create: `/Users/leexd/AstraStoreXion/deploy/systemd/astrastore-xion.env.example`
- Create: `/Users/leexd/AstraStoreXion/scripts/smoke-test.sh`
- Modify: `/Users/leexd/AstraStoreXion/Makefile`
- Modify: `/Users/leexd/AstraStoreXion/README.md`
- Modify: `/Users/leexd/AstraStoreXion/docs/客户端使用文档.md`
- Modify: `/Users/leexd/AstraStoreXion/docs/operations/README.md`
- Create: `/Users/leexd/AstraStoreXion/test/fixtures/generated/astrastore-test-cover.png`
- Create: `/Users/leexd/AstraStoreXion/test/fixtures/generated/AstraStoreXion-使用手册.pdf`
- Create: `/Users/leexd/AstraStoreXion/test/fixtures/generated/AstraStoreXion-部署检查单.docx`
- Create: `/Users/leexd/AstraStoreXion/test/fixtures/generated/utf8-sample.txt`
- Create: `/Users/leexd/AstraStoreXion/test/fixtures/generated/manifest.json`
- Modify: `/Users/leexd/blog_li/.env.example`
- Modify: `/Users/leexd/blog_li/docs/deployment.md`
- Modify: `/Users/leexd/myblog-admin/docs/DEV_GUIDE.md`
- Modify: `/Users/leexd/myblog-admin/docs/DEPLOY.md`

- [ ] **Step 1: Write the smoke test before deployment configuration**

`scripts/smoke-test.sh` requires `XION_BASE_URL`, `XION_SERVICE_TOKEN`, and fixture path; it uploads, reads the returned ID, downloads to a temporary directory, compares `sha256sum`/`shasum -a 256`, checks status, then deletes. It must use `mktemp -d` and a trap that removes only that exact temporary directory.

- [ ] **Step 2: Verify the smoke test fails while no service is running**

Run: `XION_BASE_URL=http://127.0.0.1:18081 XION_SERVICE_TOKEN=test ./scripts/smoke-test.sh test/fixtures/generated/utf8-sample.txt`

Expected: readiness connection fails before upload.

- [ ] **Step 3: Generate and visually verify fixtures using the applicable image/PDF/document skills**

Create valid PNG, PDF and DOCX artifacts plus UTF-8 text. Render PDF and DOCX to page images and inspect them. Generate `manifest.json` from actual MIME, byte size and SHA-256 values.

- [ ] **Step 4: Add the least-privilege systemd unit**

The unit uses `User=astrastore-xion`, `Group=astrastore-xion`, `EnvironmentFile=/etc/astrastore-xion.env`, `ExecStart=/usr/local/bin/astrastore-xion`, `StateDirectory=astrastore-xion`, `NoNewPrivileges=true`, `PrivateTmp=true`, `ProtectSystem=strict`, and `ReadWritePaths=/var/lib/astrastore-xion`. The example env contains names and safe defaults but no token.

- [ ] **Step 5: Document operator and developer workflows**

Document five-minute local start, service-token handling, curl and Python examples, blog feature flag, backup, enable order, rollback order, sample upload verification, and the explicit single-node limitation.

- [ ] **Step 6: Verify docs/config and commit per repository**

Run in AstraStoreXion: `git diff --check && go test ./... && go vet ./...`

Run in blog backend: `python manage.py check && python manage.py test`

Run in frontend: `npm run check`

Expected: all commands exit zero.

Commit documentation and deployment files in their owning repositories with `docs: document Xion storage operations`.

### Task 9: Run local cross-repository end-to-end verification

**Files:**
- No product file changes unless verification exposes a defect.

- [ ] **Step 1: Start Xion with an isolated persistent directory**

Run with a generated non-production token and `XION_LISTEN_ADDR=127.0.0.1:18081`. Keep the process in a managed terminal session and record its PID/session, not the token.

- [ ] **Step 2: Run every generated fixture through the direct Xion smoke test**

Run `scripts/smoke-test.sh` once for PNG, PDF, DOCX and TXT.

Expected: each downloaded SHA-256 equals the local file and each cleanup delete succeeds.

- [ ] **Step 3: Start Django with Xion enabled against a test database**

Run migrations, create an authenticated test client through Django tests, upload all four fixtures through `/api/upload/upload/`, download through stable blog URLs, compare bytes and delete.

- [ ] **Step 4: Run complete repository gates**

```bash
# AstraStoreXion
git diff --check && go test ./... -count=1 && go test ./... -race -count=1 && go vet ./...

# blog_li
python manage.py check && python manage.py check --deploy && python manage.py makemigrations --check --dry-run && python manage.py test

# myblog-admin
npm run check
```

Expected: every command exits zero; no fixtures or credentials appear outside the intended fixture directory.

### Task 10: Deploy safely and run production smoke tests

**Files:**
- Remote: `/usr/local/bin/astrastore-xion`
- Remote: `/etc/systemd/system/astrastore-xion.service`
- Remote: `/etc/astrastore-xion.env`
- Remote: `/var/lib/astrastore-xion/`
- Remote: `/opt/blog_li/`
- Remote: `/var/www/myblog-admin/releases/<commit>/`

- [ ] **Step 1: Capture pre-deploy state and backups**

Record current HTTP status, active systemd unit state, frontend `current` target and database migration state. Run the existing blog backup job or create a timestamped MySQL/code backup outside the live paths. Do not print `.env` or database credentials.

- [ ] **Step 2: Build and install Xion without public exposure**

Cross-build `GOOS=linux GOARCH=amd64` from verified source. Create the system user/state directory, install binary/unit, generate a 32-byte token directly on the server into mode-600 `/etc/astrastore-xion.env`, start the service, and verify `ss` shows only `127.0.0.1:8081`.

- [ ] **Step 3: Deploy Django with the feature flag disabled**

Sync code while preserving `/opt/blog_li/.env`, `.venv`, `media` and user data. Install/update Python dependencies, run `manage.py check`, backup MySQL, run migration 0003, restart Gunicorn, and verify public blog, login endpoint and legacy media.

- [ ] **Step 4: Deploy the frontend atomically**

Build the verified commit, upload to a new `/var/www/myblog-admin/releases/<commit>`, point `current` to the new release atomically, run `nginx -t`, reload Nginx, and check `/blog`, `/login`, assets and API.

- [ ] **Step 5: Enable Xion writes and execute production fixture lifecycle**

Add Xion URL/token/flag to the protected Django env without exposing values, restart Django, then upload the four dedicated fixtures through the authenticated blog API. Download and compare SHA-256, confirm new rows say `storage_backend=xion`, restart both Xion and Django, download again, then delete only those dedicated test records.

- [ ] **Step 6: Verify rollback controls and final health**

Confirm the flag can be disabled without losing reads for existing Xion rows, restore it, then check `systemctl is-active`, `journalctl` for new errors, Nginx error log, HTTP status, loopback-only port binding and disk usage.

- [ ] **Step 7: Report exact deployed commits and residual risks**

List the three local commit IDs, server release paths, verification counts and the single-node/no-historical-migration limitations. Never include credentials or token values.

## Plan self-review

- Spec coverage: tasks 1–4 deliver the real storage service and SDK; tasks 5–6 preserve Django ownership and legacy media; task 7 delivers the client/tutorial; task 8 covers artifacts and operations; tasks 9–10 prove local and production behavior including restart and rollback.
- Placeholder scan: every behavior-changing task names concrete files, failing tests, implementation contracts, verification commands and expected outcomes. Production secrets are intentionally generated server-side, never represented as plan values.
- Type consistency: Go uses `files.File` and `files.Store`; Python uses matching `file_id/filename/content_type/size/checksum/created_at`; Django persists `storage_backend/storage_key/checksum/content_type`; Vue consumes only the stable Django `file_url` plus read-only storage fields.
