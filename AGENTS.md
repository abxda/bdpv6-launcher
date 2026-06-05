# AGENTS.md — build & contribute instructions for AI agents

This file is the entry point for AI coding agents (Claude Code, Cursor,
Aider, etc.) working on this repository. It is also useful for human
contributors — but written tersely for an agent that can read code but
hasn't been here before.

The launcher is **fully cross-platform**: Windows (WebView2), macOS (WKWebView),
and Linux (WebKit2GTK) all build from this same source. The current author
works on Windows; we need agents to be able to produce working binaries
on macOS and Linux too, since Cgo + the platform webview means cross-
compilation from Windows is not viable.

Platform-specific code is split via build tags (`//go:build windows` vs
`//go:build darwin || linux`). On the Unix side, macOS and Linux share
identical POSIX semantics (process groups, signals, `lsof`), so a single
`_unix.go` file covers both.

---

## 0 — TL;DR for an agent picking this up cold

### macOS (Apple Silicon or Intel)

```bash
git clone git@github.com:abxda/bdpv6-launcher.git
cd bdpv6-launcher
xcode-select -p                       # must print a path
go version                            # need >= 1.23
wails version                         # need v2.12.0
wails build -platform darwin/universal -trimpath -clean
open build/bin                        # → BDPV6 Launcher.app
```

### Linux (Ubuntu / Debian)

```bash
sudo apt update && sudo apt install -y \
    build-essential libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config
git clone git@github.com:abxda/bdpv6-launcher.git
cd bdpv6-launcher
go version                            # need >= 1.23
wails version                         # need v2.12.0
wails build -platform linux/amd64 -trimpath -clean
./build/bin/bdpv6-launcher            # plain ELF binary
```

### Linux (Fedora / RHEL)

```bash
sudo dnf install -y gcc gtk3-devel webkit2gtk4.0-devel pkg-config
# rest identical to Ubuntu
```

If any of the verifications fail, follow Section 1 below before retrying.

---

## 1 — Required toolchain on macOS

### 1.1 — Xcode Command Line Tools

Wails on macOS needs the C compiler and the WKWebView framework headers.

```bash
xcode-select --install                # GUI prompt; accept defaults
xcode-select -p                       # expect: /Library/Developer/CommandLineTools
```

If `xcode-select -p` returns nothing, the install was cancelled — retry.

### 1.2 — Go (1.23 or newer)

Recommended via Homebrew:

```bash
brew install go
go version                            # expect: go version go1.23.x darwin/...
```

If Homebrew is unavailable, install from <https://go.dev/dl/> and ensure
`/usr/local/go/bin` is on `PATH`.

### 1.3 — Wails CLI v2.12

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
wails version                         # expect: v2.12.0
```

Pinning to **v2.12.0 exactly** matches the version we develop against on
Windows. Using a newer Wails minor is allowed if you bump `go.mod` and
verify the build still passes; do not regress the version.

### 1.4 — Sanity check the host

```bash
wails doctor
```

This prints every dependency Wails detected and flags anything missing. A
green report is the prerequisite for `wails build` to succeed.

---

## 2 — Build commands

All build commands assume the working directory is the repo root.

### 2.1 — Production build (universal binary, x86_64 + arm64)

```bash
wails build -platform darwin/universal -trimpath -clean
```

- `-platform darwin/universal` → builds a fat binary that runs on both
  Apple Silicon and Intel Macs.
- `-trimpath` → strips local filesystem paths from the binary (smaller,
  reproducible).
- `-clean` → wipes `build/bin/` first.

Build output: `build/bin/BDPV6 Launcher.app/`. The directory IS the app
bundle; double-click on a Mac to run it.

### 2.2 — Single-arch build (faster, for iteration)

```bash
# Apple Silicon only
wails build -platform darwin/arm64 -trimpath

# Intel only
wails build -platform darwin/amd64 -trimpath
```

### 2.3 — Development mode (live reload)

```bash
wails dev
```

Opens a dev window connected to the Go process; edits in `frontend/src/`
hot-reload without a rebuild. Use this for UI iteration.

---

## 3 — Verifying the build

After `wails build`:

1. The bundle exists: `ls -d "build/bin/BDPV6 Launcher.app"`.
2. The binary runs: `open "build/bin/BDPV6 Launcher.app"` — a window titled
   "BDPV6 Launcher" should open within 2–3 s.
3. (If you have a BDP distribution at hand) copy the bundle next to
   `common_jdk/`, `hadoop/`, etc. and verify the Dashboard tab shows
   the layout as detected and `Distribución BDP detectada correctamente`.

If step 2 fails with a Gatekeeper warning, that is expected for unsigned
builds — see Section 4.

---

## 4 — Code signing & Gatekeeper

The `.app` produced by `wails build` is **not** signed with an Apple
Developer ID. End users on macOS Catalina+ will see "BDPV6 Launcher cannot
be opened because the developer cannot be verified".

### 4.1 — Ad-hoc signing (good enough for sharing internally)

```bash
codesign --force --deep --sign - "build/bin/BDPV6 Launcher.app"
```

The `-` (dash) is the ad-hoc identity — no Apple account required. This
removes the worst Gatekeeper warnings and lets users right-click → Open
without an error dialog.

### 4.2 — Removing the quarantine attribute (end user workaround)

After downloading from a browser or AirDrop, macOS attaches a quarantine
xattr. The end-user can clear it with:

```bash
xattr -dr com.apple.quarantine "BDPV6 Launcher.app"
```

This is documented in the user-facing README.

### 4.3 — Full Developer ID signing (out of scope for now)

If/when we acquire an Apple Developer ID, the proper command is:

```bash
codesign --force --deep --options runtime \
    --sign "Developer ID Application: Abel Coronado (TEAMID)" \
    "build/bin/BDPV6 Launcher.app"

xcrun notarytool submit "BDPV6 Launcher.zip" \
    --keychain-profile "BDPV6" --wait

xcrun stapler staple "build/bin/BDPV6 Launcher.app"
```

Do **not** attempt this without explicit instruction — it requires
credentials and a paid Apple Developer account.

---

## 5 — Repository conventions

Read these before making changes. They are not optional.

### 5.1 — No npm, no Vite, no transpilation

The frontend is vanilla HTML/CSS/JS embedded with `//go:embed all:frontend/src`.
Do **not** add a `package.json`, `vite.config.js`, TypeScript, JSX, or any
build step. New dependencies are inlined (e.g. Lucide icons → SVG path
strings in `frontend/src/app.js` under the `ICONS` object).

### 5.2 — No third-party Go modules without a clear reason

The launcher must build with `CGO_ENABLED=1` on Windows (for WebView2Loader)
and on macOS (for WKWebView). New Go dependencies must compile under both
toolchains. Prefer the standard library; add `gopsutil/v3` for CPU/RAM in
F8, but justify anything else.

### 5.3 — Cross-platform code via build tags

Platform-specific files use the `_windows.go` and `_darwin.go` suffix
(Go's filename build constraints). Avoid `runtime.GOOS == "windows"`
branches in code that has cleaner separation via filenames.

### 5.4 — Spanish-first UI, English available

User-facing strings live in `frontend/src/app.js` under `STR.es` (primary)
and `STR.en` (parity required). Use the `t(key, ...args)` helper to render.

### 5.5 — Coding style

- Go: standard `gofmt`. Run `go vet ./...` before pushing.
- JS: 4-space indent, single quotes for strings, no trailing semicolons
  inside template literals.
- CSS: design tokens (CSS custom properties) for any color/size; component
  classes prefixed `.c-*` (e.g. `.c-btn`, `.c-card`, `.c-sidebar`).

---

## 6 — Test plan an agent should run before opening a PR

For pure backend changes (`internal/...`):

```bash
go vet ./...
go build ./...
```

For UI changes, after building, manually verify:

1. App opens to the Dashboard.
2. Sidebar navigation between tabs works (placeholder views are fine
   in early phases).
3. `GetEnvInfo()` populates the Dashboard with detected paths.
4. Closing the window cleanly exits the process (`ps aux | grep BDPV6`
   shows no leftover).

For phase-specific changes, follow the verification checklist for that
phase in `docs/PLAN.md` (mirror of the canonical plan).

---

## 7 — Reporting back

When you finish a task, leave the repo in this state and reply with:

- A short summary (5–10 lines) of what you changed and why.
- The `git status` output and the commit SHA(s) you created.
- Any build/test command output that proves the change works
  (`wails build` log tail, `go vet` clean, screenshots if UI).
- Anything you noticed but did **not** fix (bugs, smells, TODOs).

Do not silently leave unrelated files modified. If you touched anything
outside the scope of the task, mention it explicitly.

---

## 8 — Quick reference: directory layout

```
.
├── main.go                       Wails entry
├── app.go                        App struct + JS-bound methods
├── go.mod / go.sum
├── wails.json                    Wails build config
├── internal/                     Backend Go packages
│   ├── paths/                    SCRIPT_DIR + service paths
│   ├── state/                    .bdp_state.json
│   ├── services/                 Service lifecycle (F2-F3)
│   ├── processctl/               Cross-platform proc control (F2)
│   ├── health/                   Probes (F2)
│   ├── ports/                    Scanner + collision (F4)
│   ├── setup/                    First-run wizard (F5)
│   ├── repair/                   Cleanup / reformat (F6)
│   ├── hdfsfs/                   WebHDFS REST client (F7)
│   ├── logsink/                  Ring buffer + tail (F2)
│   └── sysinfo/                  CPU / RAM telemetry (F8)
├── frontend/
│   └── src/
│       ├── index.html
│       ├── app.js                ICONS + STR + view registry
│       └── style.css             Tokens + .c-* components
└── build/                        Wails build assets
    ├── appicon.png
    ├── darwin/Info.plist
    └── windows/info.json
```

Welcome aboard — when in doubt, read `docs/PLAN.md` for the full design.

---

## Publishing & self-update (READ BEFORE RELEASING)

This launcher ships **inside** the ~2.7 GB Portable `.tar.gz` on Hugging Face only
as a **seed**. Do **NOT** re-upload 2.7 GB to ship a launcher change — use the
**self-update** path (`selfupdate.go`), which costs ~12 MB.

How self-update works (`checkSelfUpdate()` runs at `startup`, before services):
1. Fetches `launchers/bdpv6-launcher-latest-<os>-<arch>.json` from the HF dataset
   `abxda/bdp-lab` (descriptor: `{version, sha256, url}`).
2. If `descriptor.version != AppVersion`, downloads the exe at `url`, verifies its
   **SHA-256**, swaps itself (`exe → .old`, `new → exe` — Windows allows renaming a
   running exe), relaunches and quits. The heavy stack is never re-downloaded.

To cut a launcher release:
1. **Bump `var AppVersion` in `app.go`.** The published descriptor MUST declare the
   **same** version, or clients won't update (or loop).
2. `wails build -platform windows/amd64 -skipbindings`.
3. Upload `build/bin/bdpv6-launcher.exe` → `launchers/bdpv6-launcher-windows-amd64.exe`;
   verify its HF `oid` == local sha256.
4. Update `launchers/bdpv6-launcher-latest-windows-amd64.json` with the new
   `version`+`sha256` (upload the exe FIRST, descriptor AFTER).

Full step-by-step (commands, verification, anti-clobber, the cheap-vs-expensive
matrix) lives in the course runbook **`Curso_BDP/PUBLICAR_Y_ACTUALIZAR.md` §3**
(and §5 for when the heavy stack itself must be republished).

> Known gap: only the **windows-amd64** descriptor/exe exist today. Apple Silicon
> Portable has no auto-update yet — publish `launchers/bdpv6-launcher-{macos-arm64.exe,
> latest-macos-arm64.json}` to enable it (the code already builds the URL from
> `runtime.GOOS/GOARCH`).
