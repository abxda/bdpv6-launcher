# BDP v7 · Laboratorio en la nube

Es el mismo laboratorio de la versión portable, pero corre en la nube. No necesitas instalar nada en tu computadora.

## Qué ya está pasando

- Al crear el codespace se descargó el laboratorio (~1.7 GB). Antes de usarlo se verificó que el archivo fuera el correcto.
- Cada vez que el codespace arranca, se encienden HDFS, JupyterLab y el servicio de tu curso.

## Comandos (en la Terminal, abajo)

| Para | Comando |
|---|---|
| Ver qué está encendido y las direcciones | `python3 cloud/bdp_start.py status` |
| Curso **UP** (HDFS + Jupyter + Elasticsearch) | `python3 cloud/bdp_start.py up` |
| Curso **UNIR** (HDFS + Jupyter + Kafka) | `python3 cloud/bdp_start.py unir` |
| Verificar que todo funciona (evidencia para tu actividad) | `python3 cloud/bdp_start.py check` |
| Apagar todo limpio | `python3 cloud/bdp_start.py stop` |

En la terminal ya funcionan `hdfs dfs -ls /`, `kafka-topics.sh`, `spark-submit`, `python` y `curl -X GET "http://localhost:9200/_cat/indices?v"`. Son los mismos comandos que en Windows, Mac y Linux.

## Abrir JupyterLab

En la pestaña **Puertos**, abre el puerto **8888** ("JupyterLab"). Entra directo, sin token.

Otra opción es abrir tus cuadernos en VS Code y elegir el kernel **"BDP v7 (Spark 4.1 · Python 3.12)"**.

## Cuida tus horas gratis

- Codespaces da **120 horas-núcleo al mes**, o **180 con GitHub Student Pack**. Esta máquina tiene 4 núcleos, así que eso equivale a **unas 30–45 horas**.
- Cuando termines, cierra la pestaña. El codespace se detiene solo a los 30 minutos sin uso y tu trabajo se conserva.

Si algo falla, copia la salida de `python3 cloud/bdp_start.py check` y envíala por Blackboard.

— Dr. Abel Coronado
