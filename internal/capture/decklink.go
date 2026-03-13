package capture

type DecklinkMode struct {
	DeviceName string
}

func (d *DecklinkMode) InputArgs() []string {
	return []string{"-f", "decklink"}
}

func (d *DecklinkMode) InputDevice() string {
	return d.DeviceName
}

func (d *DecklinkMode) Name() string {
	return "decklink (auto-detect)"
}
