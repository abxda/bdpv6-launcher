#!/usr/bin/env bash
# BDP v7 (Windows): en Git Bash, '__NAME__' delega en __NAME__.cmd. El script Unix original arma un
# classpath con rutas /d/... que el java de Windows no entiende (ClassNotFoundException), y MSYS
# convertiría rutas HDFS como /test en rutas de Windows; por eso se apaga la conversión SOLO aquí
# (no globalmente: los kafka-*.sh sí la necesitan).
export MSYS_NO_PATHCONV=1
exec "$(dirname "$0")/__NAME__.cmd" "$@"
