# Xion Trash Restore Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent soft-delete trash area and restore API while preserving active-file behavior.

**Architecture:** `DiskStore` moves active object and manifest into a per-file trash directory on delete and reverses the move on restore. Trash capabilities are optional at the gateway so existing test doubles and alternate stores keep compiling. Readiness scans both active and trash trees.

**Tech Stack:** Go 1.23+, standard filesystem/JSON APIs, Gorilla mux, testify.

---

### Task 1: Add trash domain and disk operations

**Files:**
- Modify: `pkg/files/model.go`
- Modify: `pkg/files/store.go`
- Modify: `pkg/files/service.go`
- Test: `pkg/files/trash_test.go`

- [ ] **Step 1: Write failing tests for soft-delete and restore**

  Assert delete removes the file from active `Get`/`List`, creates one trash item, survives a new `DiskStore`, and restore returns the same file ID and bytes. Assert deleting twice is idempotent and restoring an active ID returns a conflict.

- [ ] **Step 2: Run the focused tests and verify missing methods fail**

  Run `go test ./pkg/files -run 'TestDiskStoreTrash' -count=1`.

- [ ] **Step 3: Implement trash paths, metadata, list, and restore operations**

  Add `trashDir`, `DeletedAt` on `File`, `TrashStore` methods `ListTrash` and `Restore`, move active files with directory syncs, and make `Delete` soft-delete while remaining idempotent.

- [ ] **Step 4: Add readiness and capacity coverage**

  Validate trash artifacts at startup and expose `TrashCount`/`TrashBytes` in capacity without counting trash as active objects.

- [ ] **Step 5: Run all storage tests and commit**

  Run `gofmt -w pkg/files`, `go test ./pkg/files -count=1`, then commit with `git add pkg/files && git commit -m "feat: add recoverable file trash"`.

### Task 2: Expose trash and restore HTTP endpoints

**Files:**
- Modify: `core/apigateway/handlers.go`
- Modify: `core/apigateway/handlers_test.go`

- [ ] **Step 1: Write failing HTTP tests**

  Upload a file, delete it, list `/api/v1/trash`, restore it, verify download works again, and cover unauthorized and missing-ID responses.

- [ ] **Step 2: Run the focused tests and verify routes are absent**

  Run `go test ./core/apigateway -run 'TestTrash' -count=1`.

- [ ] **Step 3: Add optional trash service routes and error mapping**

  Add authenticated `GET /api/v1/trash` and `POST /api/v1/files/{id}/restore`; preserve `204` for delete and return `409 file_restore_conflict` when an active object blocks restore.

- [ ] **Step 4: Run gateway regression tests and commit**

  Run `go test ./core/apigateway -count=1`, then commit with `git add core/apigateway && git commit -m "feat: expose trash restore API"`.

### Task 3: Document recovery workflow and verify the repository

**Files:**
- Modify: `README.md`
- Modify: `docs/api/README.md`
- Modify: `pkg/files/store_test.go`

- [ ] **Step 1: Document soft-delete semantics and curl examples**

  Explain that ordinary delete moves to trash, list/download exclude trash, and restore keeps the original file ID.

- [ ] **Step 2: Run repository verification**

  Run `git diff --check origin/main..HEAD`, `go test ./... -count=1`, and `go vet ./...`.

- [ ] **Step 3: Commit documentation**

  Run `git add README.md docs/api/README.md pkg/files/store_test.go && git commit -m "docs: describe file recovery workflow"`.
