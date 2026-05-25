package setup

import "testing"

// TestGenerateKafkaUUID_NeverStartsWithFlagChar is the regression test for
// the issue where a randomly generated cluster id like "-xj_dtZK…" caused
// kafka-storage to fail with:
//   kafka-storage: error: argument --cluster-id/-t: expected one argument
//
// Both '-' and '_' are valid in the URL-safe base64 alphabet but argparse
// in the Kafka tools CLI treats a value starting with '-' as a new flag.
// We re-roll until the first char is alphanumeric. 1000 iterations covers
// the ~3% probability per draw of hitting a leading '-' or '_'.
func TestGenerateKafkaUUID_NeverStartsWithFlagChar(t *testing.T) {
	for i := 0; i < 1000; i++ {
		uuid, err := generateKafkaUUID()
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if len(uuid) < 22 {
			t.Errorf("attempt %d: uuid %q too short", i, uuid)
		}
		if uuid[0] == '-' || uuid[0] == '_' {
			t.Errorf("attempt %d: uuid %q starts with %q — would break kafka-storage CLI", i, uuid, string(uuid[0]))
		}
	}
}
