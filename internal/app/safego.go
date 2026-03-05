package app

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// safeGo launches a goroutine with panic recovery and WaitGroup tracking.
// If the goroutine panics, the panic is logged and onPanic is called (if non-nil).
// The WaitGroup is incremented before launch and decremented when the goroutine exits.
func safeGo(wg *sync.WaitGroup, name string, onPanic func(name string, r interface{}), fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[%s] [PANIC] goroutine %q: %v\n%s\n",
					time.Now().Format("15:04:05.000"), name, r, debug.Stack())
				if onPanic != nil {
					onPanic(name, r)
				}
			}
		}()
		fn()
	}()
}

// SafeGo launches a tracked background goroutine on the App's WaitGroup.
// The goroutine is recovered on panic and drained during Stop().
// Use this from external callers (e.g., daemon.go) that need tracked goroutines.
func (a *App) SafeGo(name string, fn func()) {
	safeGo(&a.bgWg, name, nil, fn)
}

// StopCh returns a channel that is closed when Stop() is called.
// Background goroutines should select on this to exit promptly.
func (a *App) StopCh() <-chan struct{} {
	return a.stopCh
}
