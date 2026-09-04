# AstraStoreXion README Logo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate the approved AstraStoreXion logo asset and display it at the top of the repository README.

**Architecture:** Generate one project-bound PNG from the approved image specification, inspect the raster output visually and mechanically, then add a minimal HTML image block to the existing README. Keep the asset under `docs/assets` so the README uses a stable repository-relative path.

**Tech Stack:** Built-in image generation tool, PNG, Markdown/HTML, shell verification with `file` and Git.

---

## File Structure

- Create `docs/assets/astrastore-xion-logo.png`: final 10:3 logo banner used by the root README.
- Modify `README.md`: add one centered image block before the existing H1.
- Reference `docs/superpowers/specs/2026-09-04-readme-logo-design.md`: approved visual and integration requirements.

### Task 1: Generate and validate the logo asset

**Files:**

- Create: `docs/assets/astrastore-xion-logo.png`
- Reference: `docs/superpowers/specs/2026-09-04-readme-logo-design.md`

- [ ] **Step 1: Confirm the destination does not already contain an asset that would be overwritten**

Run:

```bash
test ! -e docs/assets/astrastore-xion-logo.png
```

Expected: exit status 0 and no output. If the file exists, stop and use a versioned sibling filename unless replacement was explicitly authorized.

- [ ] **Step 2: Generate the approved logo with the built-in image generation tool**

Use this complete prompt:

```text
Use case: logo-brand
Asset type: wide README logo banner
Primary request: Create a premium horizontal brand lockup for AstraStoreXion, combining a translucent frosted-glass capsule with a dimensional ion-vortex symbol and a clean wordmark.
Scene/backdrop: a wide 10:3 rounded frosted-glass capsule floating over a very soft mist-white, pale-violet, and light-cyan atmospheric backdrop; generous breathing room around all foreground elements.
Subject: on the left, one translucent violet glass energy core surrounded by two or three intersecting elliptical ion orbits with convincing front-and-back occlusion; five to seven spherical ions of varied sizes in violet and electric cyan, with highlights, subtle refraction, soft shadows, and restrained glow. On the right, the exact wordmark AstraStoreXion.
Style/medium: refined dimensional glass logo, modern infrastructure brand, minimal and premium, crisp edges, controlled depth, not cartoonish.
Composition/framing: wide landscape approximately 10:3; centered lockup; icon has ample safe space and must not touch or feel compressed by the capsule; clear separation between icon and wordmark.
Lighting/mood: soft studio lighting with glass-edge highlights and gentle depth; calm, trustworthy, futuristic.
Color palette: violet primary, electric cyan accents, mist white and pale cyan background; emphasize Xion in electric cyan.
Materials/textures: translucent glass core and ion spheres, frosted-glass capsule, subtle refraction, no metallic chrome.
Text (verbatim): "AstraStoreXion"
Constraints: the only visible text is AstraStoreXion; spelling must be exact; preserve generous margins; show one core and five to seven distinct ion spheres; make the orbital structure visually unique and dimensional.
Avoid: slogans, Chinese text, badges, watermarks, dense star fields, complex space illustration, generic flat atom icon, neon overexposure, crowded composition, clipped or distorted elements, extra letters, misspelled text.
```

Expected: one landscape PNG whose primary composition matches the approved design.

- [ ] **Step 3: Copy the selected generated PNG into the repository**

Assign the exact absolute path returned by the built-in generation tool to the task-specific shell variable `logo_generated_path`, create the destination directory, and copy the selected file:

```bash
logo_generated_path='/exact/absolute/path/reported-by-the-generation-tool.png'
test -f "$logo_generated_path"
mkdir -p docs/assets
cp "$logo_generated_path" docs/assets/astrastore-xion-logo.png
```

Expected: `docs/assets/astrastore-xion-logo.png` exists inside the repository. The quoted assignment must be replaced with the exact returned path before the command block is run.

- [ ] **Step 4: Inspect the saved image mechanically and visually**

Run:

```bash
file docs/assets/astrastore-xion-logo.png
```

Expected: output identifies a PNG image with landscape dimensions. Open the image and verify exact wordmark spelling, one glass core, five to seven ions, dimensional orbit occlusion, translucent frosted treatment, and comfortable margins.

- [ ] **Step 5: Commit the generated asset**

```bash
git add docs/assets/astrastore-xion-logo.png
git commit -m "docs: add AstraStoreXion logo asset"
```

Expected: a commit containing only the new PNG asset.

### Task 2: Embed the logo in the README

**Files:**

- Modify: `README.md:1`
- Use: `docs/assets/astrastore-xion-logo.png`

- [ ] **Step 1: Add the centered logo block before the existing H1**

Insert exactly this content at the beginning of `README.md`:

```html
<p align="center">
  <img src="docs/assets/astrastore-xion-logo.png" alt="AstraStoreXion logo" width="760">
</p>

```

Keep the existing `# AstraStoreXion` line and every later line unchanged.

- [ ] **Step 2: Verify the README reference and surrounding structure**

Run:

```bash
sed -n '1,12p' README.md
test -f docs/assets/astrastore-xion-logo.png
rg -n 'docs/assets/astrastore-xion-logo\.png|^# AstraStoreXion$' README.md
```

Expected: the image block appears first, the PNG exists, and both the image path and original H1 are found.

- [ ] **Step 3: Verify the diff is limited to the intended README insertion**

Run:

```bash
git diff --check -- README.md
git diff -- README.md
```

Expected: no whitespace errors; the diff adds only the four-line image block and its trailing blank line before the existing H1.

- [ ] **Step 4: Commit the README integration**

```bash
git add README.md
git commit -m "docs: show logo in README"
```

Expected: a commit containing only the README change.

### Task 3: Final verification

**Files:**

- Verify: `README.md`
- Verify: `docs/assets/astrastore-xion-logo.png`

- [ ] **Step 1: Confirm the final repository state**

Run:

```bash
file docs/assets/astrastore-xion-logo.png
git status --short
git log -3 --oneline
```

Expected: the logo is a valid PNG; no new uncommitted changes from this implementation remain; the recent history contains the asset and README commits. Pre-existing unrelated untracked files may remain and must not be staged or modified.

- [ ] **Step 2: Review the rendered top of the README**

Open `README.md` in a Markdown renderer and verify that the logo is centered, displays at a readable size, resolves from the relative path, and does not remove or obscure the existing title and introduction.
