package setup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runKafka generates a fresh KRaft cluster id and runs the storage tool to
// format server.properties.
//
// We bypass kafka-storage.bat entirely and invoke `java` directly with the
// wildcard classpath. Reason: the bundled .bat builds a CLASSPATH from
// every jar in kafka_kraft/libs/ and bakes it onto the command line, which
// pushes the total past cmd.exe's 8191-char limit and dies with
// "La sintaxis del comando no es correcta." before Java even starts. The
// wildcard form `libs/*` keeps the command line short and works on both
// Windows and macOS without depending on the platform-specific wrappers.
func (o *Orchestrator) runKafka(ctx context.Context) error {
	uuid, err := generateKafkaUUID()
	if err != nil {
		return fmt.Errorf("generar UUID: %w", err)
	}
	o.sink.Emit("INFO", "UUID Kafka generado: "+uuid)

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	classpath := filepath.Join(o.p.Kafka, "libs", "*")
	log4j := filepath.Join(o.p.Kafka, "config", "tools-log4j.properties")

	env := os.Environ()
	env = appendEnv(env, "JAVA_HOME", o.p.CommonJDK)

	args := []string{
		"-Xms256m", "-Xmx256m",
		"-Dlog4j.configuration=file:" + filepath.ToSlash(log4j),
		"-classpath", classpath,
		"kafka.tools.StorageTool",
		"format", "-t", uuid, "-c", o.p.KafkaConfig(),
	}

	cmd := exec.CommandContext(ctx, o.p.JavaBinary(), args...)
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
