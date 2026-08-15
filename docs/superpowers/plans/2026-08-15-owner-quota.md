# Xion Owner Quota Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce an optional per-owner logical byte quota and expose its current usage.

**Architecture:** `DiskStore` stores one configured byte limit and computes owner usage from active and trash manifests under its existing mutex. One-shot uploads check before publishing; resumable sessions check at creation. Gateway and runtime configuration remain optional.

**Tech Stack:** Go 1.23+, standard filesystem/JSON APIs, Gorilla mux, testify.

---

### Task 1: Add quota domain, storage enforcement, and tests

**Files:**
- Modify: `pkg/files/model.go`
- Modify: `pkg/files/store.go`
- Modify: `pkg/files/service.go`
- Test: `pkg/files/quota_test.go`

- [ ] **Step 1: Write failing tests for owner usage and upload rejection**

  Configure a 10-byte owner quota, upload a 6-byte file for `blog`, reject a 5-byte upload, assert usage/available values, and verify resumable creation rejects a declared size larger than the remaining quota.

- [ ] **Step 2: Run the focused tests and verify missing quota methods fail**

  Run `go test ./pkg/files -run 'TestOwnerQuota' -count=1`.

- [ ] **Step 3: Implement quota fields, usage scan, enforcement, and `Quota` method**

  Add `Quota`, `ErrQuotaExceeded`, `NewDiskStoreWithPauseAndQuota`, owner normalization, and active-plus-trash usage accounting. Preserve existing constructors by defaulting the quota to zero.

- [ ] **Step 4: Run storage tests and commit**

  Run `gofmt -w pkg/files`, `go test ./pkg/files -count=1`, then commit with `git add pkg/files && git commit -m "feat: enforce owner storage quota"`.

### Task 2: Expose quota through gateway and runtime

**Files:**
- Modify: `core/apigateway/handlers.go`
- Modify: `core/apigateway/handlers_test.go`
- Modify: `core/apigateway/main.go`

- [ ] **Step 1: Write failing HTTP tests**

  Cover authorized `/api/v1/files/quota?owner=blog`, invalid owner requests, and `507 quota_exceeded` on upload.

- [ ] **Step 2: Implement optional quota service and error mapping**

  Add the authenticated quota route and keep the route disabled only when the backing store lacks quota support.

- [ ] **Step 3: Read `XION_OWNER_QUOTA_BYTES` at startup**

  Construct the store with the configured quota and fail fast for malformed or negative values.

- [ ] **Step 4: Run gateway and full Go tests, then commit**

  Run `go test ./core/apigateway ./pkg/files -count=1`, then commit with `git add core/apigateway pkg/files && git commit -m "feat: expose owner quota status"`.

### Task 3: Document quota operations and verify

**Files:**
- Modify: `README.md`
- Modify: `docs/api/README.md`
- Modify: `deploy/systemd/astrastore-xion.env.example`

- [ ] **Step 1: Document configuration, owner key, 507 response, and query example**

- [ ] **Step 2: Run `git diff --check origin/main..HEAD`, `go test ./... -count=1`, `go test ./... -race -count=1`, and `go vet ./...`**

- [ ] **Step 3: Commit documentation**

  Run `git add README.md docs/api/README.md deploy/systemd/astrastore-xion.env.example && git commit -m "docs: describe owner quotas"`.
