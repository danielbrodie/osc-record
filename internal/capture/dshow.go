package capture

import "fmt"

type DShowMode struct {
	VideoDevice string
	AudioDevice string
}

func (d *DShowMode) InputArgs() []string {
	return []string{"-f", "dshow"}
}

func (d *DShowMode) InputDevice() string {
	return fmt.Sprintf("video=%s:audio=%s", d.VideoDevice, d.AudioDevice)
}

func (d *DShowMode) Name() string {
	return "dshow (manual format)"
}
