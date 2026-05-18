# Plan — BDPV6 Launcher (Wails v2 cross-platform Win/Mac)

> Canonical, version-controlled mirror of the approved plan. The original
> lives at `C:\Users\abel.coronado\.claude\plans\warm-wandering-harbor.md`
> on the author's machine; this copy is the source of truth for
> contributors and AI agents working on the repo.

## Contexto

El proyecto **Big Data Portable (BDP)** distribuye un stack pre-empaquetado
(JDK 11 + Hadoop 3.3.x + Spark 3.4.x + Kafka 3.7 KRaft + Elasticsearch 8.x
+ Python 3.10 + Jupyter) en una carpeta autocontenida que los alumnos copian
a su disco. Hoy tiene **dos launchers PyQt5 divergentes** (`BDPV4_WIN/launcher.py`
v3.5 y `BDPV5_macOS/BDPV5_mac/launcher.py` v1.1 "Quasar") con duplicación,
bugs concretos y mala observabilidad. El alumno típico se atora porque:

1. **Los servicios fallan en silencio** — stdout/stderr de Java se va a
   ventanas separadas (Win) o a un archivo (Mac), pero el launcher solo
   muestra una bitácora propia que no incluye el detalle.
2. **Hay colisiones de puertos** sin diagnóstico anticipado ni sugerencia
   de alternativa.
3. **No hay forma guiada de "reparar / reiniciar"** — cleanup, reformat HDFS,
   reformat Kafka, reparar Jupyter están en scripts sueltos (`cleanup.bat`,
   `setup_first_run.bat`, `repair_jupyter.sh`) que el alumno debe descubrir
   y ejecutar manualmente, y peor: `setup_first_run.bat` en Windows **no se
   invoca desde el launcher**, así que el primer arranque siempre tira un
   error críptico.

Bugs concretos heredados que este plan resuelve:

- `BDPV4_WIN/launcher.py:257-258` — Jupyter se invoca como
  `python.exe jupyter-lab.exe --port=…` (frágil; debe ser `python -m jupyterlab`).
- `BDPV4_WIN/launcher.py:351` — `InsecureClient(..., user='abel.coronado')`
  hardcoded.
- `BDPV4_WIN/launcher.py:343-386` — `populate_hdfs_tree` corre en el hilo
  de GUI (congela).
- `BDPV4_WIN/launcher.py:280` — `CREATE_NEW_CONSOLE` impide capturar
  stdout/stderr.
- `BDPV5_macOS/.../launcher.py:172` — `os.getlogin()` falla en headless.
- `BDPV5_macOS/.../launcher.py:289,336` — file handles de logs sólo se
  cierran en `stop_all_services`; leak si el usuario mata el proceso.
- `BDPV5_macOS/BDPV5_mac/hadoop/etc/hadoop/{core,hdfs}-site.xml` — paths
  hardcoded a `/Users/Sl4sh3r99/…` (la regeneración existe pero ocurre en
  primer arranque; los archivos del template son contaminantes).
- `BDPV5_macOS/BDPV5_mac/` — 13 archivos `._*` (resource forks de macOS)
  basura.
- Drift de versiones entre distros: ES 8.18.2 (Win) vs 8.13.4 (Mac);
  Hadoop 3.3.4 vs 3.3.6; Spark 3.4.4 vs 3.4.3.

**Resultado esperado:** un único binario por plataforma (`bdpv6-launcher.exe`
~12 MB, `BDPV6 Launcher.app` similar) que reemplaza al `launcher.py`, con
consolas de diagnóstico en vivo, gestor de colisiones de puertos, y un
módulo "Reparar" que unifica los scripts existentes. Frontend HTML/CSS/JS
vanilla (sin Vite/npm), tokens CSS + componentes `.c-*`, iconos Lucide
inline. Una sola fuente Go con `build tags` para las diferencias Win/Mac.

## Decisiones de arquitectura

| Decisión | Elección | Razón |
|---|---|---|
| Organización | Un repo único `bdpv6-launcher` | Garantiza paridad Win/Mac; un solo set de cambios. |
| Backend | Go 1.23+, Wails v2.12.0, `CGO_ENABLED=1` con gcc MSYS2 en Win | Stack validado por el autor. |
| Frontend | HTML5 + CSS3 + JS ES2020 vanilla, sin build step (`//go:embed all:frontend/src`) | Cero npm. |
| Iconos | Lucide inline como SVG en `frontend/src/app.js` (`ICONS`) | Sin dependencias de red. |
| i18n | Objeto `STR` (es/en) | Sin librería. |
| Reemplazo del Python launcher | Completo en distribución | Decidido por el autor. |
| Ubicación del binario | Al lado de las carpetas de servicio | El launcher detecta `SCRIPT_DIR` vía `os.Executable()`. |
| Persistencia de estado | `.bdp_state.json` al lado del binario | Único archivo, fácil de borrar para "reset de fábrica". |

## Tabs del UI

1. **Dashboard** — vista compacta: tarjetas de servicios + CPU/RAM + setup alert.
2. **Servicios** — control fino por servicio.
3. **Puertos** — detección de colisiones + sugerencia de puerto libre.
4. **Consolas** — diagnóstico en vivo, una subtab por servicio.
5. **HDFS** — árbol WebHDFS con lazy-loading.
6. **Reparar** — limpiar / reformatear HDFS / reformatear Kafka / reparar Jupyter.
7. **Notebooks** — listado + doble click abre en Jupyter.
8. **Configuración** — puertos, JVM heap, idioma, "always on top", auto-Jupyter.

## Fases

| Fase | Entregable |
|---|---|
| **F1** | Esqueleto Wails, `paths.go`, `state.go`, app shell + nav, Dashboard funcional. |
| **F2** | `procctl` Win+Mac, Elasticsearch como primer servicio, consola en vivo. |
| **F3** | Kafka, HDFS NN/DN, Jupyter (con auto-token + auto-open browser). |
| **F4** | Tab Puertos con detección de proceso ocupante (`netstat+tasklist` / `lsof`). |
| **F5** | First-run wizard (4 pasos: JDK, XML, format HDFS, format Kafka). |
| **F6** | Tab Reparar (limpiar / reformatear / repair_jupyter, todo en UI). |
| **F7** | HDFS Explorer (WebHDFS REST puro Go) + Notebooks. |
| **F8** | Settings + i18n completo + diseño visual pulido. |
| **F9** | Verificación E2E en Win y Mac, limpieza distros, docs. |

## Cómo verificar (Windows, sesión actual)

1. Build: `wails build -platform windows/amd64 -trimpath` (CGO+gcc MSYS2 en PATH). Output ≤ 20 MB.
2. Copiar `bdpv6-launcher.exe` a `D:\BDP\BDPV4_WIN\bdpv6-launcher.exe`. Doble click → abre WebView2.
3. **First-run**: sin `data/hdfs/namenode/current/VERSION` → wizard arranca; 4 pasos; verificar `core-site.xml` regenerado tiene `file:///D:/BDP/BDPV4_WIN/data/hadoop_tmp`.
4. **Iniciar TODO**: 5 servicios `healthy` en <60 s. Consolas muestran stdout/stderr en vivo.
5. **Colisión de puerto**: ocupar 9200 con `python -m http.server 9200`; tab Puertos marca Python como ocupante; "Sugerir libre" propone 9201.
6. **Jupyter auto-open**: tras Iniciar TODO, badge "Abrir Jupyter" se activa en <15 s.
7. **Reparar → Limpiar**: confirmación doble; `data/` y `logs/` quedan vacíos; siguiente arranque dispara wizard.
8. **Detener TODO + cerrar**: 0 procesos Java vivos (`tasklist | findstr java`).
9. **Stress de consola**: 5 min de HDFS → buffer no crece indefinido; RAM launcher < 150 MB.

## Cómo verificar (macOS)

Equivalente con `wails build -platform darwin/universal`. Ver
[`AGENTS.md`](../AGENTS.md) para el procedimiento completo.

## Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| WebView2 ausente en Win 10 < 22H2 | Wails v2.12 inyecta el bootstrapper; detectar al arrancar y mostrar instrucciones si falta. |
| Gatekeeper macOS bloquea `.app` no firmado | Documentar `xattr -dr com.apple.quarantine`; firma Developer ID en v1.1. |
| Procesos Java zombies si el launcher crashea | `OnShutdown` corre `StopAll`; watchdog opcional con `pids.json`. |
| Cambio de puerto Kafka requiere reformat | UI avisa explícitamente; ofrece "cambiar y reformatear" atómico con doble confirmación. |
| Alumno mueve el `.exe` a otra carpeta | `paths.go` valida que las carpetas existen junto al binario; modal informativo si falla. |
| Drift de versiones (ES/Hadoop/Spark) | Fuera del alcance del launcher; issue separado para alinear distros. |
