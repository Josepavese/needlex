"""Corpus/observer regressions that protect denominators and source completeness."""
import importlib.util
import json
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
base = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(base / "internal"))
spec = importlib.util.spec_from_file_location("live", base / "decision_observer/live.py")
live = importlib.util.module_from_spec(spec)
spec.loader.exec_module(live)


class PopularCorpusTests(unittest.TestCase):
    def test_exact_distinct_urls_and_preserved_legacy_occurrences(self):
        manifest = base / "corpora/popular-web-v1.json"
        cases = live.cases([manifest])
        self.assertEqual(len(cases), 1000)
        self.assertEqual(len({c["url"] for c in cases}), 1000)
        old = [c for c in cases if c["cohort"] == "historical"]
        self.assertEqual(len(old), 99)
        self.assertEqual(sum(len(c["origins"]) for c in old), 100)
        self.assertEqual(len({c["family"] for c in cases if c["cohort"] == "ranked"}), 901)

    def test_source_language_is_not_guessed_from_question_or_domain(self):
        cases = live.cases([base / "corpora/popular-web-v1.json"])
        ranked = [c for c in cases if c["cohort"] == "ranked"]
        self.assertEqual({c["source_language"] for c in ranked}, {"und"})
        self.assertEqual(len({c["language"] for c in ranked}), 6)

    def test_partial_source_and_failed_render_do_not_become_complete(self):
        stages = [dict(stage="acquire", metadata=dict(partial_body="true"))]
        self.assertFalse(live.capture_complete(stages))
        stages.append(dict(stage="render", metadata=dict(rendered="false")))
        self.assertFalse(live.capture_complete(stages))
        stages.append(dict(stage="render", metadata=dict(rendered="true", render_path="cdp", partial="false", network_truncated="false", network_body_missing="0")))
        self.assertTrue(live.capture_complete(stages))
        stages[-1]["metadata"]["network_truncated"] = "true"
        self.assertFalse(live.capture_complete(stages))

    def test_unknown_metadata_is_not_zero_or_complete(self):
        self.assertIsNone(live.capture_complete([dict(stage="acquire", metadata={})]))
        self.assertIsNone(live.capture_complete([dict(stage="agent_readable", metadata=dict(selected="true"))]))

    def test_error_taxonomy_does_not_read_status_from_diagnostic_hash(self):
        sys.path.insert(0, str(base / "decision_observer"))
        from summarize import error_class
        self.assertEqual(error_class({"message":"unexpected status code 503; diagnostic_id:log_403abcd"}),"unavailable_upstream")
        self.assertEqual(error_class({"message":"context deadline exceeded"}),"upstream_timeout")
        self.assertEqual(error_class({"kind":"observer_timeout"}),"benchmark_timeout")


if __name__ == "__main__":
    unittest.main()
