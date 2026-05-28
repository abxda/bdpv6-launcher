# Cómo compilar BDPV6 para Mac Intel (darwin/amd64) — reflexión

> **Status:** reflexión, no plan ejecutado. Este doc lista las opciones que
> tenemos para producir una build BDPV6 funcional para Mac Intel (x86_64)
> *sin* tener acceso físico a una Mac Intel — recursos actuales: una Mac
> Apple Silicon (M2) y una máquina Linux Ubuntu. Sirve para decidir qué
> camino tomar antes de invertir tiempo.

## Por qué este documento existe

La distro BDPV6 actual (incluyendo el `.tar.gz` en `deploy-mac/` del drive
portátil y los binarios desplegados por los agentes de Mac) está compilada
para **`darwin/arm64`** — sirve para Macs M1/M2/M3/M4 pero **no arranca en
Mac Intel**. Rosetta 2 traduce x86 → arm64, no al revés, así que no hay
forma de que un Mac Intel ejecute la build actual.

Mientras los estudiantes con Apple Silicon tienen un camino fluido
("extrae y arranca", apoyado por el heal del PR #3), los estudiantes con
Mac Intel quedan completamente bloqueados. Este doc enumera caminos para
desbloquearlos.

---

## El problema es de dos capas

| Capa | Qué es | Por qué es arch-específico |
|---|---|---|
| **A. El launcher** | `BDPV6 Launcher.app` — binario Go + Wails (CGo + WebKit) | Mach-O nativo; necesita compilarse por arch |
| **B. La distro** | JDK, Python, Elasticsearch, Hadoop, Kafka, Spark | JDK/Python/ES son binarios nativos; Hadoop/Kafka/Spark son JVM puro |

Estas capas se atacan **independientemente**. No se necesita resolver
ambas a la vez para validar la primera.

---

## Capa A — El launcher (Go + Wails)

### A.1 — Cross-compile aquí mismo, en esta Mac Apple Silicon (intentar primero)

Wails soporta cross-compile entre `darwin/arm64` y `darwin/amd64` porque el
SDK macOS de Xcode CLT contiene "fat" frameworks (slices arm64 y x86_64 en
el mismo archivo). Comando:

```bash
cd ~/bdpv6-launcher
wails build -platform darwin/amd64 -trimpath
# produce build/bin/BDPV6 Launcher.app — Mach-O x86_64
```

**Probabilidad de éxito:** alta. Las CGo bindings de Wails (AppKit, WebKit)
existen para ambas arquitecturas en el mismo SDK.

**Tiempo:** ~1 minuto de build si funciona a la primera. Si falla, lo más
común es un flag de linker que pida ajustes vía `CGO_LDFLAGS`.

**Costo:** prácticamente cero. Es la primera prueba obligatoria.

### A.2 — Build universal (un solo `.app` para ambas arches)

```bash
wails build -platform darwin/universal -trimpath
```

Internamente compila para ambas y usa `lipo` para fusionarlas en un binario
Mach-O universal. **Ventaja:** un solo `.app` que sirve a todos los
estudiantes — simplifica la distribución (no necesitas dos
`BDPV6-mac-*.tar.gz`). **Costo:** binario ~2× tamaño (es marginal, hablamos
de 18 MB → 36 MB). **Prerrequisito:** que A.1 funcione.

### A.3 — GitHub Actions con runner macOS Intel (recomendado para mantenimiento)

GitHub provee runners hospedados gratuitos para repos públicos:

- `macos-13` → Intel x86_64
- `macos-14` y `macos-latest` → arm64

Workflow ejemplo (`.github/workflows/build-mac.yml`):

```yaml
name: Build macOS launcher
on:
  push:
    tags: ['v*']
  workflow_dispatch:
jobs:
  build:
    strategy:
      matrix:
        include:
          - runner: macos-13   # Intel
            platform: darwin/amd64
          - runner: macos-14   # Apple Silicon
            platform: darwin/arm64
    runs-on: ${{ matrix.runner }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: brew install pkg-config
      - run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
      - run: wails build -platform ${{ matrix.platform }} -trimpath
      - uses: actions/upload-artifact@v4
        with:
          name: launcher-${{ matrix.platform == 'darwin/amd64' && 'amd64' || 'arm64' }}
          path: build/bin/*.app
```

**Por qué es la mejor opción a mediano plazo:**
- Builds nativos (sin riesgos de cross-compile).
- Reproducible — cada release queda con artefactos en GitHub.
- Cero hardware adicional.
- Gratuito para repos públicos.
- Se ejecuta automáticamente al taggear una release.

**Costo:** ~30 minutos de armado inicial del workflow. Después, cero
mantenimiento.

### A.4 — Cross-compile desde Ubuntu vía `osxcross`

[osxcross](https://github.com/tpoechtrager/osxcross) provee toolchain
darwin (clang + macOS SDK) para Linux. Permitiría compilar el launcher en
la máquina Ubuntu sin tocar el Mac. Setup aproximado:

1. Extraer una imagen SDK de Xcode 13+ (`Xcode_xx.xip` → `MacOSXxx.sdk`).
2. Compilar el toolchain osxcross (~30 min en hardware moderno).
3. Configurar `CC`, `CXX`, `CGO_ENABLED=1`, `GOOS=darwin`, `GOARCH=amd64`.
4. Correr `wails build -platform darwin/amd64`.

**Problemas:**
- **Legal:** redistribuir el SDK macOS no está permitido por la licencia
  de Xcode; extraerlo para uso personal está en zona gris (tolerado en la
  práctica). No subir el SDK al repo.
- **Frágil:** versiones nuevas de Wails/Go a veces rompen el cross-compile
  desde Linux porque dependen de cabeceras Objective-C específicas.
- **Mantenimiento:** la imagen del toolchain hay que actualizarla con cada
  nuevo Xcode/SDK.

**Cuándo tiene sentido:** si Apple Silicon no está disponible (no es
nuestro caso). Si A.1 funciona, **A.4 no aporta** valor adicional.

### A.5 — Rosetta no aplica al launcher

Rosetta 2 traduce *runtime* x86_64 → arm64. No es un compilador. No nos
ayuda a *producir* binarios amd64. Solo aparece como opción accidental si
instaláramos un Homebrew bajo Rosetta y usáramos su Go (que generaría
ejecutables x86_64 nativos). Es un camino convoluto y no aporta sobre A.1.

---

## Capa B — La distro (componentes pre-construidos)

La distro **no se compila**, se ensambla a partir de tarballs oficiales.
Tres tipos de componentes según portabilidad:

### B.1 — Componentes "JVM puros" reusables tal cual

| Componente | Razón |
|---|---|
| **Hadoop 3.3.6** | 99% Java. Las libs nativas en `lib/native/` ya están desactivadas en nuestra build (el WARN `NativeCodeLoader: Unable to load native-hadoop library` aparece incluso en arm64 — no las usa) |
| **Kafka KRaft** | 100% JVM |
| **Spark 3.4.x** | JVM + Scala; PySpark llama a Python pero el binario de Spark mismo no es arch-específico |

Estos se pueden **copiar tal cual** desde la distro arm64 actual a la
distro amd64. Ahorran ~3 GB de trabajo de re-descarga.

### B.2 — JDK amd64

**Fuente:** [Azul Zulu](https://www.azul.com/downloads/?package=jdk#zulu)
ofrece tarballs amd64 macOS de Zulu 11. Estructura exacta a la actual
(`zulu-11.jdk/Contents/Home/`), drop-in replacement.

```bash
curl -L -o zulu11-mac-amd64.tar.gz \
  "https://cdn.azul.com/zulu/bin/zulu11.xx.x-ca-jdk11.0.27-macosx_x64.tar.gz"
tar -xzf zulu11-mac-amd64.tar.gz -C common_jdk_amd64/
```

(URL exacta hay que confirmarla; Azul cambia el número de build con cada
release de parche.)

**Tamaño:** ~400 MB.

### B.3 — Python amd64 — el componente más delicado

La distro actual lleva un Python 3.10 con **muchos paquetes pre-instalados
vía pip** (Jupyter, pandas, numpy, scikit-learn, etc.). Esos paquetes
incluyen **wheels nativos** (numpy/scikit-learn compilan C/C++) que son
arch-específicos. No se pueden copiar desde la versión arm64.

Tres opciones:

**B.3.a — `python-build-standalone` + pip install desde una freeze**

[python-build-standalone](https://github.com/astral-sh/python-build-standalone)
publica tarballs portátiles de Python 3.10/3.11/3.12 para
`x86_64-apple-darwin` (y arm64). Estructura limpia, sin dependencias del
sistema. Process:

1. Aquí, en la distro arm64: `python3.10 -m pip freeze > requirements.lock`
2. Descargar el tarball amd64 de python-build-standalone.
3. Extraer a `python_amd64/`.
4. En una **Mac Intel** (o vía Rosetta — ver B.3.b), correr
   `python_amd64/bin/python3.10 -m pip install -r requirements.lock`.
5. Empaquetar.

**Problema:** el paso 4 requiere un host x86_64 con conexión a internet.
**Aquí está el obstáculo central de "no tengo Intel".**

**B.3.b — Truco con Rosetta para resolver el paso 4 en esta Mac**

Rosetta puede ejecutar binarios x86_64 en Apple Silicon. Si descargamos
un Python x86_64 standalone y lo invocamos aquí, **se ejecuta bajo
Rosetta como un proceso x86_64**, y los `pip install` instalan wheels
**x86_64** (porque pip mira el arch del intérprete, no del host).

```bash
softwareupdate --install-rosetta --agree-to-license  # si no está
# descargar python-build-standalone amd64
curl -L -O https://github.com/astral-sh/python-build-standalone/releases/.../cpython-3.10.x+xxxx-x86_64-apple-darwin-install_only.tar.gz
tar -xzf cpython-...x86_64....tar.gz   # crea python/
arch -x86_64 ./python/bin/python3.10 -m pip install -r requirements.lock
# todos los wheels resultantes serán x86_64; la carpeta entera es portable
```

**Esto resuelve el problema sin necesitar hardware Intel.** Tiempo
estimado: 30-60 min (depende de cuántos wheels haya que compilar).

**Validación posterior:** el `.tar.gz` resultante hay que probarlo en una
Mac Intel real (puede ser una sola sesión presencial con un estudiante o
colega). El testing en producción no se puede saltar.

**B.3.c — Docker amd64 en Ubuntu**

La máquina Ubuntu puede correr un contenedor `--platform=linux/amd64`
con Python 3.10 y producir un conjunto de wheels amd64. **Pero esos
wheels son Linux, no macOS**, así que no sirven directamente. Necesitarías
un contenedor que emule macOS, que no existe legalmente. Descartar.

### B.4 — Elasticsearch amd64

ES distribuye tarballs por arch:
https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-8.x.x-darwin-x86_64.tar.gz

Drop-in replacement de `elasticsearch/`. Lleva su propio JDK bundleado
adentro (también arch-específico).

**Tamaño:** ~700 MB.

### B.5 — Estrategia de armado

Crear un script `build_amd64_distro.sh` (en `scripts/` del repo, no del
drive) que:

1. Descarga los tarballs amd64 (JDK, Python standalone, ES) en
   `/tmp/bdpv6_amd64_sources/`.
2. Reusa Hadoop/Kafka/Spark/notebooks copiándolos de un
   `BDPV5_macOS_arm64/` de referencia (o re-descarga si prefieres).
3. Ensambla `BDPV5_macOS_amd64/` con la layout final.
4. Inyecta el `.app` Intel construido en A.1.
5. Genera `.tar.gz` excluyendo `data/`, `logs/`, `.bdp_state.json`
   (mismo patrón que el script actual para arm64).
6. Verifica `tar -tzf` y archivos clave.

Idempotente, reproducible, versionable.

---

## El rol de la máquina Ubuntu

Honestamente, **Ubuntu juega un papel secundario** en este problema:

| Ubuntu **sí** sirve para... | Ubuntu **no** sirve para... |
|---|---|
| Hostear el workflow de GitHub Actions (no, eso lo hostea GitHub). En realidad: descargar tarballs amd64 (cualquier OS puede hacer eso). | Compilar el launcher (necesita osxcross — complejo, ver A.4). |
| Correr scripts de ensamblado de la distro amd64 (tar/cp/curl son portables). | Ejecutar binarios macOS (no son compatibles). |
| Probar Python wheels para Linux (irrelevante a nuestro caso). | Instalar wheels macOS amd64 (no se ejecutan ahí). |
| Validar que el tar.gz extrae bien con `tar xzf`. | Probar que el launcher arranca (necesita una Mac). |

**Veredicto:** Ubuntu puede ayudar a automatizar descargas y ensamblado
pero no aporta a la parte difícil (los binarios deben ejecutarse en Mac).
Probablemente más útil sería **olvidarnos de Ubuntu para esto y resolver
todo desde esta Mac Apple Silicon + GitHub Actions** para el launcher
amd64.

---

## Camino recomendado (corto y de bajo riesgo)

Si todo lo que queremos es **una distro Intel funcional para los estudiantes
con Macs viejas**, este es el camino:

**Fase 1 — Validar la cross-compile del launcher (20 minutos aquí)**

```bash
cd ~/bdpv6-launcher
wails build -platform darwin/amd64 -trimpath
```

Si produce un `.app` Mach-O x86_64 sin errores, confirmamos que A.1
funciona y seguimos. Si no, troubleshooting de flags CGo o pasar a A.3
(GitHub Actions).

**Fase 2 — Montar el workflow de GitHub Actions (30 minutos)**

Crear `.github/workflows/build-mac.yml` con el matrix amd64+arm64. Hacer
un push de prueba, descargar los artefactos. Esto garantiza builds
nativos reproducibles para el futuro.

**Fase 3 — Ensamblar la distro amd64 (medio día)**

- Descargar JDK Zulu amd64, Elasticsearch amd64, Python standalone amd64.
- Correr `pip install -r <requirements de la distro arm64>` bajo Rosetta
  (B.3.b) para poblar `python/lib/python3.10/site-packages/` amd64.
- Copiar Hadoop/Kafka/Spark/notebooks tal cual.
- Inyectar el `.app` amd64 (Fase 1).
- `tar -czf BDPV6-mac-amd64.tar.gz BDPV5_macOS_amd64 Ejercicio_01`.

**Fase 4 — Validar en una Mac Intel real (presencial, 30 min)**

Inevitable. Buscar un estudiante o colega con Intel para correr el
WordCount E2E. Si pasa, distribuir.

**Inversión total estimada:** un día de trabajo + una sesión de
validación. Resultado: paridad funcional con Apple Silicon.

---

## Caminos a evitar

- **No comprar/rentar una Mac Intel solo para esto** — los runners de
  GitHub Actions son gratuitos y resuelven el mismo problema.
- **No invertir tiempo en osxcross** mientras la cross-compile desde esta
  Mac (A.1) o GitHub Actions (A.3) sean viables.
- **No empaquetar binarios linux/amd64** "porque sí pueden compilarse en
  Ubuntu" — no se ejecutan en macOS.
- **No prometer un universal binary** para la distro entera (multiplicar
  ×2 el tamaño de 12 GB = 24 GB no compresibles). Mejor dos tarballs
  separados, uno por arch.

---

## Mantenimiento futuro

Una vez funcionando el workflow de GitHub Actions:

- Cada `git tag v0.x.y` produce automáticamente dos `.app` (amd64 + arm64)
  como artefactos de la release.
- El script `build_amd64_distro.sh` corre en cualquier máquina (puede ser
  Ubuntu) y produce el tar.gz amd64 al final del flujo de release.
- Mismo para arm64. Idealmente un solo script parametrizado por arch.
- Periódicamente (cada 6-12 meses), refrescar las versiones de JDK,
  Python, ES descargando las últimas tarballs y revalidando con
  WordCount.

---

## Apéndice — Por qué Rosetta resuelve B.3 pero no A

Rosetta 2 hace **traducción binaria runtime** x86_64 → arm64. Eso significa:

- Un binario x86_64 ejecutado en Apple Silicon **corre con instrucciones
  arm64 traducidas**, pero **se identifica a sí mismo y a su entorno como
  x86_64**.
- Procesos x86_64 bajo Rosetta solo cargan librerías x86_64 (no pueden
  mezclar arches en un mismo proceso).
- Cualquier herramienta que ese proceso invoque (pip, gcc, etc.) **mira
  el arch del proceso padre** y opera como si fuera x86 nativo.

Por eso para **B.3** (ensamblar un Python amd64 + wheels en esta Mac)
Rosetta funciona: corremos un Python x86, pip descarga/compila wheels
x86, el resultado es 100% x86. Para **A** (compilar el launcher) Rosetta
también ayudaría si usáramos un Go x86 bajo Rosetta — pero es innecesario
porque el toolchain Go arm64 nativo ya sabe cross-compilar a amd64
(opción A.1, mucho más limpia).

---

## TL;DR

1. **Intentar `wails build -platform darwin/amd64` en esta Mac** (20 min).
   Si funciona, capa A resuelta.
2. **Montar GitHub Actions** con runners `macos-13` (Intel) y `macos-14`
   (arm64) para builds nativos del launcher en cada release (30 min).
3. **Ensamblar la distro amd64 aquí**, instalando wheels Python bajo
   Rosetta — sin necesidad de Mac Intel (medio día).
4. **Validar en una Mac Intel real** una sola vez (30 min presenciales).
5. **Olvidarse del Ubuntu** para esta tarea — no aporta a lo difícil.

Resultado: dos `.tar.gz` (`BDPV6-mac-arm64.tar.gz`, `BDPV6-mac-amd64.tar.gz`)
y un launcher que se compila solo en CI a partir de ahora.
