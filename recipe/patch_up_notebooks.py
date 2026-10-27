"""BDP v7 · genera los cuadernos UP para el laboratorio v7 (Codespaces y portable) a partir de la
variante container/ de Curso_BDP (Spark 4 / Scala 2.13), con cambios mínimos:

- quita la descarga de Sedona 1.8 por Maven (v7 trae Sedona 1.9.1 pre-cargado para Spark 4.1);
- driver de 2 GB (el recorte de un estado cabe de sobra);
- comenta los `pip install` (todo viene instalado en el kernel "BDP v7");
- limpia salidas y apunta el cuaderno al kernel "BDP v7".
Las rutas HDFS NO cambian: `bdp_start.py datos` carga el recorte del estado en esas mismas rutas.

    python recipe/patch_up_notebooks.py <Curso_BDP> <carpeta_destino_notebooks>
"""
import json
import re
import shutil
import sys
from pathlib import Path

SRC, DEST = Path(sys.argv[1]), Path(sys.argv[2])
FILES = {
    "Semana_2/notebooks/container/TestGlobalBigData.ipynb": "semana_2/TestGlobalBigData.ipynb",
    "_lab_work/drone_sensors_data.csv": "semana_2/drone_sensors_data.csv",
    "Semana_3/notebooks/container/01_ProcesarDirecciones.ipynb": "semana_3/01_ProcesarDirecciones.ipynb",
    "Semana_3/notebooks/container/02_IndexacionGeoespacial.ipynb": "semana_3/02_IndexacionGeoespacial.ipynb",
    "Semana_3/notebooks/container/03_prompt_app_streamlit.txt": "semana_3/03_prompt_app_streamlit.txt",
    "Semana_4/notebooks/container/01_ClasificacionNoSupervisada_KMeans.ipynb": "semana_4/01_ClasificacionNoSupervisada_KMeans.ipynb",
    "Semana_4/notebooks/container/02_CalculoVariablesDenueOxxo.ipynb": "semana_4/02_CalculoVariablesDenueOxxo.ipynb",
    "Semana_4/notebooks/container/03_ClasificacionSupervisada_GBT.ipynb": "semana_4/03_ClasificacionSupervisada_GBT.ipynb",
    "Semana_4/notebooks/container/04_PrediccionPorCoordenada.ipynb": "semana_4/04_PrediccionPorCoordenada.ipynb",
    "Semana_4/notebooks/container/05_api.py": "semana_4/05_api.py",
}
NOTE = "# BDP v7: Sedona 1.9.1 ya viene pre-cargado en el laboratorio (sin descargas de Maven)"
PKG_MULTI = re.compile(r'\n([ \t]*)\.config\(\s*"spark\.jars\.packages",\s*(?:"[^"]*"\s*)+\)')
PKG_SINGLE = re.compile(r'\n([ \t]*)\.config\("spark\.jars\.packages",[^\n]*')
PKG_LIST = re.compile(r'^([ \t]*)"org\.(?:apache\.sedona|datasyslab):[^"\n]*",?[ \t]*$', re.M)
MEM = re.compile(r'\.config\("spark\.driver\.memory",\s*"\d+g"\)')
# client.download(hdfs, local, overwrite=True) sobre una carpeta local que ya existe anida la copia
# y Pandas lee todo dos veces (filas duplicadas al re-ejecutar). Se borra la copia local antes.
DOWNLOAD = re.compile(r'^([ \t]*)client\.download\((\w+),\s*(\w+),\s*overwrite=True\)', re.M)
PIP = re.compile(r'^([ \t]*)([!%]pip install [^\n]*)$', re.M)
KERNEL = {"name": "bdp-v7", "display_name": "BDP v7 (Spark 4.1 · Python 3.12)", "language": "python"}
MD_VARIANT = re.compile(r"^> ⚙️ \*\*Esta variante es para:[^\n]*$", re.M)
MD_CHECK = re.compile(r"^> Comprueba tu versión con [^\n]*$", re.M)
MD_DOWNLOAD = re.compile(r"^⬇️ \*\*Descarga \(Hugging Face\):\*\* [^\n]*$", re.M)
# Fichas y comentarios que aún describían el dato nacional o las variantes Portable/Container de v6.
MD_FIX = [
    ("Extraer coordenadas del padrón nacional (Sedona)", "Extraer coordenadas del padrón de domicilios (Sedona)"),
    ("Leer el GeoParquet nacional desde HDFS", "Leer el GeoParquet de domicilios desde HDFS"),
    ("`/data/raw/geocoder/integrado_ne.parquet` (1.62 GB, 33.6M domicilios INEGI).",
     "`/data/raw/geocoder/integrado_ne.parquet` (recorte de Aguascalientes: 467,926 domicilios INEGI)."),
    ("Dato nacional cargado en HDFS. Sedona se baja vía spark.jars.packages.",
     "Datos cargados en HDFS con `python3 bdp_start.py datos`. Sedona 1.9.1 ya viene pre-cargado."),
    ("`/data/raw/geodatos_mexico/censo_2020_nacional.parquet`.",
     "`/data/raw/geodatos_mexico/censo_2020_nacional.parquet` (recorte de Aguascalientes: 15,818 manzanas; "
     "el archivo conserva el nombre del dato nacional)."),
    ("`/data/raw/geodatos_mexico/{censo_2020_nacional,denue_nacional}.parquet`.",
     "`/data/raw/geodatos_mexico/{censo_2020_nacional,denue_nacional}.parquet` (recorte de Aguascalientes)."),
    ("Censo y DENUE en HDFS. PESADO: sube spark.driver.memory (8g) o acota a un estado.",
     "Censo y DENUE en HDFS (`python3 bdp_start.py datos`). Con el recorte de un estado basta el driver de 2 GB."),
    ("+ métricas (AUC ~0.92).",
     "+ métricas. Con datos nacionales el AUC ronda 0.92; con un solo estado hay menos de 100 casos de prueba "
     "y puede salir más alto: coméntalo en tu reporte."),
    ("contra el DENUE y el Censo NACIONAL) **cada vez** que predices un punto. Por eso la inferencia es lenta "
     "y necesita ~6 GB de memoria de driver.",
     "contra el DENUE y el Censo) **cada vez** que predices un punto. Por eso la inferencia es lenta: con los datos "
     "nacionales necesitaba ~6 GB de memoria de driver; con el recorte de un estado basta el driver de 2 GB."),
]
CODE_FIX = [
    ("# Funciona igual en Portable (Spark 3.4) y Container (Spark 4.0): el conector\n"
     "# Spark-ES no existe para Spark 4.0, así que usamos el cliente Python, que es\n",
     "# El conector Spark-ES no existe para Spark 4.x, así que usamos el cliente Python, que es\n"),
    # El cliente ya trae request_timeout=120; repetirlo en bulk() dispara un DeprecationWarning en elasticsearch 8.
    ("chunk_size=2000, request_timeout=120)", "chunk_size=2000)"),
]
USED = set()


def fix(src, pairs):
    for old, new in pairs:
        if old in src:
            src = src.replace(old, new)
            USED.add(old)
    return src


def patch_md(src, semana):
    src = MD_VARIANT.sub("> ⚙️ **Laboratorio BDP v7 (Codespaces o portable) · Spark 4.1 · Scala 2.13 · Sedona 1.9.1.**", src)
    src = MD_CHECK.sub("> Un solo cuaderno para todas las plataformas, con el kernel **BDP v7 (Spark 4.1 · Python 3.12)**. "
                       "Datos de práctica: recorte de Aguascalientes, cargado en HDFS con `python3 bdp_start.py datos`.", src)
    src = fix(src, MD_FIX)
    return MD_DOWNLOAD.sub(f"⬇️ **Repositorio:** https://github.com/abxda/bdp-lab-nube/tree/main/notebooks/{semana}", src)


def patch(src):
    src = fix(src, CODE_FIX)
    src = PKG_MULTI.sub(lambda m: f"\n{m.group(1)}{NOTE}", src)
    src = PKG_SINGLE.sub(lambda m: f"\n{m.group(1)}{NOTE}", src)
    src = PKG_LIST.sub(lambda m: f"{m.group(1)}{NOTE}", src)
    src = MEM.sub('.config("spark.driver.memory", "2g")  # BDP v7: recorte de un estado', src)
    src = DOWNLOAD.sub(lambda m: f"{m.group(1)}import shutil; shutil.rmtree({m.group(3)}, ignore_errors=True)  "
                                 f"# BDP v7: re-ejecutar no duplica filas\n"
                                 f"{m.group(1)}client.download({m.group(2)}, {m.group(3)}, overwrite=True)", src)
    return PIP.sub(lambda m: f"{m.group(1)}# BDP v7: ya viene instalado en el kernel 'BDP v7' -> {m.group(2)}", src)


for rel_src, rel_dest in FILES.items():
    src, dest = SRC / rel_src, DEST / rel_dest
    dest.parent.mkdir(parents=True, exist_ok=True)
    if src.suffix == ".ipynb":
        nb = json.loads(src.read_text(encoding="utf-8"))
        changed = 0
        for c in nb["cells"]:
            if c["cell_type"] == "markdown":
                c["source"] = patch_md("".join(c["source"]), rel_dest.split("/")[0]).splitlines(keepends=True)
            if c["cell_type"] == "code":
                old = "".join(c["source"])
                new = patch(old)
                changed += new != old
                c["source"] = new.splitlines(keepends=True)
                c["outputs"], c["execution_count"] = [], None
        nb.setdefault("metadata", {})["kernelspec"] = KERNEL
        dest.write_text(json.dumps(nb, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
        print(f"{rel_dest}: {changed} celdas ajustadas")
    elif src.suffix == ".py":
        old = src.read_text(encoding="utf-8")
        new = patch(old)
        dest.write_text(new, encoding="utf-8")
        print(f"{rel_dest}: {'ajustado' if new != old else 'sin cambios'}")
    else:
        shutil.copy2(src, dest)
        print(f"{rel_dest}: copiado")

for old, _ in MD_FIX + CODE_FIX:
    if old not in USED:
        print(f"AVISO: no se encontró en la fuente (¿cambió Curso_BDP?): {old[:70]}")
