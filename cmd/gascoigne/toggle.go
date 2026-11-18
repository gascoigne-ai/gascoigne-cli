package main

import "os"

// A toggle is an on or off answer. Saved prefs and setup answers keep these
// strings on disk and on the wire.
type toggle string

const (
	toggleOn  toggle = "on"
	toggleOff toggle = "off"
)

// toggleChoices are the valid answers to an on/off flag.
var toggleChoices = []string{string(toggleOn), string(toggleOff)}

// envOff reports an environment switch set to off.
func envOff(name string) bool { return toggle(os.Getenv(name)) == toggleOff }
