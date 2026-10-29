"""Oracle regressions: transport success alone must never qualify a route."""
import unittest
import summarize as s


def receipt(static=True, rendered=True, complete=True, url="fixture://fetch"):
    def route(action, facts, browser):
        return dict(action=action, actual_action=action, facts_found=facts,
                    capture_complete=complete, witnesses=[dict(id="proof")] if facts else [],
                    browser_uses=browser, fetch_attempts=1, latency_ms=100,
                    snapshot_ms=5, browser_http_events=2*browser)
    return dict(case=dict(url=url), input=dict(allowed_actions=[s.SERVE, s.RENDER, s.INSUFFICIENT]),
                routes=[route("baseline", rendered, 1), route(s.SERVE, static, 0), route(s.RENDER, rendered, 1)],
                observation=dict(status="observed", proposed_action=s.INSUFFICIENT, runtime_ms=20), model_wall_ms=30)


class OracleTests(unittest.TestCase):
    def test_static_and_required_render_have_distinct_optima(self):
        self.assertEqual(s.oracle(receipt()), [s.SERVE])
        self.assertEqual(s.oracle(receipt(static=False)), [s.RENDER])

    def test_degraded_capture_or_changed_public_source_does_not_prove_absence(self):
        self.assertEqual(s.oracle(receipt(False, False, False)), [])
        self.assertEqual(s.oracle(receipt(False, False, url="https://example.org/changed")), [])
        self.assertEqual(s.oracle(receipt(False, False, url="fixture://unavailable")), [s.INSUFFICIENT])

    def test_false_insufficiency_has_cost_and_is_not_success(self):
        result = s.summary([receipt()], "julia", .5)
        self.assertEqual(result["counts"]["false_insufficient"], 1)
        self.assertEqual(result["counts"]["verified_fact_delivery"], 0)
        self.assertEqual(result["median_estimated_total_ms"], 135)
        self.assertEqual(result["measured_native_get_attempts"], 1)

    def test_unavailable_initial_fetch_has_unknown_oracle(self):
        item = receipt(False, False)
        item["input"]["allowed_actions"] = None
        self.assertEqual(s.oracle(item), [])

    def test_dense_missing_score_abstains_instead_of_zero(self):
        self.assertEqual(s.dense_choice(receipt(), .5), "")
        item = receipt()
        item["reference_similarity"] = -.3
        self.assertEqual(s.dense_choice(item, -.5), s.SERVE)


if __name__ == "__main__":
    unittest.main()
