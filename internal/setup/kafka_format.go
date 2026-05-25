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
//
// Before formatting we wipe any pre-existing data_kraft directory. Kafka's
// kafka-storage format refuses a directory that already has a meta.properties
// (it would conflict with the new cluster id) and the on-disk state is
// useless anyway once we reformat — the broker can only ever talk to its
// own cluster id, so a stale dir guarantees a DUPLICATE_BROKER_REGISTRATION
// loop on next start.
func (o *Orchestrator) runKafka(ctx context.Context) error {
	uuid, err := generateKafkaUUID()
	if err != nil {
		return fmt.Errorf("generar UUID: %w", err)
	}
	o.sink.Emit("INFO", "UUID Kafka generado: "+uuid)

	// Wipe stale KRaft data so the new cluster id takes effect.
	dataKraft := filepath.Join(o.p.Kafka, "data", "data_kraft")
	if _, statErr := os.Stat(dataKraft); statErr == nil {
		o.sink.Emit("INFO", "Limpiando "+dataKraft+" antes de reformatear…")
		if rmErr := os.RemoveAll(dataKraft); rmErr != nil {
			o.sink.Emit("WARN", "No pude limpiar "+dataKraft+": "+rmErr.Error())
		}
	}

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
//
// We re-roll if the first character is '-' or '_'. Both are valid in the
// URL-safe base64 alphabet but kafka-storage's argparse-based CLI sees a
// value like "-xj_dtZK…" as another flag, not as the value of -t, and
// dies with "argument --cluster-id/-t: expected one argument". Trying a
// new random ID until the first char is alphanumeric solves it without
// changing the alphabet or breaking compatibility with what Kafka expects.
func generateKafkaUUID() (string, error) {
	for i := 0; i < 8; i++ {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		s := strings.TrimRight(base64.URLEncoding.EncodeToString(b[:]), "=")
		if len(s) > 0 && s[0] != '-' && s[0] != '_' {
			return s, nil
		}
	}
	// 8 consecutive coin flips landing on '-'/'_' is essentially impossible
	// (probability ~(2/64)^8) but if it happens we still need to return
	// something; just prepend an 'A' so kafka-storage parses it as a value.
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	s := strings.TrimRight(base64.URLEncoding.EncodeToString(b[:]), "=")
	return "A" + s[1:], nil
}
