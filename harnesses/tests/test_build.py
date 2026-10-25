"""Exercise build/export modes without a daemon, credentials, or publication."""

import json
import os
import re
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class BuildTests(unittest.TestCase):
    def run_build(self, mode, platforms="linux/amd64,linux/arm64", *args):
        with tempfile.TemporaryDirectory() as tmp:
            docker = Path(tmp) / "docker"
            docker.write_text('#!/usr/bin/env python3\nimport json,sys\nprint(json.dumps(sys.argv[1:]))\n')
            docker.chmod(0o755)
            return subprocess.run(
                [str(ROOT / "harnesses/build-images.sh"), *args],
                env=dict(os.environ, PATH=tmp + os.pathsep + os.environ["PATH"],
                         MODE=mode, PLATFORMS=platforms),
                cwd=tmp, text=True, capture_output=True, check=False,
            )

    def test_validation_has_no_export(self):
        result = self.run_build("check")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), ["buildx", "bake", "--file", "harnesses/docker-bake.hcl"])

    def test_load_is_single_platform_and_explicit(self):
        self.assertEqual(self.run_build("load").returncode, 2)
        result = self.run_build("load", "linux/amd64", "codex")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)[-3:], ["--set", "*.output=type=docker", "codex"])

    def test_publication_is_explicit(self):
        result = self.run_build("push")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)[-2:], ["--set", "*.output=type=registry"])
        self.assertEqual(self.run_build("typo").returncode, 2)

    def test_release_check_is_read_only_and_rejects_mismatch(self):
        script = ROOT / "harnesses/release-images.sh"
        catalog = ROOT / "harnesses/images.yaml"
        before = catalog.read_bytes()
        current = re.search(rb"image:\s+[^\s]+:(v[^\s]+)", before).group(1).decode()
        for tag, expected in ((current, 0), ("v99.0.0", 1), ("latest", 2)):
            result = subprocess.run([str(script), tag, "--check"], capture_output=True, check=False)
            self.assertEqual(result.returncode, expected, result.stderr)
            self.assertEqual(catalog.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
