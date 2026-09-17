package exit

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
)

var (
	// A flag rather than a counter, so atomic.Bool says what it is and needs no 0/1
	// comparison at the call sites.
	exitRequested atomic.Bool
	exitError     error
	exitLock      sync.Mutex
)

// ExitRequested returns true if exit has been requested.
//
//goland:noinspection GoNameStartsWithPackageName
func ExitRequested() bool {
	return exitRequested.Load()
}

// SetExitRequested requests a graceful exit.
func SetExitRequested() {
	exitRequested.Store(true)
}

// SetExitRequestedWithError requests a graceful exit and records err,
// which causes a non-zero exit status.
func SetExitRequestedWithError(err error) {
	SetExitRequested()
	exitLock.Lock()
	defer exitLock.Unlock()
	exitError = err
}

// Fatal prints an error message and exits with a non-zero status.
// If err is non-nil, it prints "msg: err"; otherwise it prints msg.
func Fatal(msg string, err error) {
	if err != nil {
		fmt.Printf("%s: %s\n", msg, err)
	} else {
		fmt.Println(msg)
		err = errors.New(msg)
	}
	ExitWithStatus(err)
}

// ExitWithStatus exits with status 0 if err is nil and no prior error was recorded, otherwise 1.
//
//goland:noinspection GoNameStartsWithPackageName
func ExitWithStatus(err error) {
	exitLock.Lock()
	defer exitLock.Unlock()
	code := 0
	if err != nil || exitError != nil {
		code = 1
	}
	os.Exit(code)
}

// PanicOnError panics if err is non-nil.
func PanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

// HandleSignal listens for CTRL-C (SIGINT) and requests a graceful exit. Work in progress
// checks ExitRequested and stops; it does not record an error, so an error already set by
// SetExitRequestedWithError survives and still decides the exit status.
func HandleSignal() {
	signals := make(chan os.Signal, 1)
	// NOTE: was catching syscall.SIGPIPE to allow use of 'tee',
	//       but was getting spurious errors, so removed it.
	signal.Notify(signals, os.Interrupt)

	go func() {
		sig := <-signals
		fmt.Printf("\n\n*** Signal '%s' detected, exiting... ***\n\n", sig)
		SetExitRequested()
	}()
}

// ClearExitRequested clears the exit-requested flag (useful in unit tests).
func ClearExitRequested() {
	exitRequested.Store(false)
}
