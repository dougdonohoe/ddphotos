package exit

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitRequestedAndCatchPanic(t *testing.T) {
	var cleaned bool

	cb := func() {
		cleaned = true
	}

	assert.False(t, ExitRequested())
	SetExitRequested()
	assert.True(t, ExitRequested())

	ClearExitRequested()
	err := fmt.Errorf("test error")
	SetExitRequestedWithError(err)
	assert.True(t, ExitRequested())
	assert.Equal(t, err, exitError)

	ClearExitRequested()
	SetCleanupCallback(cb)
	assert.False(t, ExitRequested())
	boom()
	assert.True(t, ExitRequested())
	assert.True(t, cleaned)
}

func boom() {
	defer CatchPanic()
	panic("boom")
}

func TestCatchPanicError(t *testing.T) {
	assert.Nil(t, boom2(false))
	err := boom2(true)
	// require, not assert: assert would carry on and panic on err.Error() below.
	require.Error(t, err)
	assert.Equal(t, "panic: boom2", err.Error())
	fmt.Printf("Error: %s\n", err)
}

func boom2(goBoom bool) (err error) {
	defer CatchPanicError(&err)
	if goBoom {
		panic("boom2")
	}
	return nil
}
