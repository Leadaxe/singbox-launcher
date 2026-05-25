package traffic

import "sync"

// Singleton instance — one TrafficProfiler per app process. Created on
// first GetInstance call; started by the UI/app wiring once the Clash
// config and log path are known.
var (
	instOnce sync.Once
	inst     *TrafficProfiler
)

// GetInstance returns the process-wide TrafficProfiler. Safe to call
// from any goroutine; the first call constructs, subsequent calls return
// the same pointer.
func GetInstance() *TrafficProfiler {
	instOnce.Do(func() {
		inst = NewTrafficProfiler()
	})
	return inst
}
