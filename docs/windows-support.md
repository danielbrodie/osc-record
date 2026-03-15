# Windows Support — Developer Notes

Complete technical reference for Windows support in osc-record. This document captures what was discovered, what was tried, what works, what doesn't, and why — so the next person doesn't have to rediscover it.

---

## Summary

osc-record works on Windows using the Blackmagic Desktop Video **WDM (Windows Driver Model)** capture interface via ffmpeg's `dshow` input format. This is distinct from the native DeckLink mode used on macOS, which requires a custom ffmpeg build. On Windows, standard ffmpeg (e.g. from Scoop or gyan.dev) is sufficient.

**Tested configuration (March 2025):**
- Windows 11 Pro 10.0.26100
- Blackmagic UltraStudio Recorder 3G
- Blackmagic Desktop Video 14.x drivers
- ffmpeg 8.0.1 (gyan.dev full build, installed via WinGet / Scoop)
- Signal: SD NTSC 720×486 @ 29.97fps

---

## How Windows Capture Works

### macOS vs Windows

| Aspect | macOS | Windows |
|--------|-------|---------|
| Capture framework | Native DeckLink API | DirectShow (dshow) via WDM driver |
| ffmpeg input | `-f decklink` | `-f dshow` |
| Signal auto-detect | Yes — DeckLink API probes format | Yes — WDM driver negotiates format with OS |
| Format config needed | No (auto on first run) | No (driver handles it) |
| Custom ffmpeg needed | Yes (`--enable-decklink`) | No (standard build works) |
| Audio | Embedded SDI audio via DeckLink | Separate `audio=` device in dshow |

### DirectShow Device Names (UltraStudio Recorder 3G)

When Blackmagic Desktop Video is installed, these devices appear via `ffmpeg -f dshow -list_devices true -i dummy`:

| Device name | Type | Use |
|-------------|------|-----|
| `Blackmagic WDM Capture` | video + audio | **Use this as the video device.** Auto-negotiates format. |
| `Decklink Video Capture` | video only | Virtual device; less reliable. |
| `Decklink Audio Capture` | audio only | Can be used as audio source. |
| `Line In (Blackmagic UltraStudio Recorder 3G Audio)` | audio only | Physical audio input. **Preferred audio source.** |

**Important:** `Blackmagic WDM Capture` is listed as `(audio, video)` — a combo device. In ffmpeg's dshow syntax, the audio source in `video=X:audio=Y` must be a **pure audio** device. The combo device cannot be used as the `audio=` value. Use `Line In (...)` or `Decklink Audio Capture` instead.

### The ffmpeg dshow command

osc-record's `DShowMode.BuildInputArgs` produces:

```
ffmpeg -f dshow -i "video=Blackmagic WDM Capture:audio=Line In (Blackmagic UltraStudio Recorder 3G Audio)" \
  -c:v libx264 -crf 18 -preset fast -c:a aac -b:a 192k output.mp4
```

No `-video_size`, `-framerate`, or pixel format flags needed. The WDM driver automatically matches the incoming signal (720×486 @ 29.97fps in testing, but works for any signal the hardware supports).

### Signal format auto-detection

Unlike DeckLink native mode (which uses `decklink -list_formats` to probe), dshow auto-negotiates format through the Windows DirectShow filter graph. When ffmpeg opens `video=Blackmagic WDM Capture`, the Blackmagic WDM driver reports the current incoming signal format. If the signal changes (e.g., source switches from 1080i to 720p), you must restart the recording session — there's no hot-switching.

To inspect supported formats manually:
```
ffmpeg -hide_banner -f dshow -list_options true -i "video=Blackmagic WDM Capture"
```
This lists all formats the driver can deliver (not just the currently active signal).

---

## Bugs Found and Fixed

### 1. `parseDShow` — ffmpeg 6+ format change (critical)

**Symptom:** `osc-record devices` showed `(none found)` for all device categories despite hardware being connected and visible via `ffmpeg -f dshow -list_devices true -i dummy`.

**Root cause:** ffmpeg changed the DirectShow device listing format around version 6. The old format used section headers:
```
DirectShow video devices (some may be both video and audio devices)
 "Device Name"
DirectShow audio devices
 "Audio Device"
```
The new format (ffmpeg 6+) annotates each device inline:
```
[dshow @ addr] "Device Name" (video)
[dshow @ addr]   Alternative name "@device_pnp_..."
[dshow @ addr] "Another Device" (audio, video)
[dshow @ addr] "Audio Only" (audio)
```

**Fix:** Added `parseDShowNew()` that splits on the per-line type annotation and routes to it when the old section headers are absent. Both formats are now supported.

**Key details:**
- "Alternative name" lines contain a quoted path starting with `@` — filtered out via `strings.HasPrefix(name, "@")`
- Combo `(audio, video)` devices are excluded from the audio list — they cannot be used as the `audio=` source in dshow's `video=X:audio=Y` syntax
- `strings.TrimRight(line, "\r")` strips Windows CRLF artifacts before regex matching

**File:** `internal/devices/devices.go` — `parseDShow`, `parseDShowOld`, `parseDShowNew`

---

### 2. `BuildInputArgs` — Go `%q` quoting bug (critical)

**Symptom:** Recording failed to start; ffmpeg could not find the specified device.

**Root cause:** The original implementation used `fmt.Sprintf("video=%q:audio=%q", ...)`. Go's `%q` verb produces a double-quoted string: `"Device Name"`. The resulting ffmpeg argument was:
```
-i video="Blackmagic WDM Capture":audio="Line In (...)"
```
ffmpeg's dshow parser does not strip these quotes — it looked for a device literally named `"Blackmagic WDM Capture"` (with embedded quotes), which doesn't exist.

**Fix:** Changed to string concatenation: `"video=" + videoDevice`. Also added a guard for empty `audioDevice` to avoid producing `video=X:audio=`:
```go
input := "video=" + videoDevice
if audioDevice != "" {
    input += ":audio=" + audioDevice
}
```

**File:** `internal/capture/dshow.go` — `DShowMode.BuildInputArgs`

---

### 3. Setup wizard — no device selection step (UX bug)

**Symptom:** After `osc-record setup`, the device name in config was empty (`name = ""`). On first `osc-record run`, the daemon would prompt interactively for a device — but only in TUI mode. In `--no-tui` mode, this caused a hang or error.

**Root cause:** The original `setup` wizard asked for:
1. OSC record address
2. OSC stop address
3. Video input (HDMI/SDI/Auto) — **DeckLink-only concept, meaningless for dshow**
4. Output directory
5. Filename prefix

It never asked which device to use.

**Fix:** Rewrote the setup flow:
1. Detect capture mode (tries DeckLink, falls back to dshow)
2. Probe available devices for that mode
3. **Prompt user to select a video device from numbered list**
4. Auto-match audio device by name (BestAudioMatch)
5. Prompt for audio device if auto-match fails
6. If DeckLink mode: prompt for video input (HDMI/SDI/Auto)
7. OSC record address
8. OSC stop address
9. Output directory
10. Filename prefix

**File:** `cmd/setup.go`

---

### 4. `BestAudioMatch` — generic word bias (minor bug)

**Symptom:** When auto-matching audio for `Blackmagic WDM Capture`, the scorer could prefer `Decklink Audio Capture` (shares the word "Capture") over `Line In (Blackmagic UltraStudio Recorder 3G Audio)` (shares the word "Blackmagic").

**Root cause:** The word-by-word scoring counted matches equally (`score++`) regardless of word length. Generic words like "Capture" and "Audio" (short, common) competed with specific brand/model words like "Blackmagic" and "UltraStudio".

**Fix:** Weight score by word length: `score += len(w)`. "Blackmagic" (10) now outranks "Capture" (7), and "UltraStudio" (11) outranks "Audio" (5).

**Result:** `Blackmagic WDM Capture` correctly auto-matches to `Line In (Blackmagic UltraStudio Recorder 3G Audio)` (score 10 for "blackmagic") over `Decklink Audio Capture` (score 7 for "capture").

**File:** `internal/devices/devices.go` — `BestAudioMatch`

---

### 5. Device display — empty `[]` ID brackets (cosmetic)

**Symptom:** `osc-record devices` displayed dshow devices as `[] Device Name` instead of just `Device Name`.

**Root cause:** dshow devices are name-addressed (you reference them by `video=Name`), unlike avfoundation which uses numeric indices (`[0]`, `[1]`). The `parseDShowNew` function correctly leaves `Device.ID` empty, but the display code always printed `[%s]` without checking.

**Fix:** Added the same nil-check that already existed for audio devices:
```go
if item.ID == "" {
    fmt.Printf("  %s\n", item.Name)
} else {
    fmt.Printf("  [%s] %s\n", item.ID, item.Name)
}
```

**File:** `cmd/devices.go`

---

## What Doesn't Work on Windows

### Native DeckLink mode (`-f decklink`)

The native DeckLink capture mode requires ffmpeg compiled with `--enable-decklink`, which links against the Blackmagic DeckLink SDK. There is no pre-built Windows ffmpeg with DeckLink support available publicly (the SDK has licensing restrictions on redistribution of binaries compiled against it).

**Impact:** `auto` mode falls back to dshow. The warning message is expected:
```
Warning: ffmpeg does not support decklink input format. Falling back to dshow.
```

**What you lose without native DeckLink:**
- The `F1` signal scanner (probes all DeckLink format codes)
- The signal panel in TUI shows static rather than live signal lock status
- `format_code` and `video_input` config fields are irrelevant

**Potential path to native DeckLink on Windows:**
1. Install Visual Studio Build Tools + Windows SDK
2. Download Blackmagic DeckLink SDK from [blackmagicdesign.com/developer](https://www.blackmagicdesign.com/developer)
3. Build ffmpeg from source with `--enable-decklink` pointing to the SDK headers
4. This is a significant undertaking; document it separately if attempted

### ProRes encoding

ProRes is blocked on Windows (`if runtime.GOOS == "windows" && profile == "prores"`) because it requires Apple codec infrastructure. Use `h264` or `hevc` instead.

### SO_REUSEPORT

`reuseport_windows.go` returns a no-op `ListenConfig`. This means you cannot have osc-record and a separate OSC monitor (like Protokol) both listening on port 8000 simultaneously. On macOS, SO_REUSEPORT allows this. Windows does not support SO_REUSEPORT in the same way.

---

## Release Process

There is no CI/CD automation for releases. All release artifacts are built and uploaded manually.

### Step-by-step

**1. Determine version** (following semver, no `v` prefix in the version string):
```bash
# Patch: bug fixes → X.Y.Z+1
# Minor: new features → X.Y+1.0
# Major: breaking changes → X+1.0.0
```

**2. Update main.go default** (only affects dev builds; release builds use ldflags):
The `var version = "0.1.0"` in `main.go` is the dev fallback. Release builds override it via `-ldflags "-X main.version=X.Y.Z"`.

**3. Build all platforms** — must use Windows-native absolute paths for `-o`:
```bash
# From Git Bash on Windows — use C:/ paths, NOT /tmp/ paths
# Go is a Windows native binary and does NOT translate /tmp → AppData\Local\Temp

GOPATH_WIN="C:/Users/Daniel/AppData/Local/Temp/release"
mkdir -p "$GOPATH_WIN"

GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.version=X.Y.Z" -o "$GOPATH_WIN/osc-record.exe" .
GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X main.version=X.Y.Z" -o "$GOPATH_WIN/osc-record_arm64" .
GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X main.version=X.Y.Z" -o "$GOPATH_WIN/osc-record_amd64" .
```

**CRITICAL: Do NOT include a `v` prefix in the version string.** `main.version` is set to just the number (e.g. `1.3.0`), and the display code prepends `v` when printing. Passing `v1.3.0` produces `osc-record vv1.3.0`.

**4. Package archives:**
```bash
cd "$GOPATH_WIN"

# Darwin: binary inside tar must be named "osc-record" (no suffix)
cp osc-record_arm64 osc-record && tar czf osc-record_darwin_arm64.tar.gz osc-record && rm osc-record
cp osc-record_amd64 osc-record && tar czf osc-record_darwin_amd64.tar.gz osc-record && rm osc-record

# Windows: zip with osc-record.exe inside
python3 -c "
import zipfile
wr = '$GOPATH_WIN'
with zipfile.ZipFile(wr+'/osc-record_windows_amd64.zip', 'w', zipfile.ZIP_DEFLATED) as zf:
    zf.write(wr+'/osc-record.exe', 'osc-record.exe')
"
```

**5. Compute SHA256:**
```bash
sha256sum "$GOPATH_WIN"/*.tar.gz

python3 -c "
import hashlib
with open('C:/path/to/osc-record_windows_amd64.zip', 'rb') as f:
    print(hashlib.sha256(f.read()).hexdigest())
"
```

Note: `sha256sum` in Git Bash works fine for `.tar.gz` files. For `.zip` on Windows, use Python since `sha256sum` may report incorrect hashes due to path translation issues.

**6. Tag and push:**
```bash
git tag vX.Y.Z       # lightweight tag (no email in metadata — avoids GitHub push protection)
git push origin main
git push origin vX.Y.Z
```

**7. Create GitHub release with all assets:**
```bash
gh release create vX.Y.Z \
  "$GOPATH_WIN/osc-record_windows_amd64.zip" \
  "$GOPATH_WIN/osc-record_darwin_arm64.tar.gz" \
  "$GOPATH_WIN/osc-record_darwin_amd64.tar.gz" \
  --title "vX.Y.Z — <summary>" \
  --notes "..."
```

**8. Verify asset hashes match what was uploaded:**
```bash
gh release view vX.Y.Z --repo danielbrodie/osc-record --json assets \
  --jq '.assets[] | "\(.name): \(.digest)"'
```

**9. Update `danielbrodie/homebrew-tap`:**

Clone the repo, update both files, commit, push:

`osc-record.rb`:
- Update `version "X.Y.Z"`
- Update both `sha256` lines (arm64 and amd64)

`bucket/osc-record.json`:
- Update `"version": "X.Y.Z"`
- Update `"hash": "sha256:..."`
- (The `autoupdate.url` uses `$version` substitution — no change needed)

```bash
cd /path/to/homebrew-tap
git add osc-record.rb bucket/osc-record.json
git commit -m "vX.Y.Z: bump formula and Scoop manifest"
git push
```

### Known gotchas

| Problem | Cause | Fix |
|---------|-------|-----|
| `go build` produces exe in wrong place | Git Bash `/tmp` ≠ Windows `%TEMP%` | Use `C:/Users/.../AppData/Local/Temp/` paths |
| `osc-record vv1.3.0` (double v) | Passed `v1.3.0` to ldflags instead of `1.3.0` | Omit the `v` prefix in `-X main.version=` |
| `git push` rejected (email privacy) | Commit email exposed private address | Use `danielbrodie@users.noreply.github.com` or lightweight tags |
| `Edit` tool fails on CRLF files | Git on Windows checks out files with CRLF | `sed -i 's/\r//' file` or rewrite the whole file |
| `sha256sum` wrong hash for zip | Path translation artifacts | Use Python `hashlib.sha256` for Windows zip files |
| Empty zip (22 bytes) | Python `zf.write('filename')` with relative path, wrong cwd | Pass full absolute Windows path to `zf.write()` |

---

## Scoop Bucket Structure

The `danielbrodie/homebrew-tap` repo serves dual purpose:
- **Homebrew**: `.rb` formula files in the root
- **Scoop**: JSON manifests in the `bucket/` subdirectory

Scoop installation:
```powershell
scoop bucket add danielbrodie https://github.com/danielbrodie/homebrew-tap
scoop install osc-record
```

The `depends: "main/ffmpeg"` in the manifest auto-installs standard ffmpeg from Scoop's main bucket. This is the gyan.dev full build (no DeckLink support, but works for dshow capture).

**Manifest auto-update:** `checkver.github` + `autoupdate.url` allow `scoop checkver` to detect new releases and update the manifest URL automatically. The `hash` still needs to be updated manually (or via `scoop checkver --update` which recomputes it).

---

## Windows Development Environment Notes

### PowerShell module loading issues

When running PowerShell from Git Bash (via `powershell -Command ...`), the `Microsoft.PowerShell.Security` module may fail to load. This breaks `Set-ExecutionPolicy` and the Scoop installer's policy check. Workarounds:
- Use PowerShell 7 (`pwsh`) instead of Windows PowerShell 5.x
- Run Scoop installer: `pwsh -NoProfile -ExecutionPolicy Bypass -Command "irm get.scoop.sh | iex"`

### Scoop PATH after install

After installing Scoop, `~/scoop/shims` is added to the user PATH in PowerShell sessions. It is NOT automatically available in existing Git Bash sessions. Either:
- Open a new PowerShell window and use `osc-record` directly
- In Git Bash, use the full path: `~/scoop/shims/osc-record.exe`
- Add `~/scoop/shims` to your Git Bash PATH: `export PATH="$PATH:$HOME/scoop/shims"` in `~/.bashrc`

### CRLF line endings

Git on Windows checks out files with CRLF line endings by default. This causes the Claude Code `Edit` tool to fail to match multi-line strings. Fix for a specific file before editing:
```bash
sed -i 's/\r//' filename.go
```
Or just rewrite the whole file with the `Write` tool.

### Python path translation

Python on Windows does not translate Unix-style `/tmp` paths from Git Bash's mount points. When scripting file operations in Python from within a Git Bash session, always use Windows-native paths (`C:/Users/...` or raw strings `r'C:\Users\...'`).
