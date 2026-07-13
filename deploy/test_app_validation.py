"""Unit tests for the request validators / scrubbers in app.py.

app.py imports the Modal SDK at module load, so we stub `modal` with a
MagicMock before importing it. The helpers under test are pure functions —
no Modal, FastAPI, or network needed — so this runs anywhere Python does:

    python3 deploy/test_app_validation.py
    python3 -m unittest deploy.test_app_validation
"""
import os
import sys
import unittest
from unittest.mock import MagicMock

# Modal isn't a test dependency; stub it so `import app` succeeds. The module
# only touches modal for App/Volume/Image wiring at import time, all of which
# a MagicMock happily absorbs.
sys.modules.setdefault("modal", MagicMock())
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import app  # noqa: E402


class ValidateRepoSlug(unittest.TestCase):
    def test_accepts_owner_name(self):
        self.assertEqual(app._validate_repo_slug("TanGentleman/looptap"), "TanGentleman/looptap")
        self.assertEqual(app._validate_repo_slug(" owner/name "), "owner/name")

    def test_rejects_traversal_and_junk(self):
        for bad in ["../etc", "owner/..", "a/b/c", "owner", "-owner/name", "owner/name; rm -rf /"]:
            with self.subTest(bad=bad):
                with self.assertRaises(ValueError):
                    app._validate_repo_slug(bad)


class RedactSecrets(unittest.TestCase):
    def test_replaces_known_secret_values(self):
        key = "AIzaSyDummyProviderKeyValue0001"
        blob = f"<html>echo {key} and again {key}</html>"
        self.assertEqual(
            app._redact_secrets(blob, key),
            "<html>echo [REDACTED] and again [REDACTED]</html>",
        )

    def test_ignores_empty_secrets_and_empty_text(self):
        self.assertEqual(app._redact_secrets("", "secret"), "")
        self.assertEqual(app._redact_secrets("plain", ""), "plain")
        self.assertEqual(app._redact_secrets("plain"), "plain")

    def test_redacts_multiple_distinct_secrets(self):
        a, b = "secret-aaa", "secret-bbb"
        self.assertEqual(
            app._redact_secrets(f"{a}|{b}", a, b),
            "[REDACTED]|[REDACTED]",
        )


if __name__ == "__main__":
    unittest.main()
