package exit

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

var (
	exitRequested   int32
	exitError       error
	cleanupCalled   bool
	cleanupLock     sync.Mutex
	cleanupCallback func()
)

// Return true if exit has been requested
//
//goland:noinspection GoNameStartsWithPackageName
func ExitRequested() bool {
	return atomic.LoadInt32(&exitRequested) == 1
}

// Request exit
func SetExitRequested() {
	atomic.StoreInt32(&exitRequested, 1)
}

// Request exit, setting exitError, which causes non-zero status
// upon exit
func SetExitRequestedWithError(err error) {
	SetExitRequested()
	cleanupLock.Lock()
	defer cleanupLock.Unlock()
	exitError = err
}

// Callback to call if signal received or panic handled
func SetCleanupCallback(cb func()) {
	cleanupCallback = cb
}

// Exit with status 0 if err is nil and no panic, otherwise 1
func ExitWithStatus(err error) {
	cleanupLock.Lock()
	defer cleanupLock.Unlock()
	code := 0
	if err != nil || exitError != nil {
		code = 1
	}
	os.Exit(code)
}

// Catch panic - prints error and triggers an exit, which
// calls the cleanup callback, if set.  Note that any values
// returned from the enclosing method are set to the default
// values (e.g., bool is false, error is nil). Thus a method
// that returns true to continue works well with this:
//
//	func process() bool {
//	   defer CatchPanic()
//	   ...
//	}
//
// This will automatically return false on panic.
func CatchPanic() {
	if r := recover(); r != nil {
		fmt.Printf("PANIC %v\n%s", r, string(debug.Stack()))
		exitTriggered(fmt.Errorf("panic: %v", r))
	}
}

// Catch panic - prints error and sets error message into given error variable.
// Common use case is to set into a named return variable:
//
//	func broken() (err error) {
//	   defer exit.CatchPanicError(&err)
//	   ...
//	}
func CatchPanicError(err *error) {
	if r := recover(); r != nil {
		fmt.Printf("PANIC %v\n%s", r, string(debug.Stack()))
		*err = fmt.Errorf("panic: %v", r)
	}
}

// panic if err != nil
func PanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

// listen for CTRL-C so we can gracefully cleanup.  Calls cleanup callback, if set
func HandleSignal() {
	signals := make(chan os.Signal, 1)
	// NOTE: was catching syscall.SIGPIPE to allow use of 'tee',
	//       but was getting spurious errors, so removed it.
	signal.Notify(signals, os.Interrupt)

	go func() {
		sig := <-signals
		fmt.Printf("\n\n*** Signal '%s' detected, exiting... ***\n\n", sig)
		exitTriggered(nil)
	}()
}

// Clear ExitRequested flag (useful in unit tests)
func ClearExitRequested() {
	atomic.StoreInt32(&exitRequested, 0)
}

func exitTriggered(err error) {
	cleanupLock.Lock()
	defer cleanupLock.Unlock()
	SetExitRequested()
	exitError = err
	if cleanupCallback != nil && !cleanupCalled {
		cleanupCallback()
		cleanupCalled = true
	}
}
