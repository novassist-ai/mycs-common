package run

// os specific interrupt handler where additional handling is
// required to trap ctrl-c events. does nothing for *nix platforms.
// handler should return true if event is handled and propagation
// to other handlers in chaing should stop.
var HandleInterruptEvent = func(handler func() bool) error { return nil }
