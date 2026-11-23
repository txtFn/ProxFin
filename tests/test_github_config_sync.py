
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[1] / "scripts" / "github_config_sync.py"

spec = importlib.util.spec_from_file_location("github_config_sync", SCRIPT)
sync = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sync)


class GitHubConfigSyncTests(unittest.TestCase):

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)

        self.config_dir = Path(self.temp.name) / "conf"
        self.config_dir.mkdir()

        self.files = {
            name: json.dumps({"test": name})
            for name in sync.FILES
        }

        for name, content in self.files.items():
            (self.config_dir / name).write_text(
                content, encoding="utf-8"
            )

    @patch.object(sync, "request_github")
    @patch.object(sync, "fetch_backup", return_value=(None, None))
    def test_first_backup_initialization(self, fetch, request):
        sync.backup(self.config_dir, initialize=True)

        self.assertTrue(
            (self.config_dir / ".github-backup-ready").exists()
        )
        request.assert_called_once()
        self.assertEqual(request.call_args.args[0], "PUT")

    @patch.object(sync, "request_github")
    @patch.object(sync, "fetch_backup")
    def test_initialization_cannot_overwrite(self, fetch, request):
        fetch.return_value = (
            {"version": 1, "files": self.files},
            "existing-sha",
        )

        with self.assertRaises(RuntimeError):
            sync.backup(self.config_dir, initialize=True)

        request.assert_not_called()

    @patch.object(sync, "request_github", side_effect=RuntimeError("Upload failed"))
    @patch.object(sync, "fetch_backup", return_value=(None, None))
    def test_failed_upload_does_not_enable_backup(self, fetch, request):
        with self.assertRaises(RuntimeError):
            sync.backup(self.config_dir, initialize=True)

        self.assertFalse(
            (self.config_dir / ".github-backup-ready").exists()
        )

    @patch.object(sync, "fetch_backup")
    def test_restore_configuration(self, fetch):
        fetch.return_value = (
            {"version": 1, "files": self.files},
            "backup-sha",
        )

        for name in sync.FILES:
            (self.config_dir / name).unlink()

        sync.restore(self.config_dir)

        for name, content in self.files.items():
            self.assertEqual(
                (self.config_dir / name).read_text(encoding="utf-8"),
                content,
            )

    @patch.object(sync, "fetch_backup")
    def test_invalid_backup_is_rejected(self, fetch):
        fetch.return_value = (
            {"version": 1, "files": {"settings.json": "{}"}},
            "backup-sha",
        )

        with self.assertRaises(ValueError):
            sync.restore(self.config_dir)

        self.assertFalse(
            list(self.config_dir.glob(".proxfin-restore-*"))
        )


if __name__ == "__main__":
    unittest.main()
