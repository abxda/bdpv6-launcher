// Package sysinfo wraps gopsutil to provide cheap CPU/RAM samples for the
// dashboard widgets. cpu.Percent without an interval returns the average
// since boot on the first call, so the launcher takes a baseline at start
// and the foreground sampler uses a non-blocking 0 interval thereafter.
package sysinfo

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Sample is the snapshot emitted to the frontend on every tick.
type Sample struct {
	CPUPercent float64 `json:"cpuPercent"`
	RAMPercent float64 `json:"ramPercent"`
	RAMUsedMB  uint64  `json:"ramUsedMB"`
	RAMTotalMB uint64  `json:"ramTotalMB"`
}

// Warm primes gopsutil's CPU sampler by issuing a blocking call so the next
// non-blocking call returns a meaningful value rather than the boot
// average. Call this once at startup.
func Warm() { _, _ = cpu.Percent(150*time.Millisecond, false) }

// Now returns a fresh Sample. Errors are squashed; the UI prefers stale-but-
// valid numbers over noisy zeros.
func Now() Sample {
	s := Sample{}
	if vals, err := cpu.Percent(0, false); err == nil && len(vals) > 0 {
		s.CPUPercent = round1(vals[0])
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.RAMPercent = round1(vm.UsedPercent)
		s.RAMUsedMB = vm.Used / (1024 * 1024)
		s.RAMTotalMB = vm.Total / (1024 * 1024)
	}
	return s
}

// Tick samples every `interval` and invokes `emit` until ctx is cancelled.
func Tick(ctx context.Context, interval time.Duration, emit func(Sample)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			emit(Now())
		}
	}
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
