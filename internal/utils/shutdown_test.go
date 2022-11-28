package utils

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsShuttingDown(t *testing.T) {
	NewLogger("testing")
	assert.Equal(t, false, IsShuttingDown(), "initial value")
	err := syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	if err != nil {
		t.Error(err)
	}
	WaitForShutdownSignal()
	assert.Equal(t, true, IsShuttingDown(), "SIGINT signal trap")
}
