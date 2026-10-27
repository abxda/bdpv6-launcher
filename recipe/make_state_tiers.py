"""BDP v7 · Recortes por estado de los datos nacionales (UP S3-S4).

Uso:  python make_state_tiers.py <carpeta_datos_nacionales> <carpeta_salida> [cve_ent ...]
Ej.:  python make_state_tiers.py D:/datos D:/datos/estados 01        (Aguascalientes)

Lee por lotes (pyarrow.dataset) para no cargar el archivo nacional completo en memoria y conserva los
metadatos GeoParquet ('geo') del esquema original.
"""
import os
import sys
import time

import pyarrow as pa
import pyarrow.compute as pc
import pyarrow.dataset as ds
import pyarrow.parquet as pq

SRC, DEST = sys.argv[1], sys.argv[2]
ESTADOS = sys.argv[3:] or ["01"]

# archivo nacional -> (columna con la clave de entidad, cómo comparar)
FUENTES = {
    "integrado_ne.parquet": ("CVEGEO", "prefijo"),      # CVEGEO empieza con la clave de 2 dígitos
    "censo_2020_nacional.parquet": ("ENTIDAD", "igual"),
    "denue_nacional.parquet": ("cve_ent", "igual"),
}


def filtro(col, modo, cve, tipo):
    campo = ds.field(col)
    if modo == "prefijo":
        return pc.starts_with(campo, cve)
    if pa.types.is_integer(tipo):
        return campo == int(cve)
    return campo == cve


for cve in ESTADOS:
    out_dir = os.path.join(DEST, cve)
    os.makedirs(out_dir, exist_ok=True)
    for archivo, (col, modo) in FUENTES.items():
        t0 = time.time()
        src = os.path.join(SRC, archivo)
        dataset = ds.dataset(src, format="parquet")
        schema = pq.read_schema(src)            # trae los metadatos 'geo'
        tipo = schema.field(col).type
        nombre = archivo.replace("_nacional", "").replace(".parquet", f"_{cve}.parquet")
        dest = os.path.join(out_dir, nombre)
        filas = 0
        with pq.ParquetWriter(dest, schema, compression="zstd") as w:
            for lote in dataset.to_batches(filter=filtro(col, modo, cve, tipo), batch_size=65536):
                if lote.num_rows:
                    w.write_batch(lote)
                    filas += lote.num_rows
        print(f"{cve} {archivo:32s} -> {nombre:30s} {filas:>10,} filas  "
              f"{os.path.getsize(dest) / 2**20:7.1f} MB  {time.time() - t0:5.1f}s")
