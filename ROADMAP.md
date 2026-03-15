# osc-record Roadmap

## v1.2.0 — Auto-Detection ✓

Shipped. Auto-detects `video_input` (HDMI/SDI) and `format_code` on first run. Setup wizard includes input selection. Signal panel shows `⟳ probing...` during detection. Config persisted atomically after full detection sequence.

---

## Backlog

- **amd64 bottle for `ffmpeg-decklink`**: needs Intel Mac or GitHub Actions runner with DeckLink SDK
- **Audio meter CPU optimization**: currently ~29% CPU on Apple Silicon; explore lower-rate polling or lighter astats config
- **Windows `ffprobe-decklink` fallback**: `ffprobe-decklink.exe` equivalent not packaged yet
- **Multi-device auto-detection**: v1.2.0 scope covers single device only; `[[devices]]` array auto-detect is future work
