//go:build e2e
// +build e2e

package services

import (
	"context"
	"os/exec"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// newJavaCmd builds the os/exec.Cmd used by the E2E tests. Lives in its own
// file so the platform-specific HideWindow attribute can be added later
// without changing the test bodies.
func newJavaCmd(ctx context.Context, p *paths.Paths, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, p.JavaBinary(), args...)
	cmd.Dir = p.Kafka
	cmd.Env = append(cmd.Env, "JAVA_HOME="+p.CommonJDK)
	return cmd
}
