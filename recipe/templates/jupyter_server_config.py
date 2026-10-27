# BDP v7 · JupyterLab del lab: sin token (solo escucha en 127.0.0.1) y terminal = bash en todos los SO.
# El launcher apunta JUPYTER_CONFIG_DIR a esta carpeta: no se mezcla con el ~/.jupyter del alumno.
import os

c = get_config()  # noqa: F821 - lo inyecta Jupyter
c.ServerApp.ip = "127.0.0.1"
c.ServerApp.open_browser = False
c.IdentityProvider.token = ""
c.ServerApp.root_dir = os.environ.get("BDP_NOTEBOOKS", os.getcwd())

# Windows: la terminal de JupyterLab abre el Git Bash del lab (no PowerShell), para que los comandos
# del deck (hdfs dfs, kafka-topics.sh, curl -X GET) sean idénticos a Mac/Linux.
bash = os.environ.get("BDP_BASH")
if os.name == "nt" and bash and os.path.exists(bash):
    c.ServerApp.terminado_settings = {"shell_command": [bash, "--login", "-i"]}
