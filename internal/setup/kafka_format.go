package setup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runKafka generates a fresh KRaft cluster id and runs kafka-storage format
// against the bundled server.properties. The legacy Windows setup script
// used PowerShell to generate the UUID; we use crypto/rand directly so the
// cross-platform path is identical.
func (o *Orchestrator) runKafka(ctx context.Context) error {
	uuid, err := generateKafkaUUID()
	if err != nil {
		return fmt.Errorf("generar UUID: %w", err)
	}
	o.sink.Emit("INFO", "UUID Kafka generado: "+uuid)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	env := os.Environ()
	env = appendEnv(env, "JAVA_HOME", o.p.CommonJDK)

	cmd := exec.CommandContext(ctx, o.p.KafkaStorageCommand(), "format", "-t", uuid, "-c", o.p.KafkaConfig())
	cmd.Env = env
	cmd.Dir = o.p.Kafka
	cmd.SysProcAttr = hideWindowAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("kafka-storage start: %w", err)
	}
	go streamLines(stdout, func(s string) { o.sink.Emit("INFO", s) })
	go streamLines(stderr, func(s string) { o.sink.Emit("INFO", s) })

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("kafka-storage wait: %w", err)
	}
	return nil
}

// generateKafkaUUID returns a 22-character URL-safe base64 string derived
// from 16 random bytes — the exact shape Kafka KRaft expects.
func generateKafkaUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	s := base64.URLEncoding.EncodeToString(b[:])
	return strings.TrimRight(s, "="), nil
}
