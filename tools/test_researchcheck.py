"""Regression tests for the v6 research/benchmark metadata checker."""
from pathlib import Path
import tempfile
import unittest

from researchcheck import section_ids, eval_case_ids, registry_urls, check


class ResearchCheckTest(unittest.TestCase):
    def test_sections(self):
        self.assertEqual(section_ids("## 18. first\n## 19. second\n## 24. last\n"), [18, 19, 24])

    def test_cases(self):
        self.assertEqual(eval_case_ids("| E01 | task | evidence |\n| E02 | task | evidence |\n"), ["E01", "E02"])

    def test_registry(self):
        line = "| Go | official | now | use | [Original source](https://go.dev/doc/) |\n"
        self.assertEqual(registry_urls(line), ["https://go.dev/doc/"])

    def test_missing_documents_reported(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertTrue(any("missing v6 document" in err for err in check(Path(tmp))))


if __name__ == "__main__":
    unittest.main()
