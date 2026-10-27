# Laboratorio BDP v7 en la nube

La versión en la nube (GitHub Codespaces) vive en su propio repositorio, que es la fuente única:
**https://github.com/abxda/bdp-lab-nube** (`bdp_start.py`, `.devcontainer/` y los cuadernos de la UP).

Los cuadernos de ese repositorio se generan con `recipe/patch_up_notebooks.py`:

    python recipe/patch_up_notebooks.py <Curso_BDP> <bdp-lab-nube>/notebooks
