# xionctl 日志命令 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe `xionctl logs` command for the Xion systemd service.

**Architecture:** Keep the fixed unit name and journal argument construction inside `cmd/xionctl`; inject a journal runner in tests while production uses `exec.CommandContext`. Update the CLI usage and operator docs without changing the Xion HTTP API.

**Tech Stack:** Go standard library `flag`, `os/exec`, `context`, `io`.

---

### Task 1: Test the log command contract

**Files:**
- Modify: `cmd/xionctl/main_test.go`

- [ ] **Step 1: Add tests for default tail, flags, follow, and error propagation.**

Use a fake journal runner that records the exact argument slice and writes sample lines to stdout. Assert that the fixed unit, `--no-pager`, `--output=cat`, default `-n 100`, custom `--since`, and `-f` are present.

- [ ] **Step 2: Run the focused test and confirm it fails because `logs` is unknown.**

Run: `go test ./cmd/xionctl -run Logs -count=1`

Expected: FAIL until the command is implemented.

### Task 2: Implement journal execution

**Files:**
- Modify: `cmd/xionctl/main.go`
- Create: `cmd/xionctl/logs.go`

- [ ] **Step 1: Add `parseLogsFlags` with `--lines`, `--since`, and `--follow`.**

Validate lines between 1 and 10000 and reject positional arguments.

- [ ] **Step 2: Add `journalctlRunner` using `exec.CommandContext`.**

Invoke `journalctl -u astrastore-xion.service --no-pager --output=cat -n <lines>`, append `--since <value>` and `-f` only when requested, and connect stdout/stderr directly.

- [ ] **Step 3: Dispatch `logs` and update usage text.**

Use the injected runner in tests and the real runner in `run`; preserve runner errors.

- [ ] **Step 4: Run the focused tests and the full Go suite.**

Run: `go test ./cmd/xionctl -run Logs -count=1 && go test ./... -count=1 && go vet ./...`.

### Task 3: Document, install, and verify on server

**Files:**
- Modify: `README.md`
- Modify: `docs/客户端使用文档.md`
- Modify: `docs/operations/README.md`

- [ ] **Step 1: Add the three log examples and permission note.**
- [ ] **Step 2: Build Linux amd64 and install over the existing `/usr/local/bin/xionctl`.**
- [ ] **Step 3: Run `xionctl logs --lines 5` on the server and verify output.**
- [ ] **Step 4: Commit and push directly to `main`.**
