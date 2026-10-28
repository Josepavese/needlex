package platform

import (
	"os"
	"os/signal"
	"syscall"
)

// WatchShutdown owns the signal subscription and its worker. Stop waits for
// any in-progress callback; normal Stop never invokes the shutdown callback.
func WatchShutdown(shutdown func()) (stop func()) {
	signals := make(chan os.Signal, 1)
	done, stopped := make(chan struct{}), make(chan struct{})
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer close(stopped)
		select {
		case <-signals:
			shutdown()
		case <-done:
		}
	}()
	return func() { signal.Stop(signals); close(done); <-stopped }
}
