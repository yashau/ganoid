package tray

// NewRebuildChan returns a buffered channel and a non-blocking notify function.
// The tray calls notify whenever the daemon connection state changes.
func NewRebuildChan() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	notify := func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return ch, notify
}
