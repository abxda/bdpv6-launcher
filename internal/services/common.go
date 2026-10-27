package services

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// javaEnv returns the minimum environment a Hadoop / Kafka / Elasticsearch
// process needs to find the bundled JDK and Hadoop config.
func javaEnv(p *paths.Paths) map[string]string {
	env := map[string]string{
		"JAVA_HOME":       p.CommonJDK,
		"HADOOP_HOME":     p.Hadoop,
		"HADOOP_CONF_DIR": p.HadoopConf,
	}
	if p.IsV7() {
		env["HADOOP_OPTS"] = paths.HadoopAddOpens
		// Techo de heap para NameNode/DataNode (y clientes hdfs). Los .cmd de
		// Windows leen HADOOP_HEAPSIZE en MB (si no, -Xmx1000m); los scripts
		// Unix leen HADOOP_HEAPSIZE_MAX.
		env["HADOOP_HEAPSIZE"] = "512"
		env["HADOOP_HEAPSIZE_MAX"] = "512m"
	}
	return env
}

// fileURI convierte una ruta absoluta en file:///D:/... (Windows) o
// file:///home/... (Unix), la forma que esperan log4j2 y Hadoop.
func fileURI(abs string) string {
	s := filepath.ToSlash(abs)
	if strings.HasPrefix(s, "/") {
		return "file://" + s
	}
	return "file:///" + s
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// pythonBinDirs son las carpetas del Python portable que van al PATH para que
// `python` (y `python -m pip/streamlit/...`) resuelvan al del lab.
func pythonBinDirs(p *paths.Paths) []string {
	if runtime.GOOS == "windows" {
		return []string{p.Python}
	}
	return []string{filepath.Join(p.Python, "bin")}
}

// jupyterEnv extends javaEnv with PySpark hooks so notebooks that import
// pyspark.sql can find the embedded Python and the bundled Spark. Además
// antepone al PATH los bin del stack portable (hadoop, spark, jdk) para que las
// TERMINALES que JupyterLab abre (que heredan este entorno) reconozcan los
// comandos `hdfs`, `hadoop`, `spark-submit` y `java` por nombre — sin esto, el
// binario existe en hadoop/bin pero no está en el PATH del terminal.
func jupyterEnv(p *paths.Paths) map[string]string {
	env := javaEnv(p)
	env["SPARK_HOME"] = p.Spark
	env["PYSPARK_PYTHON"] = p.PythonBinary()
	env["PYSPARK_DRIVER_PYTHON"] = p.PythonBinary()
	sep := string(os.PathListSeparator)
	dirs := []string{
		filepath.Join(p.Hadoop, "bin"),
		filepath.Join(p.Spark, "bin"),
		filepath.Join(p.CommonJDK, "bin"),
	}
	if p.IsV7() {
		// v7: la terminal homologada (bash en los 3 SO) reconoce por nombre
		// python, hdfs, kafka-topics.sh, spark-submit y java, sin exports.
		dirs = append(pythonBinDirs(p),
			filepath.Join(p.Hadoop, "bin"),
			filepath.Join(p.Spark, "bin"),
			filepath.Join(p.Kafka, "bin"),
			filepath.Join(p.CommonJDK, "bin"))
		// Los kafka-*.sh pasan a java una ruta /d/... para log4j2 que en
		// Windows no existe (llenan la terminal de errores); se fija aquí.
		env["KAFKA_LOG4J_OPTS"] = p.KafkaLog4jOpts()
		// Jupyter aislado del ~/.jupyter del alumno: config sin token y con
		// terminal bash (jupyter-config/jupyter_server_config.py).
		env["JUPYTER_CONFIG_DIR"] = p.JupyterConfig
		env["JUPYTER_DATA_DIR"] = filepath.Join(p.Data, "jupyter", "data")
		env["JUPYTER_RUNTIME_DIR"] = filepath.Join(p.Data, "jupyter", "runtime")
		env["BDP_NOTEBOOKS"] = p.Notebooks
		env["CHERE_INVOKING"] = "1" // Git Bash: no saltar a $HOME al abrir
		if runtime.GOOS == "windows" {
			env["BDP_BASH"] = p.Bash
		}
	}
	env["PATH"] = strings.Join(dirs, sep) + sep + os.Getenv("PATH")
	return env
}

// sinkWriter adapts a logsink.Sink to io.Writer for processctl.Spec.Out. It
// splits on newline so the Sink stores one Line per terminal line, and
// strips trailing '\r' so Windows .bat output is not double-spaced.
//
// onLine, if non-nil, is invoked once per complete line — used by Jupyter
// to scrape the access URL on the way through.
type sinkWriter struct {
	s      *logsink.Sink
	stream string // "stdout" or "stderr" (best-effort; we usually merge both)
	onLine func(text string)
}

func (w sinkWriter) Write(p []byte) (int, error) {
	start := 0
	for i, b := range p {
		if b == '\n' {
			line := trimCR(string(p[start:i]))
			w.s.Emit("INFO", line)
			if w.onLine != nil {
				w.onLine(line)
			}
			start = i + 1
		}
	}
	if start < len(p) {
		line := trimCR(string(p[start:]))
		w.s.Emit("INFO", line)
		if w.onLine != nil {
			w.onLine(line)
		}
	}
	return len(p), nil
}

func trimCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}

// mergeOSEnvFor returns os.Environ() with the given overrides applied. Used
// for one-shot helper commands (like dfsadmin) that don't go through
// processctl.Spec.Env. Later occurrences win.
func mergeOSEnvFor(maps ...map[string]string) []string {
	merged := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	out := []string{}
	used := map[string]bool{}
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if v, ok := merged[k]; ok {
			out = append(out, k+"="+v)
			used[k] = true
		} else {
			out = append(out, kv)
		}
	}
	for k, v := range merged {
		if !used[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}
