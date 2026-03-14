package tui

type UserCmd int

const (
	UserCmdRecord UserCmd = iota + 1
	UserCmdStop
	UserCmdConfigChanged
	UserCmdGrabPreview
)
