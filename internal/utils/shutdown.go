package utils

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

var shutdown atomic.Bool

var waitForShutdown chan struct{}

func init() {
	shutdown.Store(false)
	waitForShutdown = make(chan struct{})

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c,
			// https://www.gnu.org/software/libc/manual/html_node/Termination-Signals.html
			syscall.SIGTERM, // "the normal way to politely ask a program to terminate"
			syscall.SIGINT,  // Ctrl+C
			syscall.SIGQUIT, // Ctrl-\
			syscall.SIGKILL, // "always fatal", "SIGKILL and SIGSTOP may not be caught by a program"
			syscall.SIGHUP,  // "terminal is disconnected"
		)
		<-c
		shutdown.Store(true)
		close(waitForShutdown)
	}()
}

func IsShuttingDown() bool {
	return shutdown.Load()
}

func WaitForShutdownSignal() {
	logger.Debug("Shutdown signal wait started")
	<-waitForShutdown
	logger.Info("Shutdown signal received")
}
