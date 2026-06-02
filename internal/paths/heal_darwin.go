//go:build darwin

package paths

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HealReport summarises what the macOS startup heal did. Empty Actions
// means the distro layout was already intact; Errors holds non-fatal
// failures (the launcher continues either way and surfaces them in logs).
type HealReport struct {
	Actions []string
	Errors  []string
}

// HealDarwin restores the Unix features (exec bits, named-version symlinks,
// quarantine state) that exFAT, .zip-via-Archive-Utility and similar
// transports strip when the BDP distribution is moved across filesystems.
// Each step is probe-then-fix: cheap when the distro is already healthy,
// recursive only when a marker file proves the fix is needed. Idempotent —
// safe to call on every startup.
//
// Called from app.startup() before any service launch. Errors are recorded
// but non-fatal: a partial heal is still better than refusing to boot, and
// the user can re-run via the repair UI.
func (p *Paths) HealDarwin() HealReport {
	var r HealReport
	p.healExecBits(&r)
	p.healPythonWrappers(&r)
	p.healLibsqlite3(&r)
	p.healQuarantine(&r)
	return r
}

// healExecBits restores u+x across each service's binary trees when the
// canonical probe binary lost its exec bit (the symptom of an exFAT or
// Windows-zip transport). Each service lists ALL trees that ship executables,
// not just bin/: e.g. Elasticsearch bundles its own JDK under jdk/ and native
// CLIs under modules/ + lib/ — chmod'ing only elasticsearch/bin leaves
// es bundled `java` non-executable and ES fails to start. Coverage mirrors the
// chmod sweep in bootstrap_mac.sh. The walk is skipped on healthy installs
// (probe already executable), so this is cheap on a clean tar extract.
func (p *Paths) healExecBits(r *HealReport) {
	type probe struct {
		marker string   // if this canonical binary lost +x, the trees need heal
		trees  []string // directories to chmod -R u+x
	}
	probes := []probe{
		{filepath.Join(p.CommonJDK, "bin", "java"), []string{p.CommonJDK}},
		{filepath.Join(p.Hadoop, "bin", "hdfs"), []string{
			filepath.Join(p.Hadoop, "bin"), filepath.Join(p.Hadoop, "sbin"), filepath.Join(p.Hadoop, "libexec"),
		}},
		{filepath.Join(p.Kafka, "bin", "kafka-server-start.sh"), []string{filepath.Join(p.Kafka, "bin")}},
		{filepath.Join(p.Elastic, "bin", "elasticsearch"), []string{
			filepath.Join(p.Elastic, "bin"), filepath.Join(p.Elastic, "jdk"),
			filepath.Join(p.Elastic, "modules"), filepath.Join(p.Elastic, "lib"),
		}},
		{filepath.Join(p.Python, "bin", "python3.10"), []string{filepath.Join(p.Python, "bin")}},
		{filepath.Join(p.Spark, "bin", "spark-submit"), []string{
			filepath.Join(p.Spark, "bin"), filepath.Join(p.Spark, "sbin"),
		}},
	}
	for _, pr := range probes {
		if !fileExists(pr.marker) || isExecutable(pr.marker) {
			continue
		}
		for _, tree := range pr.trees {
			if !isDir(tree) {
				continue
			}
			if err := chmodExecTree(tree); err != nil {
				r.Errors = append(r.Errors, fmt.Sprintf("chmod %s: %v", tree, err))
				continue
			}
			r.Actions = append(r.Actions, "restored exec bits under "+tree)
		}
	}
}

// healPythonWrappers writes the python and python3 shims next to
// python3.10. The launcher and the Hadoop Streaming template invoke
// `python` and `python3` literally; the embedded interpreter only ships
// python3.10. We copy a one-line bash wrapper instead of a symlink because
// exFAT's symlink support is the source of half the bugs this file exists
// to repair.
func (p *Paths) healPythonWrappers(r *HealReport) {
	pybin := filepath.Join(p.Python, "bin")
	src := filepath.Join(pybin, "python3.10")
	if !fileExists(src) {
		return
	}
	const shim = "#!/bin/bash\nexec \"$(dirname \"$0\")/python3.10\" \"$@\"\n"
	for _, name := range []string{"python", "python3"} {
		dst := filepath.Join(pybin, name)
		if fileExists(dst) {
			continue
		}
		if err := os.WriteFile(dst, []byte(shim), 0o755); err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("write %s: %v", dst, err))
			continue
		}
		r.Actions = append(r.Actions, "created wrapper "+name)
	}
}

// healLibsqlite3 recreates the unversioned libsqlite3.dylib that the
// embedded Python's _sqlite3 module loads via @rpath. Without it dyld falls
// back to the macOS system libsqlite3 (Apple builds it without extension
// loading) and Jupyter crashes on import with
// "Symbol not found: _sqlite3_enable_load_extension". Copy from the
// versioned dylib that ships with the distro; do not symlink (exFAT eats
// them — that is precisely the bug this function exists to repair).
func (p *Paths) healLibsqlite3(r *HealReport) {
	lib := filepath.Join(p.Python, "lib")
	dst := filepath.Join(lib, "libsqlite3.dylib")
	if fileExists(dst) {
		return
	}
	entries, err := os.ReadDir(lib)
	if err != nil {
		return
	}
	var src string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "libsqlite3.") || !strings.HasSuffix(n, ".dylib") || n == "libsqlite3.dylib" {
			continue
		}
		src = filepath.Join(lib, n)
		break
	}
	if src == "" {
		return
	}
	if err := copyRegularFile(src, dst); err != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("copy libsqlite3.dylib: %v", err))
		return
	}
	r.Actions = append(r.Actions, "restored libsqlite3.dylib from "+filepath.Base(src))
}

// healQuarantine strips the com.apple.quarantine xattr from the distro
// tree when the probe binary still carries it. Without this, Gatekeeper
// blocks Hadoop / Java / Python from running with no useful error in the
// launcher log. We shell out to xattr(1) so the recursive walk uses the
// system tool's fast implementation rather than reimplementing it in Go.
// Note: this only heals the distro contents — the launcher's own .app
// must be released by Gatekeeper before any of our code runs at all.
func (p *Paths) healQuarantine(r *HealReport) {
	probe := filepath.Join(p.CommonJDK, "bin", "java")
	if !fileExists(probe) {
		return
	}
	out, err := exec.Command("xattr", probe).Output()
	if err != nil || !strings.Contains(string(out), "com.apple.quarantine") {
		return
	}
	if err := exec.Command("xattr", "-dr", "com.apple.quarantine", p.ScriptDir).Run(); err != nil {
		r.Errors = append(r.Errors, "xattr -dr: "+err.Error())
		return
	}
	r.Actions = append(r.Actions, "removed com.apple.quarantine from distro")
}

// --- helpers (darwin-only) -----------------------------------------------

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&0o111 != 0
}

// chmodExecTree adds u+x to every entry under root. Directories also get
// +x so traversal works. Per-entry chmod errors are swallowed: the caller
// already decided this tree needs heal and a partial chmod is fine.
func chmodExecTree(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		_ = os.Chmod(path, info.Mode()|0o100)
		return nil
	})
}

func copyRegularFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
