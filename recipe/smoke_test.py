"""BDP v7 · prueba de humo SIN interfaz sobre un portable recién construido (local o en CI).

    python recipe/smoke_test.py --dist dist/portable --profile up     # HDFS + Spark/Sedona/MLlib + Elasticsearch
    python recipe/smoke_test.py --dist dist/portable --profile unir   # HDFS + Spark + Kafka (streaming)

Reproduce el entorno que el launcher da a los servicios (internal/services/common.go) y la configuración
HDFS que escribe su asistente (internal/setup/hadoop_xml.go). Sale con código != 0 si algo falla, y
siempre apaga lo que encendió.
"""
import argparse
import json
import os
import signal
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

WIN = os.name == "nt"
ADD_OPENS = ("--add-opens=java.base/java.lang=ALL-UNNAMED --add-opens=java.base/java.util=ALL-UNNAMED "
             "--add-opens=java.base/java.nio=ALL-UNNAMED")
RESULTS, PROCS = [], []


def log(msg):
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def check(name, ok, detail=""):
    RESULTS.append((name, bool(ok), detail))
    log(f"{'OK   ' if ok else 'FALLA'} {name} {detail}")
    return ok


def port_open(port):
    with socket.socket() as s:
        s.settimeout(0.5)
        return s.connect_ex(("127.0.0.1", port)) == 0


def wait_port(port, secs):
    for _ in range(secs):
        if port_open(port):
            return True
        time.sleep(1)
    return False


def file_uri(p: Path):
    s = p.as_posix()
    return "file://" + s if s.startswith("/") else "file:///" + s


class Lab:
    def __init__(self, dist: Path):
        self.d = dist
        self.py = dist / "python" / ("python.exe" if WIN else "bin/python3")
        site = (dist / "python" / "Lib" / "site-packages") if WIN else next((dist / "python" / "lib").glob("python3*/site-packages"))
        self.spark_home = site / "pyspark"
        self.env = dict(os.environ)
        self.env.update({
            "JAVA_HOME": str(dist / "common_jdk"), "HADOOP_HOME": str(dist / "hadoop"),
            "HADOOP_CONF_DIR": str(dist / "hadoop" / "etc" / "hadoop"), "HADOOP_OPTS": ADD_OPENS,
            "HADOOP_HEAPSIZE": "512", "HADOOP_HEAPSIZE_MAX": "512m", "HADOOP_IDENT_STRING": "bdp",
            "SPARK_HOME": str(self.spark_home), "PYSPARK_PYTHON": str(self.py), "PYSPARK_DRIVER_PYTHON": str(self.py),
            "KAFKA_LOG4J_OPTS": "-Dlog4j2.configurationFile=" + file_uri(dist / "kafka_kraft" / "config" / "tools-log4j2.yaml"),
        })
        bins = [self.py.parent, dist / "hadoop" / "bin", self.spark_home / "bin", dist / "kafka_kraft" / "bin",
                dist / "common_jdk" / "bin"]
        self.env["PATH"] = os.pathsep.join(map(str, bins)) + os.pathsep + self.env.get("PATH", "")
        self.hdfs = str(dist / "hadoop" / "bin" / ("hdfs.cmd" if WIN else "hdfs"))

    def start(self, args, cwd, log_name, env=None):
        logf = open(self.d / "logs" / f"smoke_{log_name}.log", "w", encoding="utf-8", errors="replace")
        kw = {"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if WIN else {"start_new_session": True}
        p = subprocess.Popen(args, cwd=cwd, env=env or self.env, stdout=logf, stderr=subprocess.STDOUT, **kw)
        PROCS.append(p)
        return p

    def write_hadoop_xml(self):
        uri = file_uri(self.d)
        conf = self.d / "hadoop" / "etc" / "hadoop"
        (conf / "core-site.xml").write_text(f"""<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property><name>fs.defaultFS</name><value>hdfs://localhost:9000</value></property>
  <property><name>hadoop.tmp.dir</name><value>{uri}/data/hadoop_tmp</value></property>
</configuration>
""", encoding="utf-8")
        props = {
            "dfs.replication": "1", "dfs.namenode.name.dir": f"{uri}/data/hdfs/namenode",
            "dfs.datanode.data.dir": f"{uri}/data/hdfs/datanode", "dfs.permissions.enabled": "false",
            "dfs.namenode.rpc-bind-host": "127.0.0.1", "dfs.namenode.http-address": "127.0.0.1:9870",
            "dfs.datanode.address": "127.0.0.1:9866", "dfs.datanode.http.address": "127.0.0.1:9864",
            "dfs.datanode.ipc.address": "127.0.0.1:9867", "dfs.datanode.hostname": "localhost",
            "dfs.client.use.datanode.hostname": "true", "dfs.datanode.use.datanode.hostname": "true",
            "dfs.webhdfs.user.provider.user.pattern": "^[A-Za-z0-9_][A-Za-z0-9._-]*[$]?$",
        }
        body = "\n".join(f"  <property><name>{k}</name><value>{v}</value></property>" for k, v in props.items())
        (conf / "hdfs-site.xml").write_text(f'<?xml version="1.0" encoding="UTF-8"?>\n<configuration>\n{body}\n</configuration>\n', encoding="utf-8")

    def py_run(self, code, timeout=900):
        r = subprocess.run([str(self.py), "-c", code], env=self.env, capture_output=True, text=True, timeout=timeout)
        out = (r.stdout or "").strip().splitlines()
        return r.returncode == 0, (out[-1] if out else (r.stderr or "")[-400:])


def stop_all():
    for p in reversed(PROCS):
        if p.poll() is None:
            if WIN:
                subprocess.run(["taskkill", "/T", "/F", "/PID", str(p.pid)], capture_output=True)
            else:
                try:
                    os.killpg(p.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
    for p in PROCS:
        try:
            p.wait(timeout=30)
        except subprocess.TimeoutExpired:
            if not WIN:
                os.killpg(p.pid, signal.SIGKILL)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dist", required=True, type=Path)
    ap.add_argument("--profile", choices=["up", "unir", "all"], default="all")
    a = ap.parse_args()
    lab = Lab(a.dist.resolve())
    (lab.d / "logs").mkdir(exist_ok=True)
    up, unir = a.profile in ("up", "all"), a.profile in ("unir", "all")
    try:
        # --- HDFS (base) -------------------------------------------------------------------------
        lab.write_hadoop_xml()
        if not (lab.d / "data" / "hdfs" / "namenode" / "current" / "VERSION").exists():
            r = subprocess.run([lab.hdfs, "namenode", "-format", "-nonInteractive", "-force"], env=lab.env,
                               capture_output=True, text=True, cwd=lab.d / "hadoop")
            check("hdfs namenode -format", (lab.d / "data" / "hdfs" / "namenode" / "current" / "VERSION").exists(), r.stderr[-200:] if r.returncode else "")
        lab.start([lab.hdfs, "namenode"], lab.d / "hadoop", "namenode")
        check("NameNode escucha en 9870", wait_port(9870, 120))
        lab.start([lab.hdfs, "datanode"], lab.d / "hadoop", "datanode")
        check("DataNode escucha en 9864", wait_port(9864, 120))
        time.sleep(8)
        subprocess.run([lab.hdfs, "dfs", "-mkdir", "-p", "/smoke"], env=lab.env, capture_output=True)
        sample = lab.d / "data" / "smoke.csv"
        sample.write_text("id,ciudad,valor\n" + "\n".join(f"{i},AGS,{i * 1.5}" for i in range(500)), encoding="utf-8")
        r = subprocess.run([lab.hdfs, "dfs", "-put", "-f", str(sample), "/smoke/"], env=lab.env, capture_output=True, text=True)
        check("hdfs dfs -put", r.returncode == 0, r.stderr[-200:] if r.returncode else "")
        req = urllib.request.Request("http://localhost:9870/webhdfs/v1/smoke/web.txt?op=CREATE&user.name=17064&overwrite=true", method="PUT")

        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, *args, **kw):
                return None
        try:
            urllib.request.build_opener(NoRedirect).open(req, timeout=30)
            loc = ""
        except urllib.error.HTTPError as e:
            loc = e.headers.get("Location", "")
        check("WebHDFS (usuario numérico) redirige a localhost", loc.startswith("http://localhost:9864/"), loc[:80])

        # --- Spark (base + perfil) ------------------------------------------------------------------
        out_local = file_uri(lab.d / "data" / "smoke_out")
        code = f"""
from pyspark.sql import SparkSession, functions as F
s = (SparkSession.builder.master('local[2]').config('spark.driver.memory','1g')
     .config('spark.sql.extensions','org.apache.sedona.sql.SedonaSqlExtensions').getOrCreate())
df = s.read.csv('hdfs://localhost:9000/smoke/smoke.csv', header=True)
df.write.mode('overwrite').parquet('{out_local}/p')
n = s.read.parquet('{out_local}/p').count()
extra = ''
if {up}:
    from sedona.spark import SedonaContext
    sd = SedonaContext.create(s)
    k = sd.sql("SELECT ST_Intersects(ST_Point(-102.29,21.88), ST_Buffer(ST_Point(-102.29,21.88),0.01)) AS ok").first().ok
    from pyspark.ml.clustering import KMeans
    from pyspark.ml.feature import VectorAssembler
    v = VectorAssembler(inputCols=['x'], outputCol='features').transform(s.range(200).withColumn('x', F.rand(1)))
    KMeans(k=3, seed=1).fit(v)
    extra = f' sedona={{k}} mllib=ok'
import pyspark
print(f'pyspark={{pyspark.__version__}} filas={{n}}{{extra}}')
s.stop()
"""
        ok, detail = lab.py_run(code)
        check("Spark: HDFS -> parquet local" + (" + Sedona + MLlib" if up else ""), ok and "filas=500" in detail, detail)

        # --- Elasticsearch (perfil UP) ------------------------------------------------------------
        if up:
            es_env = dict(lab.env, ES_JAVA_HOME=str(lab.d / "common_jdk"), ES_JAVA_OPTS="-Xms512m -Xmx512m")
            es_bin = lab.d / "elasticsearch" / "bin" / ("elasticsearch.bat" if WIN else "elasticsearch")
            lab.start([str(es_bin)], lab.d / "elasticsearch", "elasticsearch", env=es_env)
            healthy = False
            for _ in range(180):
                try:
                    h = json.loads(urllib.request.urlopen("http://127.0.0.1:9200/_cluster/health", timeout=2).read())
                    healthy = h["status"] in ("green", "yellow")
                    if healthy:
                        break
                except Exception:  # noqa: BLE001 - aún arrancando
                    time.sleep(1)
            check("Elasticsearch responde (JDK común, sin ML)", healthy)
            code = """
from elasticsearch import Elasticsearch, helpers
es = Elasticsearch('http://localhost:9200')
es.indices.create(index='smoke', mappings={'properties': {'location': {'type': 'geo_point'}}})
helpers.bulk(es, ({'_index': 'smoke', '_source': {'n': i, 'location': {'lat': 21.88, 'lon': -102.29}}} for i in range(100)))
es.indices.refresh(index='smoke')
print('docs', es.count(index='smoke')['count'], 'ml', es.xpack.info()['features']['ml']['enabled'])
"""
            ok, detail = lab.py_run(code, timeout=300)
            check("ES: helpers.bulk 100 docs con geo_point", ok and "docs 100" in detail, detail)
            data_inside = (lab.d / "data" / "es_data").exists()
            check("ES guarda su índice DENTRO del portable (data/es_data)", data_inside)

        # --- Kafka (perfil UNIR) ------------------------------------------------------------------
        if unir:
            K = lab.d / "kafka_kraft"
            cfg = K / "config" / "kraft" / "server.properties"
            cp = str(K / "libs" / "*")
            java = str(lab.d / "common_jdk" / "bin" / ("java.exe" if WIN else "java"))
            log4j = "-Dlog4j2.configurationFile=" + file_uri(K / "config" / "log4j2.yaml")
            if not (K / "data" / "data_kraft" / "meta.properties").exists():
                uuid = subprocess.run([java, log4j, "-cp", cp, "kafka.tools.StorageTool", "random-uuid"], env=lab.env,
                                      capture_output=True, text=True, cwd=K).stdout.strip().splitlines()[-1]
                r = subprocess.run([java, log4j, "-cp", cp, "kafka.tools.StorageTool", "format", "-t", uuid, "-c", str(cfg)],
                                   env=lab.env, capture_output=True, text=True, cwd=K)
                check("kafka-storage format (quórum estático)", r.returncode == 0, (r.stdout + r.stderr)[-200:])
            lab.start([java, "-Xms256m", "-Xmx512m", log4j, "-cp", cp, "kafka.Kafka", str(cfg)], K, "kafka")
            check("Kafka escucha en 9092", wait_port(9092, 120))
            time.sleep(5)
            code = """
from pyspark.sql import SparkSession
s = SparkSession.builder.master('local[2]').config('spark.driver.memory','1g').getOrCreate()
s.createDataFrame([(f'm{i}',) for i in range(10)], 'value string').write.format('kafka') \\
    .option('kafka.bootstrap.servers','localhost:9092').option('topic','smoke').save()
n = s.read.format('kafka').option('kafka.bootstrap.servers','localhost:9092').option('subscribe','smoke') \\
    .option('startingOffsets','earliest').load().count()
print('mensajes', n)
s.stop()
"""
            ok, detail = lab.py_run(code)
            check("Kafka <-> Spark (conector pre-cargado)", ok and "mensajes 10" in detail, detail)
            r = subprocess.run([java, log4j, "-cp", cp, "org.apache.kafka.tools.TopicCommand", "--bootstrap-server",
                                "localhost:9092", "--delete", "--topic", "smoke"], env=lab.env, capture_output=True, text=True)
            check("Kafka rechaza --delete (seguro para Windows)", "disabled" in (r.stdout + r.stderr))
    finally:
        stop_all()
        time.sleep(3)
        busy = [p for p in (9000, 9092, 9093, 9200, 9864, 9866, 9870) if port_open(p)]
        check("apagado limpio: puertos libres", not busy, f"ocupados: {busy}" if busy else "")
    fails = [r for r in RESULTS if not r[1]]
    log(f"RESUMEN perfil={a.profile}: {len(RESULTS) - len(fails)}/{len(RESULTS)} OK")
    sys.exit(1 if fails else 0)


if __name__ == "__main__":
    main()
