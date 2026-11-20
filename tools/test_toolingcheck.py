"""Negative assertions for external tooling adoption metadata."""
from pathlib import Path
import tempfile
import unittest

from toolingcheck import check, NAMES


class ToolingCheckTest(unittest.TestCase):
    def test_all_eleven_candidates_are_catalogued(self):
        self.assertEqual(len(NAMES), 11)
        self.assertEqual(len(set(NAMES)), 11)

    def test_empty_repository_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            errors = check(Path(directory))
            self.assertTrue(any("missing adoption artifact" in e for e in errors))


if __name__ == "__main__":
    unittest.main()
