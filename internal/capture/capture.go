package capture

// Mode builds the ffmpeg input arguments for a given capture configuration.
type Mode interface {
	// InputArgs returns the ffmpeg input arguments (everything before -i and the input string).
	InputArgs() []string
	// InputDevice returns the -i argument value.
	InputDevice() string
	// Name returns a human-readable name for display.
	Name() string
}
