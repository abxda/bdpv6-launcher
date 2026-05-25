// Package exercises models a self-contained student exercise (dataset +
// scripts + step-by-step playbook) and discovers them on disk alongside
// the BDP distribution. The discovery is convention-based so the teacher
// can drop a new Ejercicio_NN/ folder and have it appear in the launcher
// without recompiling anything.
package exercises

// Exercise describes one playable exercise. Steps are the ordered list of
// shell commands the launcher will run on the student's behalf, each one
// tagged with a short Spanish title and a longer description suitable for
// teaching context.
type Exercise struct {
	ID          string `json:"id"`          // stable machine id, e.g. "ej01"
	Title       string `json:"title"`       // user-facing label
	Description string `json:"description"` // 1-3 sentence what-and-why
	Path        string `json:"path"`        // absolute path to the exercise dir
	Template    string `json:"template"`    // detected template id ("wordcount", "raw", …)
	Requires    []string `json:"requires"`  // service ids that must be running (e.g. "hdfs_namenode")
	Files       []string `json:"files"`     // notable files in the dir (.csv, .py, .pdf)
	Steps       []Step `json:"steps"`
}

// Step is one runnable unit. Cmd is the binary; Args is the argument list
// (already split — we do NOT shell-parse a freeform string because that's
// where injection bugs come from). Notes is shown above the command in
// the UI so the student understands WHY this step exists.
type Step struct {
	Title string   `json:"title"`
	Notes string   `json:"notes"`
	Cmd   string   `json:"cmd"`   // e.g. "hdfs" or "hadoop"
	Args  []string `json:"args"`
	// Shell, if true, runs the step through `cmd /c` (Windows) or
	// `bash -lc` (Unix) so pipes and redirection work. Use sparingly —
	// most steps should be a clean exec of a single binary.
	Shell bool `json:"shell"`
	// PrintAs, if set, is the cosmetic command string shown in the UI
	// console before the real one runs. Lets us show pretty multi-line
	// commands without affecting execution.
	PrintAs string `json:"printAs,omitempty"`
}
