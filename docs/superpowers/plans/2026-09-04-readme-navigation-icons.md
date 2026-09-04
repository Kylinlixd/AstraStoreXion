# README Icon Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a compact icon-based table of contents and matching section icons to the root README.

**Architecture:** Use repository-local HTML anchors for stable navigation and standard Unicode Emoji for visual scanning. Modify only `README.md`; do not add dependencies, remote assets, or generated files.

**Tech Stack:** Markdown, inline HTML anchors, Unicode Emoji, Git.

---

## File Structure

- Modify `README.md`: add the centered quick-navigation row, explicit section anchors, and matching heading icons.
- Reference `docs/superpowers/specs/2026-09-04-readme-navigation-icons-design.md`: approved mappings and constraints.

### Task 1: Add the quick-navigation row

**Files:**

- Modify: `README.md:9`

- [ ] **Step 1: Insert the navigation block after the project introduction**

Insert this exact block after the introductory paragraph and before the current-capabilities section:

```html
<p align="center">
  <a href="#features">✨ 当前能力</a> ·
  <a href="#quick-start">🚀 快速开始</a> ·
  <a href="#python-sdk">🐍 Python SDK</a> ·
  <a href="#production">🛠️ 生产部署</a> ·
  <a href="#test-assets">🧪 测试资料</a> ·
  <a href="#verification">✅ 验证</a> ·
  <a href="#limitations">⚠️ 限制</a> ·
  <a href="#license">📄 License</a>
</p>
```

Expected: one centered row that may wrap naturally on narrow screens.

- [ ] **Step 2: Verify all eight navigation destinations are present exactly once**

Run:

```bash
for anchor_name in features quick-start python-sdk production test-assets verification limitations license; do
  test "$(rg -o "href=\"#$anchor_name\"" README.md | wc -l | tr -d ' ')" = "1"
done
```

Expected: exit status 0 and no output.

### Task 2: Add stable anchors and matching heading icons

**Files:**

- Modify: `README.md`

- [ ] **Step 1: Replace each main heading with its explicit anchor and icon mapping**

Apply these exact replacements:

```text
## 当前能力       -> <a id="features"></a>       + ## ✨ 当前能力
## 快速开始       -> <a id="quick-start"></a>    + ## 🚀 快速开始
## Python SDK     -> <a id="python-sdk"></a>     + ## 🐍 Python SDK
## 生产部署       -> <a id="production"></a>     + ## 🛠️ 生产部署
## 测试资料       -> <a id="test-assets"></a>    + ## 🧪 测试资料
## 验证           -> <a id="verification"></a>   + ## ✅ 验证
## 限制           -> <a id="limitations"></a>    + ## ⚠️ 限制
## License        -> <a id="license"></a>        + ## 📄 License
```

Each anchor must appear on its own line with one blank line before the Markdown heading.

- [ ] **Step 2: Add icons to the two feature subsections**

Apply these exact heading changes without adding them to the quick-navigation row:

```text
### 可选主从异步复制 -> ### 🔁 可选主从异步复制
### 可恢复分片上传   -> ### 📦 可恢复分片上传
```

- [ ] **Step 3: Verify heading and anchor mappings**

Run:

```bash
rg -n '^<a id="(features|quick-start|python-sdk|production|test-assets|verification|limitations|license)"></a>$' README.md
rg -n '^## (✨ 当前能力|🚀 快速开始|🐍 Python SDK|🛠️ 生产部署|🧪 测试资料|✅ 验证|⚠️ 限制|📄 License)$' README.md
rg -n '^### (🔁 可选主从异步复制|📦 可恢复分片上传)$' README.md
```

Expected: eight anchors, eight main icon headings, and two icon subsection headings.

### Task 3: Verify and commit the README change

**Files:**

- Verify: `README.md`

- [ ] **Step 1: Check formatting and diff scope**

Run:

```bash
git diff --check -- README.md
git diff -- README.md
```

Expected: no whitespace errors. The diff contains only the navigation block, explicit anchors, and heading icon changes; the existing Logo, body text, code examples, and links remain unchanged.

- [ ] **Step 2: Run the repository test suite**

Run:

```bash
go test ./...
```

Expected: exit status 0 and no failing packages.

- [ ] **Step 3: Commit only the README**

Run:

```bash
git add README.md
git commit -m "docs: add icon navigation to README"
```

Expected: one commit containing only `README.md`.

### Task 4: Push the approved main-branch changes

**Files:**

- Verify: Git branch and remote state

- [ ] **Step 1: Confirm the current branch and configured push remote**

Run:

```bash
git branch --show-current
git remote -v
```

Expected: current branch is `main`, and a configured remote is available.

- [ ] **Step 2: Push main**

Run:

```bash
git push
```

Expected: the remote accepts the new main-branch commits.

- [ ] **Step 3: Confirm repository state**

Run:

```bash
git status --short
git log -5 --oneline
```

Expected: no new uncommitted changes from this task remain. Pre-existing unrelated untracked files may remain unchanged.
