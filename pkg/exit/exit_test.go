package exit

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
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
	assert.Error(t, err)
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
