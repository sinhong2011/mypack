#!/usr/bin/env python3
"""Fixture checks for the relative-link scanner. No network."""

from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from doclinks import changed_markdown, missing_links  # noqa: E402


class DoclinksTest(unittest.TestCase):
    def test_changed_markdown_rename_and_skip(self) -> None:
        diff = "\n".join(
            [
                "M\tREADME.md",
                "A\tREADME.es.md",
                "M\tdocs/screenshots/real-library.png",
                "R100\told.md\tdocs/new.md",
                "D\tinternal/cli/cli.go",
            ]
        )
        self.assertEqual(
            changed_markdown(diff),
            ["README.md", "README.es.md", "docs/new.md"],
        )

    def test_relative_link_and_external_skip(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "docs").mkdir()
            (root / "docs" / "pic.png").write_bytes(b"png")
            readme = root / "README.md"
            readme.write_text(
                "\n".join(
                    [
                        "[guide](docs/guide.md)",
                        "![pic](docs/pic.png)",
                        "[web](https://example.com/gone)",
                        "[mail](mailto:a@b.c)",
                        "[section](#top)",
                    ]
                ),
                encoding="utf-8",
            )
            (root / "docs" / "guide.md").write_text("ok\n", encoding="utf-8")
            self.assertEqual(missing_links(root, ["README.md"]), [])
            readme.write_text("[gone](docs/missing.md)\n", encoding="utf-8")
            problems = missing_links(root, ["README.md"])
            self.assertEqual(len(problems), 1)
            self.assertIn("missing docs/missing.md", problems[0])


if __name__ == "__main__":
    unittest.main()
