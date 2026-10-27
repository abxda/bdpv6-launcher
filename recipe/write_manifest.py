"""BDP v7 · compone y sube v7/manifest-v7.txt a partir de los JSON de publish.py.

Un SOLO escritor (el job final del CI) evita la carrera entre plataformas. Las entradas de plataformas
que no vienen en esta corrida se conservan del manifiesto publicado (no se pisan).

    python recipe/write_manifest.py dist/publish-*.json [--dry-run]
"""
import argparse
import json
import os
import sys
import urllib.request
from pathlib import Path

try:  # red corporativa con inspección TLS
    import truststore
except ImportError:
    try:
        from pip._vendor import truststore
    except ImportError:
        truststore = None
if truststore:
    truststore.inject_into_ssl()

REPO = "abxda/bdp-lab"
DEST = "v7/manifest-v7.txt"
HEADER = """# ============================================================================
# Big Data Lab v7 "Convergente" — manifiesto (Dr. Abel Coronado)
# ============================================================================
# Una sola receta (recipe/stack.toml) construida por CI en los 3 SO; perfiles por
# curso en el launcher (UP = + Elasticsearch, UNIR = + Kafka).
# El manifiesto v6 (manifest.txt) NO se toca: v6 sigue siendo el Plan B.
#
# Esquema por entrada (clave base = <os>-<arch>-portable):
#   <base>.file / .sha256 / .url / .launch / .size / .version
# Generado por recipe/write_manifest.py — no editar a mano.
# ============================================================================
"""


def parse(text):
    out = {}
    for line in text.splitlines():
        if line.strip() and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            out[k.strip()] = v.strip()
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("entries", nargs="+", type=Path)
    ap.add_argument("--dry-run", action="store_true")
    a = ap.parse_args()
    try:
        current = parse(urllib.request.urlopen(f"https://huggingface.co/datasets/{REPO}/resolve/main/{DEST}", timeout=60).read().decode())
    except Exception:  # noqa: BLE001 - primera publicación
        current = {}
    for e in (json.loads(p.read_text(encoding="utf-8")) for p in a.entries):
        for field in ("file", "sha256", "url", "launch", "size", "version"):
            current[f"{e['key']}.{field}"] = e[field]
    bases = sorted({k.rsplit(".", 1)[0] for k in current})
    body = "\n".join(f"\n# --- {b} ---\n" + "\n".join(f"{k}={current[k]}" for k in sorted(current) if k.rsplit(".", 1)[0] == b)
                     for b in bases)
    text = HEADER + body + "\n"
    if a.dry_run:
        sys.stdout.write(text)
        return
    from huggingface_hub import HfApi
    tmp = Path("manifest-v7.txt")
    tmp.write_text(text, encoding="utf-8")
    token = os.environ["HF_TOKEN"].strip().lstrip("﻿").strip()  # ver publish.py
    HfApi(token=token).upload_file(path_or_fileobj=str(tmp), path_in_repo=DEST, repo_id=REPO,
                                                    repo_type="dataset", commit_message="v7: manifiesto")
    print(f"publicado {DEST} con {len(bases)} entradas")


if __name__ == "__main__":
    main()
