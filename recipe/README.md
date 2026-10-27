# BDP v7 "Convergente": receta única del Portable

Una sola receta construye el mismo laboratorio en **Windows, macOS (Apple Silicon) y Linux**, de forma nativa en cada SO y con el mismo procedimiento. Las versiones son idénticas por construcción y se acabaron los tarballs armados a mano.

| Archivo | Para qué |
|---|---|
| `stack.toml` | versiones, URLs y hashes de cada pieza (JDK, Hadoop, Kafka, ES, Git Bash, jars, Python) |
| `requirements.in` / `requirements.lock` | dependencias de Python. **Un solo lock universal** con hashes para los 3 SO (`uv pip compile --universal`) |
| `build_portable.py` | arma el portable desde la receta: descarga, **verifica hashes**, adelgaza, configura y empaqueta |
| `smoke_test.py` | prueba de humo sin interfaz: perfil `up` (HDFS + Spark/Sedona/MLlib + ES) y perfil `unir` (HDFS + Spark + Kafka) |
| `publish.py` / `write_manifest.py` | publican **solo bajo `v7/`** en HF (`abxda/bdp-lab`). El manifiesto lo escribe un único job |
| `make_state_tiers.py` | recortes por estado de los datos nacionales (UP S3-S4); p. ej. `01` = Aguascalientes |
| `templates/` | configuración del lab: Kafka segura para Windows, ES sin ML y con CORS, Jupyter sin token con terminal bash, shims bash de Hadoop |

## Construir localmente

Requiere Python ≥ 3.11 y, para el launcher, Go + Wails v2.12.

```bash
wails build -platform windows/amd64 -trimpath -skipbindings -clean     # o linux/amd64 -tags webkit2_41, darwin/arm64
python recipe/build_portable.py --platform win-64 --out dist/portable --launcher build/bin/bdpv6-launcher.exe --package
python recipe/smoke_test.py --dist dist/portable --profile up
python recipe/smoke_test.py --dist dist/portable --profile unir
```

- El build es **nativo**: cada SO construye su propio portable. El CI (`.github/workflows/build-v7.yml`) lo hace en los 3 runners.
- Las descargas quedan en `.cache/v7/downloads` (ignorado por git) y se reverifican en cada uso.
- En una red corporativa con inspección TLS, el build confía en el almacén de certificados del SO (`truststore`).

## Layout resultante (el que espera el launcher)

```
bdpv6-launcher(.exe | .app)   common_jdk/   hadoop/   kafka_kraft/   elasticsearch/   python/
jupyter-config/   notebooks/   data/   logs/   bdp-stack.json   [bash/  solo Windows]
```

Diferencias contra v6:
- No hay carpeta `spark/`: `SPARK_HOME` es el paquete `pyspark`, que trae los jars de Spark y los de Sedona y Kafka pre-cargados.
- Hay un solo JDK (Temurin 17). ES lo usa vía `ES_JAVA_HOME`.
- `jupyter-config/` funciona como marcador de layout v7 (`paths.IsV7()`).

## Decisiones probadas (ver `D:\BDP\v7_spike\SPIKE_REPORT.md`)

- **JDK 17 + WebHDFS:** `HADOOP_OPTS` con `--add-opens` (`java.lang`, `java.util`, `java.nio`). Sin eso, WebHDFS responde 500.
- **HDFS solo en localhost:** `dfs.datanode.hostname=localhost` hace que WebHDFS redirija a `localhost` y no a la IP de la LAN. Además se aceptan usuarios que empiezan con número.
- **Git Bash en Windows:**
  - `hdfs`, `hadoop` y `mapred` son shims que llaman a su `.cmd` con `MSYS_NO_PATHCONV=1` **local**.
  - Si esa variable fuera global rompería los `.sh` de Kafka.
  - `KAFKA_LOG4J_OPTS` usa ruta `file:///`.
- **Kafka en Windows:**
  - Sin retención, sin compactación, sin `--delete` y sin snapshots por tiempo.
  - Se probó con apagado forzado y reinicio: 0 archivos `.deleted` o `-stray`.
- **Elasticsearch:**
  - `path.data: ../data/es_data`, relativo a `ES_HOME`. En v6 era `../../` y el índice quedaba **fuera** del portable.
  - Plantilla con réplicas 0 (índices en verde).
- **Rutas locales en Spark:** como HDFS es el FS por omisión, lo local se escribe con `file:///`.
- **Sedona 1.9 `ST_Transform`** con códigos EPSG necesita internet. Sin red, usar una cadena PROJ o pyproj.
