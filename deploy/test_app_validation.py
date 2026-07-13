"""Unit tests for the request validators in app.py.

app.py imports the Modal SDK at module load, so we stub `modal` with a
MagicMock before importing it. The validators under test are pure functions —
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


class ValidateRef(unittest.TestCase):
    def test_accepts_real_refs(self):
        for good in ["main", "release/v0.5.0", "feature/foo-bar", "v1.2.3", "0123abcd0123abcd0123abcd0123abcd0123abcd"]:
            with self.subTest(good=good):
                self.assertEqual(app._validate_ref(good), good)

    def test_defaults_field_name_in_message(self):
        with self.assertRaises(ValueError) as ctx:
            app._validate_ref("bad ref", field="branch")
        self.assertIn("branch", str(ctx.exception))

    def test_rejects_argument_injection(self):
        # A leading dash would be parsed as a git option (argument injection)
        # even after shlex.quote makes it shell-safe.
        for bad in ["--upload-pack=touch /tmp/pwned", "-x", "--exec=evil"]:
            with self.subTest(bad=bad):
                with self.assertRaises(ValueError):
                    app._validate_ref(bad)

    def test_rejects_shell_and_git_metacharacters(self):
        for bad in [
            "",
            "   ",
            "main; rm -rf ~",
            "main && curl evil",
            "$(id)",
            "a b",
            "feature/..",
            "refs/../../etc",
            "trailing/",
            "locked.lock",
            "back\\slash",
            "tilde~1",
            "colon:ref",
        ]:
            with self.subTest(bad=bad):
                with self.assertRaises(ValueError):
                    app._validate_ref(bad)


if __name__ == "__main__":
    unittest.main()
