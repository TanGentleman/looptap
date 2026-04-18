"""Modal deployment for looptap. PR 2 wires in the FastAPI endpoint."""
import modal

APP_NAME = "looptap"
FUNCTION_NAME = "web"
SECRET_NAME = "looptap-secrets"

app = modal.App(APP_NAME)

# PR 2: image, volume mount, @app.function(...) serving FastAPI.
