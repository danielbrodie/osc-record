package capture

type CaptureMode interface {
	Name() string
	Summary() string
	BuildInputArgs(videoDevice, audioDevice string) []string
	NeedsAudio() bool
	SignalProbe(ffmpegPath, device string) error
}

const (
	ModeAuto         = "auto"
	ModeDecklink     = "decklink"
	ModeAVFoundation = "avfoundation"
	ModeDShow        = "dshow"
)
