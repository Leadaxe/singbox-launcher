# Git hooks (recommended)

This folder contains **recommended Git hooks** for the repository. They are not active until you install them into `.git/hooks/` after cloning or when setting up a new machine.

## Hooks

| Hook | Purpose |
|------|--------|
| **prepare-commit-msg** | Strips `Co-authored-by: Cursor` line from commit messages. |
| **pre-commit** | Runs `go vet ./...` before each commit. Set `SKIP_GO_HOOKS=1` to bypass. |
| **pre-push** | Runs `go test ./...` before each push. Set `SKIP_GO_HOOKS=1` to bypass. |

## Install (recommended after clone)

From the **repository root**:

### Linux / macOS / Git Bash (Windows)

```bash
cp scripts/git/prepare-commit-msg .git/hooks/ && chmod +x .git/hooks/prepare-commit-msg
cp scripts/git/pre-commit .git/hooks/ && chmod +x .git/hooks/pre-commit
cp scripts/git/pre-push .git/hooks/ && chmod +x .git/hooks/pre-push
```

Or install all at once:

```bash
for f in scripts/git/prepare-commit-msg scripts/git/pre-commit scripts/git/pre-push; do
  cp "$f" .git/hooks/"$(basename "$f")" && chmod +x .git/hooks/"$(basename "$f")"
done
```

### Windows (PowerShell)

```powershell
Copy-Item scripts\git\prepare-commit-msg .git\hooks\ -Force
Copy-Item scripts\git\pre-commit .git\hooks\ -Force
Copy-Item scripts\git\pre-push .git\hooks\ -Force
```

Note: `pre-commit` and `pre-push` are shell scripts; they run under **Git Bash** when you use `git commit` / `git push` from Git for Windows. If you use only PowerShell and do not have Git Bash, these two hooks will not run unless you install a sh interpreter or replace them with PowerShell equivalents.

## Skip hooks once

```bash
SKIP_GO_HOOKS=1 git commit -m "message"
SKIP_GO_HOOKS=1 git push
```

## Uninstall

Remove the hook files from `.git/hooks/`:

```bash
rm .git/hooks/prepare-commit-msg .git/hooks/pre-commit .git/hooks/pre-push
```
