# BDPV6 Launcher

Cross-platform launcher for the **Big Data Portable (BDP)** distribution — a
self-contained bundle of HDFS, Kafka (KRaft), Elasticsearch, Spark, and
Jupyter that students copy to their machine and run without Docker. The
launcher replaces two divergent PyQt5 launchers (`BDPV4_WIN/launcher.py` and
`BDPV5_macOS/.../launcher.py`) with a single Go + Wails v2 binary that runs
on Windows and macOS.

> **Status:** all nine phases delivered. See [`docs/PLAN.md`](./docs/PLAN.md)
> for the design and verification plan.

## Why

The legacy Python launchers had three pain points students kept hitting:

1. **Services fail silently** — stdout/stderr is invisible inside the launcher.
2. **Port collisions** are not detected until a service crashes on bind.
3. **Repair / reset** is a sprawl of `.bat` / `.sh` scripts that students
   have to discover and run manually.

This launcher fixes those with live diagnostic consoles per service, a
proactive port collision detector, and a unified Repair tab.

## Stack

| Layer            | Tech                                                      |
| ---------------- | --------------------------------------------------------- |
| Framework        | [Wails v2.12](https://wails.io) — Go ↔ WebView2 bridge   |
| Backend          | Go 1.23+                                                  |
| Renderer         | WebView2 on Windows · WKWebView on macOS                  |
| Frontend         | Vanilla HTML5 + CSS3 + JavaScript ES2020 — **no Vite, no npm** |
| Icons            | [Lucide](https://lucide.dev) inlined as SVG strings       |
| Distribution     | Single `.exe` (~12 MB) on Windows · `.app` bundle on macOS|

The launcher binary is placed next to the BDP service folders
(`common_jdk/`, `hadoop/`, `kafka_kraft/`, `elasticsearch/`, `python/`,
`spark/`, `notebooks/`, `data/`, `logs/`) and detects its own location with
`os.Executable()`.

## Quick start (user)

1. Download the latest `bdpv6-launcher.exe` (Windows) or `BDPV6 Launcher.app`
   (macOS) from the [Releases](https://github.com/abxda/bdpv6-launcher/releases).
2. Drop it into the root of your BDP distribution, alongside `common_jdk/`,
   `hadoop/`, etc.
3. Double-click. On macOS, the first launch may require:
   ```bash
   xattr -dr com.apple.quarantine "BDPV6 Launcher.app"
   ```

## Build from source

### Windows

Requires: Go 1.23+, [Wails CLI v2.12](https://wails.io/docs/gettingstarted/installation),
and `gcc` from [MSYS2](https://www.msys2.org) (`pacman -S mingw-w64-x86_64-toolchain`).

```bash
export PATH="/c/Program Files/Go/bin:$HOME/go/bin:/c/msys64/mingw64/bin:$PATH"
export CGO_ENABLED=1
wails build -platform windows/amd64 -trimpath
```

Output: `build/bin/bdpv6-launcher.exe`.

### macOS

See [AGENTS.md](./AGENTS.md) for the full build recipe (Xcode CLT, Go,
Wails CLI, code-signing notes).

```bash
wails build -platform darwin/universal -trimpath
```

Output: `build/bin/BDPV6 Launcher.app`.

## Repository layout

```
.
├── main.go                       Wails entry point
├── app.go                        App struct + JS-bound methods
├── wails.json                    Wails config (title, output, info.*)
├── go.mod / go.sum
├── internal/
│   ├── paths/    paths.go        SCRIPT_DIR + all derived service paths
│   ├── state/    state.go        Persistent user state (.bdp_state.json)
│   ├── services/                 Service lifecycle (F2 – F3)
│   ├── processctl/               Cross-platform process control (F2)
│   ├── health/                   TCP + HTTP probes (F2)
│   ├── ports/                    Port scanner + collision detector (F4)
│   ├── setup/                    First-run wizard logic (F5)
│   ├── repair/                   Reset/cleanup actions (F6)
│   ├── hdfsfs/                   WebHDFS REST client (F7)
│   ├── logsink/                  Ring buffer + tail-to-file (F2)
│   └── sysinfo/                  CPU / RAM telemetry (F8)
├── frontend/
│   └── src/
│       ├── index.html            App shell (sidebar + content)
│       ├── app.js                ICONS, STR (i18n), view registry
│       └── style.css             Design tokens + .c-* components
├── build/                        Wails build assets (icons, plists, manifests)
├── docs/PLAN.md                  Full design + nine-phase roadmap
├── LICENSE                       MIT
├── AGENTS.md                     Build instructions for AI agents (macOS focus)
└── README.md
```

## Plan

| Phase | Deliverable                                            | Status |
| ----- | ------------------------------------------------------ | ------ |
| F1    | Wails skeleton + paths + state                         | done   |
| F2    | Process control + first service (Elasticsearch)        | done   |
| F3    | Remaining services (Kafka, HDFS NN/DN, Jupyter)        | done   |
| F4    | Ports tab + collision detection                        | done   |
| F5    | First-run wizard + setup                               | done   |
| F6    | Repair tab (unified cleanup / reformat / repair)       | done   |
| F7    | HDFS explorer + notebooks                              | done   |
| F8    | Settings + i18n + design polish                        | done   |
| F9    | End-to-end verification + docs + distro cleanup        | done   |

## Tests

```bash
# Unit / integration (fast, no external dependencies)
go test ./...

# Real end-to-end against a populated BDP distribution. Spawns the bundled
# Java services for ~30 s each, so it is opt-in via the `e2e` build tag.
BDP_DIST=D:\BDP\BDPV4_WIN go test -tags=e2e -timeout 180s ./internal/services -run TestE2E_Elasticsearch -v
```

The E2E test in `internal/services/e2e_test.go` confirms that
processctl, sinkWriter, logsink, and the HTTP health probe form a
working pipeline end-to-end. It was used to validate F9 on a real
distribution: Elasticsearch became healthy in 24 s, then was stopped
cleanly with no leftover `java.exe` process.

## License

[MIT](./LICENSE). Bundles [Lucide](https://lucide.dev) (ISC) and
[Wails](https://wails.io) (MIT).
