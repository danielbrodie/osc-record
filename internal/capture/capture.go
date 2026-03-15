package capture

type CaptureMode interface {
	Name() string
	Summary() string
	BuildInputArgs(videoDevice, audioDevice string) []string
	// BuildExternalAudioArgs returns ffmpeg input args for a secondary audio source
	// (e.g. Dante, line-in) to mix with DeckLink video. Returns nil when the mode
	// handles audio natively (avfoundation, dshow) or when audioDevice is empty.
	BuildExternalAudioArgs(audioDevice string) []string
	NeedsAudio() bool
	SignalProbe(ffmpegPath, device string) error
}

const (
	ModeAuto         = "auto"
	ModeDecklink     = "decklink"
	ModeAVFoundation = "avfoundation"
	ModeDShow        = "dshow"
)
