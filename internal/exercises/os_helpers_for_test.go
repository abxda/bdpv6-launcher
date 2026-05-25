package exercises

import "os"

// osMkdir / osWrite are tiny shims used by template_wordcount_test.go so
// the test file itself doesn't need to import "os" twice. Kept here to
// avoid noise in the main package files.
func osMkdir(p string) error             { return os.MkdirAll(p, 0o755) }
func osWrite(p string, b []byte) error   { return os.WriteFile(p, b, 0o644) }
