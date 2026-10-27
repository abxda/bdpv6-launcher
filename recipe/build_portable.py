"""BDP v7 "Convergente" · arma el Portable desde la receta única (stack.toml).

Mismo procedimiento en los 3 SO (se ejecuta nativo en cada uno: local o en el runner de CI):

    python recipe/build_portable.py --platform win-64   --out dist/portable [--launcher build/bin/bdpv6-launcher.exe] [--package]
    python recipe/build_portable.py --platform linux-64 --out dist/portable [--launcher build/bin/bdpv6-launcher]  [--package]
    python recipe/build_portable.py --platform osx-arm64 --out dist/portable [--launcher "build/bin/BDPV6 Launcher.app"] [--package]

Requiere Python >= 3.11 (tomllib). Cada descarga se verifica (sha512 / sha256 / sha1) antes de usarse
y se guarda en --cache para no repetirla. El layout resultante es el que espera el launcher:
common_jdk/ hadoop/ kafka_kraft/ elasticsearch/ python/ jupyter-config/ notebooks/ (+ bash/ en Windows).
"""
import argparse
import hashlib
import json
import os
import platform
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import time
import tomllib
import urllib.request
import zipfile
from pathlib import Path

try:  # red corporativa con inspección TLS: confiar en el almacén de certificados del SO
    import truststore
except ImportError:
    try:
        from pip._vendor import truststore
    except ImportError:
        truststore = None
if truststore:
    truststore.inject_into_ssl()

HERE = Path(__file__).resolve().parent
UA = {"User-Agent": "bdp-v7-build"}
HOST = {("Windows", "AMD64"): "win-64", ("Linux", "x86_64"): "linux-64",
        ("Darwin", "arm64"): "osx-arm64"}.get((platform.system(), platform.machine()))
ADOPTIUM = {"win-64": ("windows", "x64"), "linux-64": ("linux", "x64"), "osx-arm64": ("mac", "aarch64")}
BUILD_MARK = ".bdp-build-en-curso"


def log(msg):
    print(f"[{time.strftime('%H:%M:%S')}] {msg}", flush=True)


def http_get(url, timeout=120):
    with urllib.request.urlopen(urllib.request.Request(url, headers=UA), timeout=timeout) as r:
        return r.read()


def parse_sha(text, n):
    """Extrae un hash hexadecimal de n caracteres de los formatos usuales: 'HEX  archivo',
    'SHA512 (archivo) = HEX' y el de Apache Kafka 'archivo: XXXX XXXX ...' (en grupos)."""
    m = re.search(r"(?<![0-9a-fA-F])[0-9a-fA-F]{%d}(?![0-9a-fA-F])" % n, text)
    if m:
        return m.group(0).lower()
    compact = re.sub(r"\s+", "", text.split(":", 1)[-1])
    m = re.match(r"[0-9a-fA-F]{%d}" % n, compact)
    if not m:
        raise ValueError(f"no encontré un hash de {n} caracteres en: {text[:120]!r}")
    return m.group(0).lower()


def hash_file(path, algo):
    h = hashlib.new(algo)
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def fetch(urls, dest: Path, algo, expected):
    """Descarga (con reintentos entre espejos) y verifica el hash. Reutiliza la caché si ya coincide."""
    if isinstance(urls, str):
        urls = [urls]
    expected = expected.lower()
    if dest.exists() and hash_file(dest, algo) == expected:
        log(f"  caché ok: {dest.name}")
        return dest
    dest.parent.mkdir(parents=True, exist_ok=True)
    last = None
    for url in urls:
        try:
            log(f"  descargando {url}")
            part = dest.with_name(dest.name + ".part")
            with urllib.request.urlopen(urllib.request.Request(url, headers=UA), timeout=300) as r, open(part, "wb") as f:
                total = int(r.headers.get("Content-Length") or 0)
                done, mark = 0, 0
                for chunk in iter(lambda: r.read(1 << 20), b""):
                    f.write(chunk)
                    done += len(chunk)
                    if total and done * 10 // total > mark:
                        mark = done * 10 // total
                        log(f"    {mark * 10}% de {total / 2**20:,.0f} MB")
            part.replace(dest)
            break
        except Exception as e:  # noqa: BLE001 - se intenta el siguiente espejo
            last = e
            log(f"  fallo ({e}); probando el siguiente espejo")
    else:
        raise RuntimeError(f"no pude descargar {dest.name}: {last}")
    actual = hash_file(dest, algo)
    if actual != expected:
        dest.unlink()
        raise RuntimeError(f"HASH NO COINCIDE en {dest.name}: {actual} != {expected}")
    log(f"  verificado {algo}: {dest.name} ({dest.stat().st_size / 2**20:,.1f} MB)")
    return dest


def extract(archive: Path, into: Path, skip=None):
    """Extrae y devuelve la carpeta superior única del archivo (o 'into' si no hay una sola).
    skip(nombre) -> True omite ese miembro (p.ej. la documentación de Hadoop: sus rutas pasan el límite
    de 260 caracteres de Windows y de todos modos se borraría)."""
    if into.exists():
        shutil.rmtree(into)
    into.mkdir(parents=True)
    name = archive.name.lower()
    if name.endswith(".zip"):
        with zipfile.ZipFile(archive) as z:
            z.extractall(into, members=[m for m in z.namelist() if not (skip and skip(m))])
    elif name.endswith((".tar.gz", ".tgz")):
        with tarfile.open(archive) as t:
            members = [m for m in t.getmembers() if not (skip and skip(m.name))]
            if hasattr(tarfile, "data_filter"):
                t.extractall(into, members=members, filter="data")
            else:
                t.extractall(into, members=members)
    elif name.endswith(".7z.exe"):  # PortableGit: autoextraíble 7-Zip
        subprocess.run([str(archive), "-y", f"-o{into}"], check=True)
        return into
    else:
        raise ValueError(f"formato no soportado: {archive.name}")
    entries = [p for p in into.iterdir()]
    return entries[0] if len(entries) == 1 and entries[0].is_dir() else into


_HADOOP_SKIP = re.compile(r"/share/doc/|/jdiff/|/sources/|-tests\.jar$|/share/hadoop/tools/lib/(?!hadoop-streaming-)")


def hadoop_skip(member):
    """Lo que el lab no usa de Hadoop (se omite al extraer; el adelgazado posterior quita el resto)."""
    return bool(_HADOOP_SKIP.search(member))


def rm(*paths):
    for p in paths:
        if p.is_dir() and not p.is_symlink():
            shutil.rmtree(p, ignore_errors=True)
        elif p.exists() or p.is_symlink():
            p.unlink()


def dir_size_mb(p: Path):
    return sum(f.stat().st_size for f in p.rglob("*") if f.is_file() and not f.is_symlink()) / 2**20


# ---------------------------------------------------------------------------------------------------
class Build:
    def __init__(self, plat, out: Path, cache: Path, launcher):
        self.plat, self.out, self.cache, self.launcher = plat, out, cache, launcher
        self.win = plat == "win-64"
        self.S = tomllib.loads((HERE / "stack.toml").read_text(encoding="utf-8"))
        self.dl = cache / "downloads"
        self.work = cache / f"work-{plat}"
        self.versions = {}

    def move_into(self, src: Path, name: str):
        dest = self.out / name
        rm(dest)
        shutil.move(str(src), str(dest))
        return dest

    # --- piezas --------------------------------------------------------------------------------
    def python(self):
        P = self.S["python"]
        triple = P["triple"][self.plat]
        fname = f"cpython-{P['version']}+{P['release']}-{triple}-install_only.tar.gz"
        base = f"https://github.com/astral-sh/python-build-standalone/releases/download/{P['release']}"
        sums = http_get(f"{base}/SHA256SUMS").decode()
        line = next(ln for ln in sums.splitlines() if ln.strip().endswith(fname))
        arc = fetch(f"{base}/{fname.replace('+', '%2B')}", self.dl / fname, "sha256", parse_sha(line, 64))
        top = extract(arc, self.work / "python")
        self.move_into(top, "python")
        self.versions["python"] = P["version"]

    def python_bin(self):
        return self.out / "python" / ("python.exe" if self.win else "bin/python3")

    def site_packages(self):
        if self.win:
            return self.out / "python" / "Lib" / "site-packages"
        return next((self.out / "python" / "lib").glob("python3*/site-packages"))

    def uv(self):
        U = self.S["uv"]
        asset = U["asset"][self.plat]
        url = f"https://github.com/astral-sh/uv/releases/download/{U['version']}/{asset}"
        sha = parse_sha(http_get(url + ".sha256").decode(), 64)
        arc = fetch(url, self.dl / f"uv-{U['version']}-{asset}", "sha256", sha)
        top = extract(arc, self.cache / f"tools-uv-{self.plat}")
        exe = next(top.rglob("uv.exe" if self.win else "uv"))
        if not self.win:
            exe.chmod(exe.stat().st_mode | stat.S_IEXEC)
        return exe

    def pip_install(self):
        uv = self.uv()
        env = dict(os.environ, UV_LINK_MODE="copy", UV_CACHE_DIR=str(self.cache / "uv-cache"),
                   UV_PYTHON_DOWNLOADS="never")
        log("  uv pip install --require-hashes (lock universal)")
        subprocess.run([str(uv), "pip", "install", "--python", str(self.python_bin()), "--require-hashes",
                        "--system-certs", "-r", str(HERE / self.S["python"]["lock"])], check=True, env=env)
        if not self.win:
            self.unix_python_fixups()
        import_check = "import pyspark, sedona, geopandas, elasticsearch, hdfs, jupyterlab; print(pyspark.__version__)"
        v = subprocess.run([str(self.python_bin()), "-c", import_check], check=True, capture_output=True, text=True)
        self.versions["pyspark"] = v.stdout.strip()

    def unix_python_fixups(self):
        """'python' por nombre y scripts de bin/ reubicables (uv escribe la ruta absoluta del build)."""
        b = self.out / "python" / "bin"
        if not (b / "python").exists():
            (b / "python").symlink_to("python3")
        root = str(self.out / "python")
        for f in b.iterdir():
            if f.is_symlink() or not f.is_file():
                continue
            with open(f, "rb") as fh:
                head = fh.read(256)
            if head.startswith(b"#!") and root.encode() in head.split(b"\n", 1)[0]:
                rest = f.read_bytes().split(b"\n", 1)[1]
                f.write_bytes(b"#!/usr/bin/env python3\n" + rest)

    def jars(self):
        dest = self.site_packages() / "pyspark" / "jars"
        for j in self.S["jars"]:
            g, a, v = j["coord"].split(":")
            fname = f"{a}-{v}.jar"
            url = f"https://repo1.maven.org/maven2/{g.replace('.', '/')}/{a}/{v}/{fname}"
            shutil.copy2(fetch(url, self.dl / "maven" / fname, "sha1", j["sha1"]), dest / fname)
        self.versions["jars"] = [j["coord"] for j in self.S["jars"]]

    def jdk(self):
        J = self.S["jdk"]
        os_, arch = ADOPTIUM[self.plat]
        api = (f"https://api.adoptium.net/v3/assets/release_name/eclipse/{J['release'].replace('+', '%2B')}"
               f"?architecture={arch}&heap_size=normal&image_type=jdk&jvm_impl=hotspot&os={os_}&project=jdk&vendor=eclipse")
        pkg = json.loads(http_get(api))["binaries"][0]["package"]
        arc = fetch(pkg["link"], self.dl / pkg["name"], "sha256", pkg["checksum"])
        top = extract(arc, self.work / "jdk")
        home = top / "Contents" / "Home" if (top / "Contents" / "Home").is_dir() else top  # macOS: bundle
        jdk = self.move_into(home, "common_jdk")
        for s in J["slim"]:
            rm(jdk / s)
        self.versions["jdk"] = J["release"]

    def hadoop(self):
        H = self.S["hadoop"]
        arc = fetch(H["urls"], self.dl / f"hadoop-{H['version']}.tar.gz", "sha512", H["sha512"])
        log("  extrayendo Hadoop (sin documentación, fuentes ni SDKs de nube)")
        top = extract(arc, self.work / "hadoop", skip=hadoop_skip)
        h = self.move_into(top, "hadoop")
        before = dir_size_mb(h)
        sh = h / "share" / "hadoop"
        # Solo HDFS + MapReduce local: fuera SDKs de nube, nativos de otra plataforma, docs, fuentes y tests.
        for f in (sh / "tools" / "lib").iterdir():
            if not f.name.startswith("hadoop-streaming-"):
                rm(f)
        if self.win or self.plat == "osx-arm64":
            rm(h / "lib" / "native")              # .so/.a de Linux: inútiles aquí (solo un WARN inocuo)
        for d in list(sh.rglob("*")):
            if d.is_dir() and d.name in ("jdiff", "sources"):
                rm(d)
        for f in sh.rglob("*-tests.jar"):
            rm(f)
        rm(h / "share" / "doc", sh / "client")
        for d in ("csi", "timelineservice", "webapps"):
            rm(sh / "yarn" / d)
        for f in (sh / "yarn").glob("*.war"):
            rm(f)
        if self.win:
            W = H["winutils"]
            for name, sha in W["sha256"].items():
                shutil.copy2(fetch(f"{W['url_base']}/{name}", self.dl / "winutils" / name, "sha256", sha), h / "bin" / name)
            # LF obligatorio: lo lee bash (con CRLF fallaría con "$'\r': command not found").
            shim = (HERE / "templates" / "hadoop-bash-shim.sh").read_text(encoding="utf-8").replace("\r\n", "\n")
            for name in ("hadoop", "hdfs", "mapred"):
                (h / "bin" / name).write_bytes(shim.replace("__NAME__", name).encode())  # LF: lo lee bash
            for f in (h / "bin").glob("*.pdb"):
                rm(f)
        log(f"  Hadoop adelgazado: {before:,.0f} MB -> {dir_size_mb(h):,.0f} MB")
        self.versions["hadoop"] = H["version"]

    def kafka(self):
        K = self.S["kafka"]
        fname = f"kafka_{K['scala']}-{K['version']}.tgz"
        urls = [f"https://downloads.apache.org/kafka/{K['version']}/{fname}",
                f"https://archive.apache.org/dist/kafka/{K['version']}/{fname}"]
        top = extract(fetch(urls, self.dl / fname, "sha512", K["sha512"]), self.work / "kafka")
        k = self.move_into(top, "kafka_kraft")
        rm(k / "site-docs")
        (k / "config" / "kraft").mkdir(parents=True, exist_ok=True)
        shutil.copy2(HERE / "templates" / "kafka-server.properties", k / "config" / "kraft" / "server.properties")
        self.versions["kafka"] = K["version"]

    def elasticsearch(self):
        E = self.S["elasticsearch"]
        fname = f"elasticsearch-{E['version']}-{E['asset'][self.plat]}"
        url = f"https://artifacts.elastic.co/downloads/elasticsearch/{fname}"
        sha = E.get("sha512", {}).get(self.plat) or parse_sha(http_get(url + ".sha512").decode(), 128)
        log("  extrayendo Elasticsearch")
        top = extract(fetch(url, self.dl / fname, "sha512", sha), self.work / "es")
        es = self.move_into(top, "elasticsearch")
        rm(es / "jdk", es / "jdk.app")            # usa ES_JAVA_HOME = JDK común
        shutil.copy2(HERE / "templates" / "elasticsearch.yml", es / "config" / "elasticsearch.yml")
        self.versions["elasticsearch"] = E["version"]

    def gitbash(self):
        G = self.S["gitbash"]
        arc = fetch(G["url"], self.dl / G["url"].rsplit("/", 1)[-1], "sha256", G["sha256"])
        log("  extrayendo PortableGit (bash del lab)")
        top = extract(arc, self.work / "gitbash")
        self.move_into(top, "bash")
        self.versions["gitbash"] = G["url"].rsplit("/", 1)[-1]

    def lab_files(self):
        jc = self.out / "jupyter-config"
        jc.mkdir(exist_ok=True)
        shutil.copy2(HERE / "templates" / "jupyter_server_config.py", jc / "jupyter_server_config.py")
        for d in ("notebooks", "data", "logs"):
            (self.out / d).mkdir(exist_ok=True)
        (self.out / "bdp-stack.json").write_text(json.dumps({
            "bdp": self.S["meta"]["version"], "platform": self.plat,
            "built_at": time.strftime("%Y-%m-%dT%H:%M:%S"), "components": self.versions}, indent=2), encoding="utf-8")

    def copy_launcher(self):
        if not self.launcher:
            log("  (sin --launcher: el portable queda sin ejecutable; útil solo para pruebas)")
            return
        src = Path(self.launcher)
        if src.is_dir():  # macOS .app
            shutil.copytree(src, self.out / src.name, symlinks=True, dirs_exist_ok=True)
        else:
            shutil.copy2(src, self.out / src.name)

    # --- orquestación ------------------------------------------------------------------------------
    def run(self):
        t0 = time.time()
        self.out.mkdir(parents=True, exist_ok=True)
        (self.out / BUILD_MARK).write_text("build en curso\n", encoding="utf-8")
        steps = [("Python portable", self.python), ("Dependencias de Python (lock universal)", self.pip_install),
                 ("Jars pre-cargados (Sedona, Kafka)", self.jars), ("JDK", self.jdk), ("Hadoop", self.hadoop),
                 ("Kafka (perfil UNIR)", self.kafka), ("Elasticsearch (perfil UP)", self.elasticsearch)]
        if self.win:
            steps.append(("Git Bash (terminal homologada)", self.gitbash))
        steps += [("Archivos del lab", self.lab_files), ("Launcher", self.copy_launcher)]
        for i, (name, fn) in enumerate(steps, 1):
            log(f"[{i}/{len(steps)}] {name}")
            fn()
        rm(self.work, self.out / BUILD_MARK)
        log(f"Portable listo en {self.out} ({dir_size_mb(self.out):,.0f} MB) en {time.time() - t0:,.0f} s")


def package(out: Path, plat: str):
    name = {"win-64": "windows-amd64", "linux-64": "linux-amd64", "osx-arm64": "macos-arm64"}[plat]
    tgz = out.parent / f"bdp-v7-portable-{name}.tar.gz"
    log(f"Empaquetando {tgz.name} (contenido en ./, como v6)")
    with tarfile.open(tgz, "w:gz", compresslevel=6) as t:
        for p in sorted(out.iterdir()):
            t.add(p, arcname=f"./{p.name}")
    sha = hash_file(tgz, "sha256")
    tgz.with_name(tgz.name + ".sha256").write_text(f"{sha}  {tgz.name}\n", encoding="utf-8")
    log(f"{tgz.name}: {tgz.stat().st_size / 2**20:,.0f} MB · sha256 {sha}")
    return tgz


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--platform", default=HOST, choices=["win-64", "linux-64", "osx-arm64"])
    ap.add_argument("--out", required=True, type=Path)
    ap.add_argument("--cache", type=Path, default=HERE.parent / ".cache" / "v7")
    ap.add_argument("--launcher", help="ejecutable del launcher compilado con wails (o el .app en macOS)")
    ap.add_argument("--package", action="store_true", help="genera el .tar.gz + .sha256 junto a --out")
    ap.add_argument("--clean", action="store_true",
                    help="borra --out si lo creó un build anterior (tiene bdp-stack.json o el marcador de build)")
    a = ap.parse_args()
    if a.platform != HOST:
        sys.exit(f"build nativo únicamente: este equipo es {HOST}, se pidió {a.platform}")
    out = a.out.resolve()
    if out.exists() and any(out.iterdir()):
        made_by_us = (out / "bdp-stack.json").exists() or (out / BUILD_MARK).exists()
        if not (a.clean and made_by_us):
            sys.exit(f"--out debe estar vacío o no existir: {out}"
                     + (" (usa --clean para rehacer un build anterior)" if made_by_us else ""))
        log(f"--clean: borrando el build anterior en {out}")
        shutil.rmtree(out)
    Build(a.platform, out, a.cache.resolve(), a.launcher).run()
    if a.package:
        package(out, a.platform)


if __name__ == "__main__":
    main()
