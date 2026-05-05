package utils

import (
	"sync"
	"time"
)

// Waits for the lock for the specified max timeout.
// Returns false if waiting timed out.
func LockTimeout(mx *sync.Mutex, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		mx.Lock()
	}()
	select {
	case <-c:
		return true // completed normally
	case <-time.After(timeout):
		return false // timed out
	}
}

// Waits for the waitgroup for the specified max timeout.
// Returns false if waiting timed out.
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return true // completed normally
	case <-time.After(timeout):
		return false // timed out
	}
}

// Invoke the given function and waits for it to complete 
// within the specified timeout. Returns false if function
// call times out. If timeout is 0 then call will wait
// indefinitely until function call completes.
func InvokeWithTimeout(fn func(), timeout time.Duration) bool {

	if timeout == 0 {
		fn()
		return true

	} else {
		c := make(chan struct{})
		go func() {
			defer close(c)
			fn()
		}()
		select {
		case <-c:
			return true // completed normally
		case <-time.After(timeout):
			return false // timed out
		}	
	}
}
