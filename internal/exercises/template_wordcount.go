package exercises

import (
	"path/filepath"
	"strings"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// wordCountExercise builds the playbook for a Hadoop Streaming WordCount
// exercise — the classic "count tokens in a CSV" assignment. Triggered
// when discovery finds both mapper.py and reducer.py in the folder.
//
// Steps are intentionally split fine-grained so the student can run them
// one at a time and SEE the output of each: prepare HDFS dir → upload
// data → rm old output → run streaming jar → top-N results. Each step
// is shown with a brief teacher's note explaining WHY, not just WHAT.
func wordCountExercise(p *paths.Paths, id, name, dir string, files []string) *Exercise {
	hdfsRoot := "/" + id
	hdfsInput := hdfsRoot + "/input"
	hdfsOutput := hdfsRoot + "/output"

	// Locate the streaming jar. Hadoop bundles it as
	//   share/hadoop/tools/lib/hadoop-streaming-<version>.jar
	// We glob to avoid hard-coding the version.
	streamingJar := findStreamingJar(p)

	// Find the .csv to upload. If there are several, we pick the first
	// alphabetically (deterministic; rare to have >1 in a single exercise).
	dataset := ""
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".csv") {
			dataset = f
			break
		}
	}
	if dataset == "" {
		dataset = "input.txt"
	}

	// Bash binary for shell-piped steps. On Windows we look for the
	// bundled GitPortable copy first (bash/usr/bin/bash.exe), falling
	// back to system bash. On Unix we just use /bin/bash since every
	// supported distro ships it.
	bashBin := resolveBash(p)

	steps := []Step{
		{
			Title: "1 · Esperar a que HDFS salga de safe mode",
			Notes: "Cuando el NameNode arranca, entra en 'safe mode' por ~30s mientras carga fsimage y los DataNodes reportan sus bloques. Durante ese tiempo HDFS rechaza writes (put / mkdir / rm). Este comando bloquea hasta que safe mode termine — si HDFS ya está listo, retorna al instante.",
			Cmd:   p.HdfsCommand(),
			Args:  []string{"dfsadmin", "-safemode", "wait"},
		},
		{
			Title: "2 · Preparar directorio de entrada en HDFS",
			Notes: "Crea " + hdfsInput + " si todavía no existe. La bandera -p evita que falle si ya estaba.",
			Cmd:   p.HdfsCommand(),
			Args:  []string{"dfs", "-mkdir", "-p", hdfsInput},
		},
		{
			Title: "3 · Subir el dataset (" + dataset + ") a HDFS",
			Notes: "Copia el archivo local al directorio recién creado. -f sobrescribe si ya estaba ahí desde una corrida anterior.",
			Cmd:   p.HdfsCommand(),
			Args:  []string{"dfs", "-put", "-f", filepath.Join(dir, dataset), hdfsInput + "/"},
		},
		{
			Title: "4 · Borrar output anterior (idempotente)",
			Notes: "Hadoop se niega a escribir sobre una carpeta de salida que ya existe. La borramos por si ejecutas el job más de una vez.",
			Cmd:   p.HdfsCommand(),
			Args:  []string{"dfs", "-rm", "-r", "-f", "-skipTrash", hdfsOutput},
		},
		{
			Title: "5 · Ejecutar el job MapReduce vía Hadoop Streaming",
			Notes: "Hadoop Streaming envía cada línea del input al stdin del mapper y el stdout del mapper al stdin del reducer. Forzamos -Dmapreduce.framework.name=local porque esta distribución NO incluye YARN; el LocalJobRunner corre el job dentro del JVM del cliente. Pre-borramos el output (por si corres este paso suelto sin haber pasado por el paso 4) para que sea idempotente — Hadoop se niega a sobrescribir un output existente.",
			Cmd:   bashBin,
			Shell: true,
			Args: []string{
				"-lc",
				// Safety pre-clean (idempotent re-runs) + the actual job.
				// All on one bash invocation so the runner reports a single
				// success/fail for the logical "run the MR" action.
				p.HdfsCommand() + " dfs -rm -r -f -skipTrash " + hdfsOutput + " 2>/dev/null ; " +
					filepath.ToSlash(filepath.Join(p.Hadoop, "bin", hadoopScript())) + " jar " +
					filepath.ToSlash(streamingJar) + " " +
					"-Dmapreduce.framework.name=local " +
					"-Dmapreduce.jobtracker.address=local " +
					"-mapper \"python " + filepath.ToSlash(filepath.Join(dir, "mapper.py")) + "\" " +
					"-reducer \"python " + filepath.ToSlash(filepath.Join(dir, "reducer.py")) + "\" " +
					"-input " + hdfsInput + " -output " + hdfsOutput,
			},
			PrintAs: "# Safety pre-clean (idempotent) + el job:\n" +
				"hdfs dfs -rm -r -f -skipTrash " + hdfsOutput + " 2>/dev/null\n" +
				"hadoop jar hadoop-streaming.jar \\\n" +
				"  -Dmapreduce.framework.name=local \\\n" +
				"  -Dmapreduce.jobtracker.address=local \\\n" +
				"  -mapper  \"python " + filepath.ToSlash(filepath.Join(dir, "mapper.py")) + "\" \\\n" +
				"  -reducer \"python " + filepath.ToSlash(filepath.Join(dir, "reducer.py")) + "\" \\\n" +
				"  -input " + hdfsInput + " -output " + hdfsOutput,
		},
		{
			Title: "6 · Top-20 palabras más frecuentes",
			Notes: "Leemos la salida del reducer (part-00000), ordenamos por la columna 2 numérica descendente, y mostramos los 20 primeros tokens.",
			Cmd:   bashBin,
			Args:  []string{"-lc", p.HdfsCommand() + " dfs -cat " + hdfsOutput + "/part-00000 | sort -t$'\\t' -k2 -nr | head -20"},
			Shell: true,
			PrintAs: "hdfs dfs -cat " + hdfsOutput + "/part-00000 \\\n" +
				"  | sort -t$'\\t' -k2 -nr | head -20",
		},
	}

	return &Exercise{
		ID:          id,
		Title:       name + " — WordCount con Hadoop Streaming",
		Description: "Cuenta cuántas veces aparece cada palabra en " + dataset + " usando un MapReduce clásico. El mapper emite (palabra, 1) por cada token; el reducer suma las repeticiones agrupadas por palabra (Hadoop ordena las claves automáticamente antes de pasarlas al reducer).",
		Path:        dir,
		Template:    "wordcount",
		Requires:    []string{"hdfs_namenode", "hdfs_datanode"},
		Files:       files,
		Steps:       steps,
	}
}

// findStreamingJar globs share/hadoop/tools/lib/hadoop-streaming-*.jar so
// we don't hard-code the Hadoop version into the launcher.
func findStreamingJar(p *paths.Paths) string {
	pattern := filepath.Join(p.Hadoop, "share", "hadoop", "tools", "lib", "hadoop-streaming-*.jar")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0]
	}
	// Best-effort fallback — the runner will error clearly if it doesn't exist.
	return filepath.Join(p.Hadoop, "share", "hadoop", "tools", "lib", "hadoop-streaming.jar")
}

// hadoopScript returns the right wrapper name per OS.
func hadoopScript() string {
	if isWindows() {
		return "hadoop.cmd"
	}
	return "hadoop"
}

// toFileURI turns an absolute local filesystem path into a file:/// URI
// the way Hadoop's GenericOptionsParser expects:
//
//	Windows: D:\foo\bar.py   → file:///D:/foo/bar.py
//	Unix:    /home/x/bar.py  → file:///home/x/bar.py
//
// Without this conversion the URI parser blows up at the first backslash
// in a Windows path, treating "D:" as an opaque-scheme prefix:
//
//	java.net.URISyntaxException: Illegal character in opaque part at
//	  index 2: D:\BDP\Ejercicio_01\mapper.py
//
// (The same fix lives in internal/setup/hadoop_xml.go as uriPath;
// duplicated here to keep the templates self-contained and avoid an
// import cycle with the setup package.)
func toFileURI(absPath string) string {
	p := strings.ReplaceAll(absPath, `\`, `/`)
	if strings.HasPrefix(p, "/") {
		return "file://" + p
	}
	return "file:///" + p
}
