"""Negative/positive tests for the Mamony skills metadata validator."""
from pathlib import Path
import tempfile
import unittest

from skillcheck import validate_registry, validate_skill


class SkillCheckTest(unittest.TestCase):
    def test_valid_skill(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "mamony-test" / "SKILL.md"
            path.parent.mkdir()
            path.write_text(
                "---\nname: mamony-test\ndescription: Use for example checks\n---\n"
                "# Example\nFollow the scope, verify acceptance criteria and record evidence.\n",
                encoding="utf-8",
            )
            self.assertEqual(validate_skill(path), [])

    def test_missing_skill_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            errors = validate_registry(Path(directory))
            self.assertTrue(any("missing skill:" in error for error in errors))

    def test_mismatched_name_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "wrong-name" / "SKILL.md"
            path.parent.mkdir()
            path.write_text(
                "---\nname: other-name\ndescription: A test description\n---\n"
                "# Example\nA sufficiently long, fully written instruction for testing.\n",
                encoding="utf-8",
            )
            self.assertTrue(any("name mismatch" in e for e in validate_skill(path)))


if __name__ == "__main__":
    unittest.main()
