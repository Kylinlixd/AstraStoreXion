# Xion Resumable Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent sequential resumable-upload API without breaking the existing one-shot upload contract.

**Architecture:** Keep the current `DiskStore` as the source of truth and add a separate upload-session directory. A session appends bytes to a private `.part` file, atomically advances a JSON manifest, and only creates a normal Xion object during completion. The HTTP gateway exposes the session lifecycle while preserving the existing `fileService` interface through an optional multipart capability.

**Tech Stack:** Go 1.23+, `net/http`, Gorilla mux, standard library JSON/crypto/filesystem APIs, testify.

---

### Task 1: Define upload-session domain types and store capability

**Files:**
- Modify: `pkg/files/model.go`
- Modify: `pkg/files/store.go`
- Modify: `pkg/files/service.go`
- Test: `pkg/files/resumable_test.go`

- [ ] **Step 1: Write failing tests for session creation and state persistence**

  Add tests that create a session with a declared size, assert `uploading` and `received_bytes=0`, append one chunk, reopen the store, and assert the offset is retained.

- [ ] **Step 2: Run the focused test and verify the expected missing-method failure**

  Run `go test ./pkg/files -run 'TestDiskStoreResumable' -count=1`.

- [ ] **Step 3: Add the domain types, errors, and optional store interface**

  Add `UploadSession`, `MultipartStartInput`, `ErrUploadNotFound`, `ErrUploadOffsetConflict`, `ErrUploadIncomplete`, and `ErrUploadChecksumMismatch`. Extend `DiskStore` with `uploadsDir` and implement `MultipartStore` methods `StartUpload`, `GetUpload`, `AppendUpload`, `CompleteUpload`, and `AbortUpload`.

- [ ] **Step 4: Run the focused tests and confirm they pass**

  Run `go test ./pkg/files -run 'TestDiskStoreResumable' -count=1`.

- [ ] **Step 5: Commit the storage-session implementation**

  Run `git add pkg/files/model.go pkg/files/store.go pkg/files/service.go pkg/files/resumable_test.go && git commit -m "feat: add persistent resumable upload sessions"`.

### Task 2: Add HTTP session lifecycle endpoints

**Files:**
- Modify: `core/apigateway/handlers.go`
- Modify: `core/apigateway/handlers_test.go`

- [ ] **Step 1: Write failing HTTP tests**

  Cover `POST /api/v1/uploads`, `GET /api/v1/uploads/{id}`, sequential `PUT` with `Content-Range`, completion, deletion, unauthorized requests, offset conflicts, and incomplete completion.

- [ ] **Step 2: Run the focused HTTP tests and verify they fail because routes are absent**

  Run `go test ./core/apigateway -run 'TestResumableUpload' -count=1`.

- [ ] **Step 3: Implement the optional multipart gateway capability and routes**

  Add a `multipartService` type assertion so existing fake services remain valid. Parse JSON start requests with a bounded body, enforce `Content-Range`, stream the request body into `AppendUpload`, and map domain errors to `400`, `404`, `409`, `422`, `507`, and `413` responses. Add `POST`, `GET`, `PUT`, `POST /complete`, and `DELETE` routes under `/api/v1/uploads`.

- [ ] **Step 4: Run the focused HTTP tests and the existing gateway tests**

  Run `go test ./core/apigateway -run 'TestResumableUpload|TestFileLifecycle|TestUploadRejectsFilesAboveConfiguredLimit' -count=1`.

- [ ] **Step 5: Commit the API implementation**

  Run `git add core/apigateway/handlers.go core/apigateway/handlers_test.go && git commit -m "feat: expose resumable upload API"`.

### Task 3: Harden readiness and add CLI/API documentation

**Files:**
- Modify: `pkg/files/store.go`
- Modify: `pkg/files/store_test.go`
- Modify: `README.md`
- Modify: `docs/api/README.md`

- [ ] **Step 1: Write failing readiness tests for unknown and incomplete session artifacts**

  Add cases for an invalid upload directory name, a missing session manifest, a temporary manifest, and an orphan `.part` file.

- [ ] **Step 2: Run the focused readiness tests and verify they fail**

  Run `go test ./pkg/files -run 'TestDiskStoreReady' -count=1`.

- [ ] **Step 3: Extend readiness scanning without rejecting valid in-progress sessions**

  Validate upload directory names and manifests, require matching `.part` files, allow a valid `uploading` session to survive restart, and reject `.tmp` artifacts or mismatched sizes. Add a safe `CleanupUpload` operation only through the authenticated API, not automatic startup deletion.

- [ ] **Step 4: Document the new workflow and curl examples**

  Document the four-step create/status/chunk/complete flow, the sequential offset contract, and the fact that the existing one-shot endpoint remains supported.

- [ ] **Step 5: Run the full verification suite**

  Run `gofmt -w pkg/files core/apigateway`, then `go test ./...` and `go vet ./...`.

- [ ] **Step 6: Commit the documentation and readiness changes**

  Run `git add pkg/files/store.go pkg/files/store_test.go README.md docs/api/README.md && git commit -m "docs: describe resumable upload recovery"`.

