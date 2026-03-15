# Release Process

How to cut a new release of osc-record. There is no CI/CD automation — everything is done manually from the development machine.

---

## Prerequisites

- Go 1.21+ installed
- `gh` CLI authenticated (`gh auth status`)
- Write access to `danielbrodie/osc-record` and `danielbrodie/homebrew-tap`
- Python 3 (for Windows zip packaging)

## Quick reference

```bash
VERSION=1.4.0    # NO "v" prefix — see gotcha below

# 1. Build
OUTDIR="C:/Users/Daniel/AppData/Local/Temp/osc-release-$VERSION"
mkdir -p "$OUTDIR"

GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.version=$VERSION" -o "$OUTDIR/osc-record.exe" .
GOOS=darwin  GOARCH=arm64 go build -ldflags "-s -w -X main.version=$VERSION" -o "$OUTDIR/osc-record_arm64" .
GOOS=darwin  GOARCH=amd64 go build -ldflags "-s -w -X main.version=$VERSION" -o "$OUTDIR/osc-record_amd64" .

# 2. Package
cd "$OUTDIR"
cp osc-record_arm64 osc-record && tar czf osc-record_darwin_arm64.tar.gz osc-record && rm osc-record
cp osc-record_amd64 osc-record && tar czf osc-record_darwin_amd64.tar.gz osc-record && rm osc-record
python3 -c "
import zipfile
d = '$OUTDIR'
with zipfile.ZipFile(d+'/osc-record_windows_amd64.zip','w',zipfile.ZIP_DEFLATED) as z:
    z.write(d+'/osc-record.exe','osc-record.exe')
"

# 3. Get hashes — IMPORTANT: use sha256sum for .tar.gz, Python for .zip
sha256sum "$OUTDIR"/*.tar.gz
python3 -c "
import hashlib
with open('$OUTDIR/osc-record_windows_amd64.zip','rb') as f:
    print(hashlib.sha256(f.read()).hexdigest())
"

# 4. Tag and push
git tag v$VERSION
git push origin main
git push origin v$VERSION

# 5. Create release
gh release create v$VERSION \
  "$OUTDIR/osc-record_windows_amd64.zip" \
  "$OUTDIR/osc-record_darwin_arm64.tar.gz" \
  "$OUTDIR/osc-record_darwin_amd64.tar.gz" \
  --title "v$VERSION" --notes "..."

# 6. Verify uploaded hashes
gh release view v$VERSION --repo danielbrodie/osc-record \
  --json assets --jq '.assets[] | "\(.name): \(.digest)"'

# 7. Update homebrew-tap (see below)
```

---

## Updating `homebrew-tap`

After uploading release assets, update two files in `danielbrodie/homebrew-tap`:

### `osc-record.rb` (Homebrew formula)

```ruby
version "X.Y.Z"                         # ← update
sha256 "arm64-hash-here"                 # ← update (darwin_arm64)
sha256 "amd64-hash-here"                 # ← update (darwin_amd64)
```

### `bucket/osc-record.json` (Scoop manifest)

```json
"version": "X.Y.Z",                     // ← update
"hash": "sha256:windows-hash-here",      // ← update
```

The `autoupdate.url` uses `$version` substitution and does not need changing.

```bash
cd /path/to/homebrew-tap
git add osc-record.rb bucket/osc-record.json
git commit -m "vX.Y.Z: bump to $(git -C ../osc-record describe --tags)"
git push
```

---

## Gotchas

### No `v` in version string

`main.version` is set to just the number (`1.3.0`). The display code prepends `v` when printing. If you pass `v1.3.0`, users see `osc-record vv1.3.0`.

✅ `-ldflags "-X main.version=1.3.0"`
❌ `-ldflags "-X main.version=v1.3.0"`

### Build output paths on Windows

`go build` is a Windows native binary that does not interpret Git Bash's `/tmp` mount. Always use Windows-native paths:

✅ `-o "C:/Users/Daniel/AppData/Local/Temp/release/osc-record.exe"`
❌ `-o "/tmp/release/osc-record.exe"` (file appears elsewhere or is silently lost)

### SHA256 for Windows zip

`sha256sum` in Git Bash can give incorrect results for `.zip` files due to text-mode line ending translation. Use Python for the Windows zip hash:

```python
import hashlib
with open(r'C:\path\to\osc-record_windows_amd64.zip', 'rb') as f:
    print(hashlib.sha256(f.read()).hexdigest())
```

### Git commit email and push protection

GitHub rejects pushes if a commit's author email is not registered with the account (privacy protection). Use the noreply address:

```bash
git config user.email "danielbrodie@users.noreply.github.com"
```

For tags: use **lightweight tags** (no annotated tags) — annotated tags include tagger email metadata which is also checked:

```bash
git tag v1.3.0           # lightweight — no email
# NOT: git tag -a v1.3.0 -m "..."  (annotated — includes email)
```

---

## Automating with goreleaser (future)

A `.goreleaser.yml` exists but goreleaser is not installed. To automate releases:

1. Install goreleaser: `scoop install goreleaser` or from goreleaser.com
2. Set a `GITHUB_TOKEN` environment variable
3. Run: `goreleaser release --clean`

The `.goreleaser.yml` handles cross-compilation, packaging, and GitHub release creation automatically. The Homebrew formula update would still be manual (or could be added to the goreleaser config).
