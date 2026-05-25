package exercises

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// Discover walks the directory containing the BDP distribution looking for
// folders matching "Ejercicio_*". For each one it detects which template
// to apply (currently: wordcount via mapper.py+reducer.py heuristic) and
// builds the full Exercise struct with auto-generated playbook.
//
// Returns exercises sorted by ID for stable UI ordering. Folders that
// don't match any known template are still returned with Template="raw"
// so the student can at least browse the files.
func Discover(p *paths.Paths) []Exercise {
	parent := filepath.Dir(p.ScriptDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}
	var out []Exercise
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "Ejercicio_") {
			continue
		}
		ex := loadExercise(p, filepath.Join(parent, name), name)
		if ex != nil {
			out = append(out, *ex)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func loadExercise(p *paths.Paths, dir, name string) *Exercise {
	id := strings.ToLower(strings.TrimPrefix(name, "Ejercicio_"))
	if id == "" {
		return nil
	}
	id = "ej" + id

	files, _ := os.ReadDir(dir)
	fileNames := make([]string, 0, len(files))
	hasMapper := false
	hasReducer := false
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		fileNames = append(fileNames, f.Name())
		switch f.Name() {
		case "mapper.py":
			hasMapper = true
		case "reducer.py":
			hasReducer = true
		}
	}
	sort.Strings(fileNames)

	if hasMapper && hasReducer {
		return wordCountExercise(p, id, name, dir, fileNames)
	}
	// Fall-through: unknown layout, show files but no auto-playbook.
	return &Exercise{
		ID:          id,
		Title:       name,
		Description: "Ejercicio sin plantilla detectada. Abre los archivos manualmente y consulta el README/PDF dentro de la carpeta.",
		Path:        dir,
		Template:    "raw",
		Files:       fileNames,
	}
}
