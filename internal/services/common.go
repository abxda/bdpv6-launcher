package services

import (
	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// javaEnv returns the minimum environment a Hadoop / Kafka / Elasticsearch
// process needs to find the bundled JDK and Hadoop config.
func javaEnv(p *paths.Paths) map[string]string {
	return map[string]string{
		"JAVA_HOME":       p.CommonJDK,
		"HADOOP_HOME":     p.Hadoop,
		"HADOOP_CONF_DIR": p.HadoopConf,
	}
}

// jupyterEnv extends javaEnv with PySpark hooks so notebooks that import
// pyspark.sql can find the embedded Python and the bundled Spark.
func jupyterEnv(p *paths.Paths) map[string]string {
	env := javaEnv(p)
	env["SPARK_HOME"] = p.Spark
	env["PYSPARK_PYTHON"] = p.PythonBinary()
	env["PYSPARK_DRIVER_PYTHON"] = p.PythonBinary()
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
