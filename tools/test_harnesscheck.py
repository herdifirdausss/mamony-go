"""Regression checks for v7 integration metadata."""
from pathlib import Path
import tempfile
import unittest

from harnesscheck import DIGEST, REQUIRED_FILES, check


class HarnessCheckTest(unittest.TestCase):
    def test_source_count_and_digest(self):
        self.assertEqual(len(DIGEST), 64)
        self.assertEqual(len(REQUIRED_FILES), 8)

    def test_missing_docs_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            errors = check(Path(tmp))
            self.assertTrue(any("missing v7 integration file:" in e for e in errors))


if __name__ == "__main__":
    unittest.main()
