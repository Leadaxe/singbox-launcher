# Release and pre-release procedure

**🌐 Language**: English | [Русский](RELEASE_PROCESS.ru.md)

This document describes how to ship a **stable release** (`vX.Y.Z`) and a **pre-release** (`vX.Y.Z-N-gSHA-prerelease`). It is the canonical source for the procedure: if another document contradicts it, fix it here and bring the rest in line.

Companion documents:
- **`.github/workflows/README.md`** — CI mechanics: run modes, version generation, jobs, `gh workflow run` commands.
- **`AGENTS.md`** — the agent's general scope and its obligations when closing a task.
- **`docs/release_notes/`** — per-version release notes, the source of the release body for CI.

---

## 0. What CI changes, what you change

CI (`ci.yml`, the `release` job) assembles the release body from two parts:

1. **The header** — an inline template in `body:` (Downloads, per-platform instructions, Checksums), templated from `${{ needs.meta.outputs.version }}`. Not your concern.
2. **Release notes** — read from **`docs/release_notes/<slug>.md`** (the `Read release notes` step), where `<slug>` is `VERSION` without the leading `v` and with `-` in place of `.`.

| VERSION                               | SLUG                                  | file                                                         |
|---------------------------------------|---------------------------------------|--------------------------------------------------------------|
| `v0.8.7`                              | `0-8-7`                               | `docs/release_notes/0-8-7.md`                                |
| `v0.8.7-1-g50c7352-prerelease`        | `0-8-7-1-g50c7352-prerelease`         | `docs/release_notes/0-8-7-1-g50c7352-prerelease.md`          |

**The file is mandatory.** Without it CI fails at the `Read release notes` step before the release is even created, with instructions in the log. This is by design: the per-version file is the single source of the release body, which rules out content leaking in from aggregated index files.

For pre-releases CI additionally prefixes the body with a `> ⚠️ **Pre-release build** off \`$VERSION\` — for testing only, ...` banner — no need to write it by hand.

---

## 1. Stable release — `vX.Y.Z`

### 1.1. Pre-flight

1. Everything on `develop` is green (`go build ./... && go test ./... && go vet ./...`). The linter (`golangci-lint run`) — at least on the changed packages.
2. `develop` is a direct descendant of the last stable tag. Check:
   ```bash
   git fetch --tags
   git describe --tags --exclude='*-prerelease'
   # Expect something like v0.8.7-N-gSHA; if it has drifted far, consider an intermediate pre-release
   ```
3. `docs/release_notes/upcoming.md` has accumulated entries for every feature/bug in the release. The format matches the existing files: `Highlights (EN)` + `Основное (RU)`, entries linking to their SPECs.
4. **Pinned dependencies** (see §5):
   - **`internal/constants.RequiredCoreVersion`** — matches the `sing-box-lx` fork version the release's final QA was run against (a fork tag shaped `X.Y.Z-lx.N`). If a new fork release has shipped (or a critical CVE / breaking fix landed upstream in SagerNet and the fork rebased onto it) and you decide to bump — a separate `chore(core): pin sing-box-lx X.Y.Z-lx.N` commit **before** the tag, plus a note in the release notes. Win7 (`windows/386`) is on the fork too now (the `windows-386-legacy-windows-7` asset from the lx build) — there is no separate constant. Details in §5.1.
   - **`internal/constants.RequiredTemplateRef`** — the source default. Do not edit it by hand; CI builds inject the SHA via `-ldflags`. The source default is bumped in §1.5 (post-flight).
   - **`bin/wizard_template.json`** on `develop` reflects the final state of the template. No half-overwritten edits from someone else's PR.

### 1.2. Moving upcoming.md → per-version

1. `git mv docs/release_notes/upcoming.md docs/release_notes/X-Y-Z.md` (for v0.8.8 → `0-8-8.md`).
2. Tidy it up: drop draft TODOs, fill in what's missing, structure it into subsections (`Resilience & observability`, `Security`, `Fixed`, `Template defaults`, `Migration notes` and so on — see `0-8-7.md` as a model).
3. **Create a fresh empty `upcoming.md`** from the template:
   ```markdown
   # Upcoming release — черновик

   Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

   ## EN
   ### Highlights
   -

   ### Technical / Internal
   -

   ## RU
   ### Основное
   -

   ### Техническое / Внутреннее
   -
   ```
4. Update `RELEASE_NOTES.md` (the repo index): add a row to the "Последний релиз / Latest release" table and, optionally, a short "Выжимка (RU) / Highlights (EN)" at the top.
5. One commit on `develop`: `docs(release): v0.8.8 notes`.

### 1.3. Merge into main and tag

CI runs **only on a tag push**, and the tag needs **its own command** (see `.github/workflows/README.md` §⚠️):

```bash
git checkout main
git pull --ff-only
git merge --no-ff develop -m "Merge branch 'develop' into main"
git push origin main

# Now, separately — the tag
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

Do **not** run `git push origin main --tags` — GitHub then sends only the branch event, and you get tests alone, without build/release.

> ⚠️ After this step the tag sits on a merge commit in `main` which is **not** an ancestor of `develop`. Until §1.5 (merging `main` back into `develop`) is done, `develop` lags behind the tag, `git describe` on develop returns the old tag, and the next pre-release gets a malformed name. **§1.5 is mandatory — do not skip it.**

### 1.4. Checking CI

```bash
gh run list --workflow=ci.yml --limit 3
gh run watch <RUN_ID> --exit-status
```

At the finish line expect:
- 5 artifacts: `macos.zip`, `macos-catalina.zip`, `win64.zip`, `win64-full.zip` (exe + pinned core + wintun + template + Mesa3D in `mesa3d/`, assembled by the release job from the source constants), `win7-32.zip` + `checksums.txt`.
- The release is published (`isDraft=false`, `isPrerelease=false`).
- The body contains Downloads + Checksums + your `X-Y-Z.md` and no foreign blocks.

### 1.5. Post-flight: bring main back into develop

After the release, the merge commit in `main` the tag sits on is **not** an ancestor of `develop`. Left unfixed, subsequent work on develop proceeds "not from the tag", `git describe` on develop keeps returning the old tag, and the next pre-release name comes out malformed.

```bash
git checkout develop
git fetch origin
git merge --no-ff origin/main -m "chore: merge main (vX.Y.Z tag) back into develop"
git push origin develop
# Check: git describe on develop now shows vX.Y.Z-N-gSHA
```

If commits have already landed on `develop` since the release and you want linear history, `git reset --hard vX.Y.Z && git cherry-pick <commits>` + `--force-with-lease` is an option — but it is a destructive operation, done deliberately.

**Then bump the `RequiredTemplateRef` source default** in `internal/constants/constants.go` to the fresh `origin/main` HEAD. This matters for `go run .` and non-standard builds without CI injection; CI builds don't care.

```bash
NEW_REF="$(git rev-parse origin/main)"
# Open internal/constants/constants.go and set RequiredTemplateRef to $NEW_REF.
git commit -am "chore(constants): bump RequiredTemplateRef source-default"
git push origin develop
```

See §5 "Pinned dependencies" for why the source default exists and how it differs from ldflags injection.

### 1.6. Verify

- `gh release view vX.Y.Z --json isLatest,isPrerelease,isDraft` → `{isLatest:true, isPrerelease:false, isDraft:false}`.
- One of the artifacts actually launches locally.
- The macOS install script works: `curl ... install-macos.sh | bash -s -- vX.Y.Z`.

---

## 2. Pre-release — `vX.Y.Z-N-gSHA-prerelease`

Used when you want builds off `develop` for hands-on testing of a new feature but aren't ready for a stable release.

### 2.1. Pre-flight

1. Everything on `develop` is green.
2. `develop` is a descendant of the last stable tag (otherwise CI generates a malformed describe).
3. **If you decide to bump `RequiredCoreVersion`** for the pre-release (say, you're testing an upgrade against a new sing-box) — a separate commit **before** starting the workflow, so CI builds the binary with the new constant. Don't touch `RequiredTemplateRef` by hand — CI injects the current HEAD's SHA automatically.
4. Compute the SLUG **locally**, exactly the way CI will:
   ```bash
   git fetch --tags
   VER="$(git describe --tags --always --exclude='*-prerelease')-prerelease"
   SLUG="${VER#v}"; SLUG="${SLUG//./-}"
   echo "docs/release_notes/${SLUG}.md"
   # For example: docs/release_notes/0-8-7-1-g50c7352-prerelease.md
   ```

### 2.2. Release notes — two routes

CI first looks for `docs/release_notes/<SLUG>.md` (the path from 2.1). If the file is absent, CI **falls back to `docs/release_notes/upcoming.md` for pre-releases** (for stable tags there is no fallback — the file is mandatory).

**Why the fallback exists.** A pre-release slug contains the short SHA of the current HEAD. Any commit — including the one adding the release-notes file — changes HEAD, so the slug changes too, and the file you just committed under the old slug is now "the wrong version" and CI won't find it. A structural chicken-and-egg, solved by the fallback.

**Route A — do nothing (the typical case).** `develop` already has an up-to-date `upcoming.md` covering every feature/bug since the last stable. Just confirm it says what you want in the pre-release and go straight to 2.3 — CI will pick it up.

**Route B — a dedicated per-prerelease file (optional).** If you want the release body to differ from the current draft (a narrow QA build for one specific feature, say), create `docs/release_notes/<SLUG>.md`:

1. Content: 1–3 entries on what's new in this pre-release relative to the last stable. Format:
   ```markdown
   ## Highlights (EN)

   - **<feature>** — one-line summary. See [SPEC NNN](../../SPECS/NNN-.../SPEC.md).

   Everything from **vX.Y.Z** is included — see the [vX.Y.Z release notes](https://github.com/Leadaxe/singbox-launcher/releases/tag/vX.Y.Z).

   ## Основное (RU)

   - **<фича>** — одно предложение. См. [SPEC NNN](../../SPECS/NNN-.../SPEC.md).

   Всё из **vX.Y.Z** уже внутри — см. [release notes vX.Y.Z](https://github.com/Leadaxe/singbox-launcher/releases/tag/vX.Y.Z).
   ```
2. Commit on `develop`: `docs(release): notes for <SLUG>`. Push.
3. **Careful:** the slug shifts after the commit (HEAD changed). Hitting the new slug exactly is a redo routine with a rename + amend; in practice it is easier to rely on Route A's fallback, or to accept that the exact file may not match → CI takes `upcoming.md` anyway.

> The `⚠️ Pre-release build` banner is added by **CI** automatically — don't put it in the file.

### 2.3. Start the workflow

```bash
gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=false
# If you're confident in the tests and want it faster:
gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=true
# Specific platforms only:
gh workflow run ci.yml --ref develop -f run_mode=prerelease -f skip_tests=true -f "target=macOS Win64"
```

### 2.4. Wait for the build

```bash
# ID of the most recently started run:
RUN_ID="$(gh run list --workflow=ci.yml --limit 1 --json databaseId -q '.[0].databaseId')"
gh run watch "$RUN_ID" --exit-status
```

On success CI itself:
- creates the annotated tag `vX.Y.Z-N-gSHA-prerelease`;
- creates a GitHub Release with `prerelease=true`;
- fills the body with Downloads + Checksums + the `⚠️ Pre-release build` banner + the contents of your `<SLUG>.md`;
- attaches the artifacts + `checksums.txt`.

> Releases used to be created as **drafts**, and you had to un-draft them and fix the body by hand. With the current CI that's unnecessary: `isDraft=false` out of the box, and the body is already clean.

### 2.5. Verify

```bash
gh release view "<TAG>" --json isDraft,isPrerelease,name,url
# Expect: {"isDraft":false, "isPrerelease":true, ...}
```

---

## 3. Troubleshooting

### CI fails with "Release notes file not found"

That is exactly what this document describes — `docs/release_notes/<slug>.md` is mandatory. Create it (see §1.2 or §2.2), push to `develop`, restart the workflow / re-push the tag.

### `git describe` on develop returns the old tag

That means `main` was never merged back into `develop` after the previous release. Either do §1.5, or `git merge --no-ff origin/main` right now in a single commit. Until it's fixed, a pre-release will generate a name from the old tag.

### The release shipped with foreign blocks from other versions in the body

This shouldn't happen: CI reads only `docs/release_notes/<slug>.md`. If you see it, check:
1. The `Read release notes` step's log has a `✓ Using release notes from: ...` line with the expected path.
2. The `docs/release_notes/<slug>.md` file itself contains no foreign blocks.
3. Hot fix for an already-published release: `gh release edit <tag> --notes-file <clean-body>.md`.

### `main` and the tag were pushed in one command, and the build never started

See `.github/workflows/README.md` §⚠️ — GitHub sends only the branch event in that case. Re-push the tag on its own:
```bash
git push origin vX.Y.Z
```
The workflow starts again.

### The tag already exists and needs to be re-issued

Deleting a tag and a release is a last resort. If you truly must:
```bash
gh release delete vX.Y.Z --yes
git push --delete origin vX.Y.Z
git tag -d vX.Y.Z
# Then repeat §1.3
```
People who already downloaded the previous artifact are unaffected, but their checksums will no longer match what others get.

---

## 4. Checklist for the agent (copy into your reply to the user)

### Stable vX.Y.Z
- [ ] `develop` is green and a descendant of the previous stable tag.
- [ ] `RequiredCoreVersion` matches the sing-box version that was tested (see §1.1, §5).
- [ ] `bin/wizard_template.json` on `develop` reflects the final template state.
- [ ] `upcoming.md` → `docs/release_notes/X-Y-Z.md`, tidied up.
- [ ] A fresh empty `upcoming.md` has been created.
- [ ] The `RELEASE_NOTES.md` index is updated.
- [ ] The `docs(release): vX.Y.Z notes` commit is pushed.
- [ ] `main` ← merge `develop`, pushed; the `vX.Y.Z` tag pushed **as a separate command**.
- [ ] `gh run watch` is green; the release has 4 archives + `checksums.txt`.
- [ ] **`main` merged back into `develop`** (§1.5) — without this step develop is "not from the tag".
- [ ] **The `RequiredTemplateRef` source default is bumped** to the new `origin/main` HEAD (§1.5, §5).
- [ ] `git describe` on develop shows `vX.Y.Z-0-...` or `vX.Y.Z`.
- [ ] `gh release view vX.Y.Z` → `isLatest:true`.

### Prerelease
- [ ] `develop` is green and a descendant of the previous stable tag.
- [ ] If `RequiredCoreVersion` was bumped — the commit is pushed **before** the workflow (§2.1, §5).
- [ ] The SLUG was computed locally (`git describe ... + '-prerelease'`).
- [ ] `docs/release_notes/<SLUG>.md` exists and holds 1–3 entries about what's new in this pre-release.
- [ ] The commit is pushed to `develop`.
- [ ] `gh workflow run ci.yml --ref develop -f run_mode=prerelease` has been started.
- [ ] `gh run watch` is green.
- [ ] `gh release view <TAG>` → `isDraft:false, isPrerelease:true`.

---

## 5. Pinned dependencies (`RequiredCoreVersion`, `RequiredTemplateRef`)

Every launcher version is tested against one specific pair of (sing-box core, wizard template). So that users get exactly that pair — rather than "whatever is fresh on GitHub", which may have drifted in format — both dependencies are pinned in code. See [SPEC 046](../SPECS/046-F-N-PINNED_CORE_AND_TEMPLATE/SPEC.md).

### 5.1 `RequiredCoreVersion` (manual)

- **Where:** a constant in `internal/constants/constants.go`.
- **Core source (SPEC 072):** the **`Leadaxe/sing-box-lx`** fork (`constants.SingboxCoreRepo`), which builds XHTTP (`with_xhttp`) and AmneziaWG (`with_awg`). The version is a fork tag shaped `X.Y.Z-lx.N` (the fork binary prints the full tag in `sing-box version`, so the strict comparison in the Core Dashboard works). **Windows 7 (`windows/386`)** is built by the fork as well (the `windows-386-legacy-windows-7` asset, since `v1.14.0-lx.1-rc.17`) → also on the fork, with no separate upstream path (the SourceForge/legacy machinery is gone). The core is downloaded straight from the GitHub release over HTTPS — there is no separate SHA256 verification (the source is trusted and TLS covers channel integrity).
- **What it means:** the sing-box version `DownloadCore` installs on "Download / Reinstall" from the Core Dashboard. The UI no longer offers "Update to latest".
- **Who changes it:** the release maintainer. The bump is a separate commit before a stable tag, or before `gh workflow run` for a pre-release.
- **When to change it:** when a new fork release `vX.Y.Z-lx.N` ships (new XHTTP/AWG features or fixes, a rebase onto an upstream tag) — there is no auto-discovery (by design, SPEC 046), so raise the constant by hand. Win7 (`windows/386`) rides the same `RequiredCoreVersion` (the separate Win7 constant is gone).
- **How to change it:**
  ```bash
  # editor: internal/constants/constants.go → RequiredCoreVersion = "X.Y.Z-lx.N"
  git commit -am "chore(core): pin sing-box-lx X.Y.Z-lx.N"
  git push origin develop
  ```
  The release notes should carry an entry about the bump (when there is one).

### 5.2 `RequiredTemplateRef` (CI-injected, source default)

- **Where:** `internal/constants/constants.go`, a variable (not a constant) with a default value.
- **What it means:** the SHA of the commit `wizard_template.json` is pulled from via `https://raw.githubusercontent.com/Leadaxe/singbox-launcher/<SHA>/bin/wizard_template.json`. It ties the template to the exact repo state at the moment the binary was built.
- **CI builds:** `build/build_*.sh|bat` inject the current SHA via `-ldflags="-X singbox-launcher/internal/constants.RequiredTemplateRef=$(git rev-parse HEAD)"`. The source default is not used there.
- **`go run .` and non-standard builds without ldflags:** the source default is used. To keep it from going stale, it is bumped in §1.5 after every release:
  ```bash
  NEW_REF="$(git rev-parse origin/main)"
  # editor: internal/constants/constants.go → RequiredTemplateRef = "<NEW_REF>"
  git commit -am "chore(constants): bump RequiredTemplateRef source-default"
  git push origin develop
  ```
- **When **not** to change it:** before a release, mid-feature — leave the source default untouched. `go run .` keeps pulling the last release's template rather than develop's HEAD, and that is by design (local builds then match what users were shipped).

### 5.3 Template invalidation on the user's side

When the launcher is upgraded, `core.InvalidateTemplateIfStale` (called from `main.go`) compares `Settings.LastTemplateLauncherVersion` (written after the last successful "Download Template") against `constants.AppVersion`. If it is lower, `bin/wizard_template.json` is deleted and the UI shows the blue "Download Template". After a successful download the UI writes a fresh `last_template_launcher_version` into `bin/settings.json`.

Dev builds (AppVersion = `v-local-test`, `unnamed-dev`, `*-dirty`) skip invalidation — otherwise local development breaks on every run.

Details and tests — [`core/template_migration.go`](../core/template_migration.go).
