# Xion Storage Capacity Autopause Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add filesystem capacity reporting and automatic upload protection to the Xion core service, exposed through its authenticated API and `xionctl`.

**Architecture:** Extend `pkg/files.DiskStore` with a configured percentage threshold and capacity provider. `core/apigateway` adds an authenticated capacity endpoint and maps threshold rejection to HTTP 507. `cmd/xionctl` calls the API; no component mutates the data directory directly.

**Tech Stack:** Go 1.23+, `golang.org/x/sys/unix`, `net/http`, standard library JSON/flag.

---

### Task 1: Add failing capacity and automatic-pause tests

**Files:**
- Modify: `pkg/files/store_test.go`
- Modify: `core/apigateway/handlers_test.go`
- Modify: `cmd/xionctl/main_test.go`

- [x] **Step 1: Test the DiskStore threshold and capacity shape.**

Inject deterministic filesystem stats into a temporary `DiskStore`, assert `WritesPaused` at the configured percentage, and assert deleting a file changes the object count/bytes while allowing the next upload.

- [x] **Step 2: Test API capacity and HTTP 507.**

Use a fake file service that returns `files.ErrStoragePaused` from `Upload` and a `Capacity` provider with known fields; assert `/api/v1/capacity` requires the token and upload returns code `storage_paused`, status 507.

- [x] **Step 3: Test `xionctl capacity` calls the authenticated endpoint and prints JSON.**

- [ ] **Step 4: Run focused tests and confirm the expected missing-symbol failures.**

Run: `go test ./pkg/files ./core/apigateway ./cmd/xionctl -run 'Capacity|Pause|Xionctl' -count=1`.

### Task 2: Implement core capacity and automatic pause

**Files:**
- Create: `pkg/files/capacity.go`
- Modify: `pkg/files/model.go`
- Modify: `pkg/files/store.go`
- Modify: `pkg/files/service.go`
- Modify: `core/apigateway/main.go`
- Modify: `core/apigateway/handlers.go`

- [ ] **Step 1: Add `Capacity` and `DiskStoreConfig` types.**

Expose total/used/available bytes, used percentage, object count/bytes, metadata count, pause threshold, and `WritesPaused` with stable JSON names.

- [ ] **Step 2: Implement filesystem statistics with `unix.Statfs`.**

Calculate usage from the data directory mount and count regular object/manifest files under the store read lock.

- [ ] **Step 3: Add threshold checking inside `DiskStore.Put`.**

Use `XION_STORAGE_PAUSE_AT_PERCENT`, defaulted by `main.go` to 90; return `ErrStoragePaused` before creating a temporary upload when threshold is reached. Keep delete/read operations available.

- [x] **Step 4: Add `/api/v1/files/capacity` and error mapping.**

Protect the endpoint with the existing Bearer middleware and map `ErrStoragePaused` to HTTP 507 / `storage_paused`.

- [ ] **Step 5: Run focused and full Go tests.**

Run: `go test ./pkg/files ./core/apigateway -count=1 && go test ./... -count=1 && go vet ./...`.

### Task 3: Add CLI capacity command and configuration docs

**Files:**
- Modify: `cmd/xionctl/client.go`
- Modify: `cmd/xionctl/main.go`
- Modify: `cmd/xionctl/main_test.go`
- Modify: `deploy/systemd/astrastore-xion.env.example`
- Modify: `README.md`
- Modify: `docs/客户端使用文档.md`
- Modify: `docs/operations/README.md`

- [x] **Step 1: Implement `xionctl capacity` and usage text.**
- [x] **Step 2: Document `XION_STORAGE_PAUSE_AT_PERCENT=90` and the automatic behavior.**
- [ ] **Step 3: Build Linux amd64, install to `/usr/local/bin/xionctl`, and run `xionctl capacity` on the server.** (待 SSH 恢复后执行)

### Task 4: Final verification and direct-main delivery

- [ ] **Step 1: Run `gofmt`, `git diff --check`, full tests, vet, and Linux build.**
- [ ] **Step 2: Confirm no manual pause endpoint or direct object deletion was introduced.**
- [ ] **Step 3: Commit and push directly to `main`.**
