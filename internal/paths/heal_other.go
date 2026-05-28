//go:build !darwin

package paths

// HealReport keeps a stable shape on all platforms so callers don't need
// build tags to consume the result of HealDarwin.
type HealReport struct {
	Actions []string
	Errors  []string
}

// HealDarwin is a no-op on non-darwin hosts: only macOS bundles need the
// exFAT / quarantine heal. Returns an empty report.
func (p *Paths) HealDarwin() HealReport { return HealReport{} }
