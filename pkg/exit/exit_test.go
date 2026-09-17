package exit

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitRequested(t *testing.T) {
	t.Cleanup(ClearExitRequested)

	assert.False(t, ExitRequested())
	SetExitRequested()
	assert.True(t, ExitRequested())

	ClearExitRequested()
	err := fmt.Errorf("test error")
	SetExitRequestedWithError(err)
	assert.True(t, ExitRequested())
	assert.Equal(t, err, exitError)
}
