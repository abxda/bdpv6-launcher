"""BDP v7 · publica en Hugging Face (dataset abxda/bdp-lab, SOLO bajo v7/) un portable construido.

    python recipe/publish.py --platform win-64 --tarball dist/bdp-v7-portable-windows-amd64.tar.gz \
        --launcher build/bin/bdpv6-launcher.exe --version 7.0.0-dev1 --out-json dist/publish-win-64.json

- Sube el tarball a v7/portable/ y verifica que el sha256 que reporta HF coincida con el local.
- Sube el ejecutable del launcher a v7/launchers/ y DESPUÉS su descriptor de auto-actualización
  (el orden importa: un descriptor que apunta a un binario aún no subido rompe la auto-actualización).
- Escribe un JSON con la entrada de manifiesto; write_manifest.py los junta (un solo escritor).
El token se toma de HF_TOKEN (secret de CI) o de --token-file; nunca se imprime.
"""
import argparse
import hashlib
import json
import os
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

from huggingface_hub import HfApi  # noqa: E402

REPO = "abxda/bdp-lab"
BASE_URL = f"https://huggingface.co/datasets/{REPO}/resolve/main"
PLAT = {  # plataforma -> (clave de manifiesto, GOOS-GOARCH del launcher, entrada .launch)
    "win-64": ("windows-amd64-portable", "windows-amd64", "bdpv6-launcher.exe"),
    "linux-64": ("linux-amd64-portable", "linux-amd64", "bdpv6-launcher"),
    "osx-arm64": ("darwin-arm64-portable", "darwin-arm64", "BDPV6 Launcher.app"),
}


def launcher_binary(launcher: Path) -> Path:
    """El ejecutable que se auto-actualiza: en macOS es el Mach-O dentro de <app>/Contents/MacOS/."""
    if launcher.is_dir():
        bins = [p for p in (launcher / "Contents" / "MacOS").iterdir() if p.is_file()]
        if len(bins) != 1:
            raise SystemExit(f"esperaba un solo ejecutable en {launcher}/Contents/MacOS, hay {len(bins)}")
        return bins[0]
    return launcher


def sha256(path: Path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def upload(api, local: Path, dest: str, msg: str):
    api.upload_file(path_or_fileobj=str(local), path_in_repo=dest, repo_id=REPO, repo_type="dataset", commit_message=msg)
    info = api.get_paths_info(REPO, [dest], repo_type="dataset", expand=True)[0]
    remote = getattr(getattr(info, "lfs", None), "sha256", None)
    local_sha = sha256(local)
    if remote and remote != local_sha:
        raise SystemExit(f"HF reporta otro sha256 para {dest}: {remote} != {local_sha}")
    print(f"subido {dest} ({local.stat().st_size / 2**20:,.1f} MB) sha256 {local_sha} "
          f"{'(verificado contra HF)' if remote else '(archivo pequeño: sin LFS)'}")
    return local_sha


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--platform", required=True, choices=list(PLAT))
    ap.add_argument("--tarball", required=True, type=Path)
    ap.add_argument("--launcher", required=True, type=Path)
    ap.add_argument("--version", required=True)
    ap.add_argument("--out-json", required=True, type=Path)
    ap.add_argument("--token-file", type=Path)
    a = ap.parse_args()
    token = os.environ.get("HF_TOKEN") or (a.token_file.read_text(encoding="utf-8").strip() if a.token_file else None)
    if not token:
        raise SystemExit("falta el token: define HF_TOKEN o --token-file")
    api = HfApi(token=token)
    key, goos_arch, launch = PLAT[a.platform]

    tar_dest = f"v7/portable/{a.tarball.name}"
    tar_sha = upload(api, a.tarball, tar_dest, f"v7 {a.version}: portable {a.platform}")

    exe = launcher_binary(a.launcher)
    exe_name = f"bdpv6-launcher-{goos_arch}" + (".exe" if a.platform == "win-64" else "")
    exe_sha = upload(api, exe, f"v7/launchers/{exe_name}", f"v7 {a.version}: launcher {goos_arch}")
    desc = a.out_json.with_name(f"bdpv6-launcher-latest-{goos_arch}.json")
    desc.write_text(json.dumps({"version": a.version, "sha256": exe_sha,
                                "url": f"{BASE_URL}/v7/launchers/{exe_name}"}, indent=2), encoding="utf-8")
    upload(api, desc, f"v7/launchers/{desc.name}", f"v7 {a.version}: descriptor {goos_arch}")

    entry = {"key": key, "file": a.tarball.name, "sha256": tar_sha, "url": f"{BASE_URL}/{tar_dest}",
             "launch": launch, "size": f"~{a.tarball.stat().st_size / 2**30:.1f} GB", "version": a.version}
    a.out_json.write_text(json.dumps(entry, indent=2, ensure_ascii=False), encoding="utf-8")
    print(f"entrada de manifiesto -> {a.out_json}")


if __name__ == "__main__":
    main()
