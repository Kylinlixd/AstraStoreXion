# Xion Replication Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in durable asynchronous replica queue for upload, delete, and restore operations.

**Architecture:** A standalone `pkg/replication` manager persists one JSON job per operation and file ID, streams source objects to a replica through the existing HTTP API, and retries failures in a background worker. The gateway calls an optional enqueue interface after primary mutations, while a replication marker prevents loops.

**Tech Stack:** Go 1.23+, `net/http`, multipart streaming with `io.Pipe`, standard JSON/filesystem APIs, testify.

---

### Task 1: Implement durable replication manager

**Files:**
- Create: `pkg/replication/manager.go`
- Test: `pkg/replication/manager_test.go`

- [ ] **Step 1: Write failing tests for upload and delete jobs**

  Use an `httptest.Server` replica and a fake source. Enqueue an upload, call deterministic `ProcessPending`, assert the replica receives the original bytes and internal marker, then enqueue delete and assert the delete request. Reopen the manager and assert unfinished jobs reload.

- [ ] **Step 2: Run the focused tests and verify missing manager methods fail**

  Run `go test ./pkg/replication -run 'TestReplication' -count=1`.

- [ ] **Step 3: Implement job persistence, streaming HTTP replication, retries, status, and retry reset**

  Add `NewManager`, `Start`, `Close`, `EnqueueUpload`, `EnqueueDelete`, `EnqueueRestore`, `ProcessPending`, `Status`, and `Retry`. Persist jobs with atomic JSON writes, reset `running` to `pending` on startup, and stream multipart bodies without buffering the object.

- [ ] **Step 4: Run manager tests and commit**

  Run `gofmt -w pkg/replication`, `go test ./pkg/replication -count=1`, then commit with `git add pkg/replication && git commit -m "feat: add durable replication queue"`.

### Task 2: Wire replication into the gateway and runtime

**Files:**
- Modify: `core/apigateway/handlers.go`
- Modify: `core/apigateway/handlers_test.go`
- Modify: `core/apigateway/main.go`
- Modify: `pkg/replication/manager.go`

- [ ] **Step 1: Write failing gateway tests for mutation enqueue and status**

  Inject a fake replication enqueuer, upload/delete/restore a file, assert the corresponding operation is recorded, and verify `X-Xion-Replication: true` suppresses re-enqueue.

- [ ] **Step 2: Run focused gateway tests and verify missing optional capability fails**

  Run `go test ./core/apigateway -run 'TestReplication' -count=1`.

- [ ] **Step 3: Add optional gateway hooks and status/retry endpoints**

  Add authenticated `GET /api/v1/replication` and `POST /api/v1/replication/{file-id}/retry`; enqueue mutations only when a manager is configured and never fail the primary request because a queue write fails without logging it.

- [ ] **Step 4: Configure the manager from environment variables**

  Read `XION_REPLICA_URL`, `XION_REPLICA_TOKEN`, `XION_REPLICATION_DIR`, and `XION_REPLICATION_MAX_ATTEMPTS` in `core/apigateway/main.go`; start and close the worker with the HTTP server lifecycle.

- [ ] **Step 5: Run gateway and full Go tests, then commit**

  Run `go test ./core/apigateway ./pkg/replication -count=1`, then commit with `git add core/apigateway pkg/replication && git commit -m "feat: wire replica queue into Xion"`.

### Task 3: Document operations and verify

**Files:**
- Modify: `README.md`
- Modify: `docs/api/README.md`
- Modify: `deploy/systemd/astrastore-xion.env.example`

- [ ] **Step 1: Document opt-in configuration, status, retry, and loop prevention**

  Include safe placeholders only; do not include production addresses or tokens.

- [ ] **Step 2: Run final verification**

  Run `git diff --check origin/main..HEAD`, `go test ./... -count=1`, `go test ./... -race -count=1`, and `go vet ./...`.

- [ ] **Step 3: Commit documentation**

  Run `git add README.md docs/api/README.md deploy/systemd/astrastore-xion.env.example && git commit -m "docs: describe replica queue operations"`.
