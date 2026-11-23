"""Offline tests of the assistant evaluation's scoring (scripts/assistant_eval.py).

    python3 -m unittest scripts/test_assistant_eval.py

Nothing here talks to a backend: the client used is a fake that records requests.
"""
import contextlib, copy, io, os, sys, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import assistant_eval as ev  # noqa: E402


def case(**over):
    c = {"id": "t", "stages": ["clips"], "mode": "manual", "messages": ["make the videos"],
         "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["expensive"], "language": "en"}}
    expect = over.pop("expect", {})
    c.update(over)
    c["expect"] = {**c["expect"], **expect}
    return c


CLIPS = {"current": "clips", "next": {"action": "render_scenes", "scene_ids": ["s1", "s2"]}}
GOOD = "I'll make both scene videos now. The button shows the seconds and the price."


def intent(action="render_scenes", reply=GOOD, **decision):
    return {"action": action, "reply": reply, "decision": {"mode": "act", "assumptions": [], **decision}}


class FakeClient:
    """Records requests through the same guard as the real client."""

    def __init__(self, log):
        self.log, self.sent = copy.deepcopy(log), []

    def request(self, method, path, body=None):
        ev.guard(method, path)
        self.sent.append((method, path, body))

    def decisions(self):
        return copy.deepcopy(self.log)

    def put_decision(self, key, value):
        self.request("PUT", "/decisions/" + key, {"value": value})
        self.log[key] = {"key": key, "value": value, "source": "user"}

    def delete_decision(self, key):
        self.request("DELETE", "/decisions/" + key)
        self.log.pop(key, None)


class ScoringTest(unittest.TestCase):
    def test_a_good_reply_scores_full(self):
        r = ev.score_case(case(), intent(), CLIPS)
        self.assertEqual(r["score"], 1.0, r["problems"])
        self.assertFalse(r["asked"])
        self.assertEqual(r["criteria"]["no_spam"], None)  # only scored in Full auto

    def test_next_resolves_from_the_stage_map(self):
        self.assertEqual(ev.allowed_actions({"actions": ["NEXT"]}, CLIPS), {"render_scenes", "finish_video"})
        self.assertEqual(ev.allowed_actions({"actions": ["NEXT", "unknown"]}, {"next": {"action": "open_voices"}}), {"open_editor", "unknown"})
        self.assertEqual(ev.allowed_actions({"actions": ["NEXT"]}, {"current": "export", "next": None}), {"unknown", "open_editor"})
        r = ev.score_case(case(), intent("generate_shots"), CLIPS)
        self.assertFalse(r["criteria"]["action"])
        self.assertLess(r["score"], 1.0)

    def test_panel_is_checked(self):
        c = case(expect={"actions": ["open_editor"], "panel": "export", "decision": "act_or_inform"})
        self.assertTrue(ev.score_case(c, {**intent("open_editor"), "editor_panel": "export"}, CLIPS)["criteria"]["action"])
        self.assertFalse(ev.score_case(c, {**intent("open_editor"), "editor_panel": "music"}, CLIPS)["criteria"]["action"])

    def test_full_auto_questions(self):
        full = case(mode="full", expect={"decision": "not_ask", "ask_reasons": ["missing_input", "confirm_plan"]})
        ask = dict(mode="ask", question="Which scenes?", options=[{"label": "All"}])
        bad = ev.score_case(full, intent(**ask, reason="options"), CLIPS)
        self.assertFalse(bad["criteria"]["decision"])
        self.assertTrue(bad["unnecessary_question"])
        ok = ev.score_case(full, intent(**ask, reason="missing_input"), CLIPS)
        self.assertTrue(ok["criteria"]["decision"], ok["problems"])
        self.assertFalse(ok["unnecessary_question"])
        # Expensive is allowed in manual, never in Full auto.
        self.assertTrue(ev.score_case(case(), intent(**ask, reason="expensive"), CLIPS)["criteria"]["decision"])
        self.assertFalse(ev.score_case(case(mode="full"), intent(**ask, reason="expensive"), CLIPS)["criteria"]["decision"])
        # A question the server overruled still counts against the assistant.
        over = ev.score_case(full, intent(overruled="full_auto", assumptions=["Went with All"]), CLIPS)
        self.assertFalse(over["criteria"]["decision"])
        self.assertTrue(over["asked"] and over["unnecessary_question"])

    def test_a_needed_question(self):
        c = case(stages=["brief"], expect={"actions": ["unknown"], "decision": "ask", "ask_reasons": ["missing_input"]})
        brief = {"current": "brief", "next": {"action": "generate_script"}}
        self.assertTrue(ev.score_case(c, intent("unknown", mode="ask", question="About what?", reason="missing_input"), brief)["criteria"]["decision"])
        self.assertFalse(ev.score_case(c, intent("unknown", mode="inform"), brief)["criteria"]["decision"])
        self.assertFalse(ev.score_case(c, intent("unknown", mode="ask", question="Format?", reason="options"), brief)["criteria"]["decision"])

    def test_act_and_inform_expectations(self):
        self.assertFalse(ev.score_case(case(expect={"decision": "act"}), intent(mode="inform"), CLIPS)["criteria"]["decision"])
        self.assertTrue(ev.score_case(case(expect={"decision": "act_or_inform"}), intent(mode="inform"), CLIPS)["criteria"]["decision"])
        self.assertFalse(ev.score_case(case(expect={"decision": "inform", "actions": ["unknown"]}), intent("unknown", mode="act"), CLIPS)["criteria"]["decision"])

    def test_settled_questions(self):
        c = case(settled=["music", "aspect_ratio"], seed={"music": "piano"})
        settled = {"music", "aspect_ratio"}
        by_key = ev.score_case(c, intent(mode="ask", question="Pick one", key="music", reason="expensive"), CLIPS, settled)
        self.assertFalse(by_key["criteria"]["no_repeat"])
        by_topic = ev.score_case(c, intent(mode="ask", question="Should it be vertical or wide?", reason="expensive"), CLIPS, settled)
        self.assertFalse(by_topic["criteria"]["no_repeat"])
        self.assertIn("asked again about aspect_ratio", by_topic["problems"])
        overruled = ev.score_case(c, intent(overruled="already_decided", key="music", assumptions=["Kept your earlier choice: music = piano"]), CLIPS, settled)
        self.assertFalse(overruled["criteria"]["no_repeat"])
        unrelated = ev.score_case(c, intent(mode="ask", question="Make both now? It costs credits.", reason="expensive"), CLIPS, settled)
        self.assertTrue(unrelated["criteria"]["no_repeat"], unrelated["problems"])
        # A key the log does not hold is not settled, so it is not scored.
        self.assertIsNone(ev.score_case(c, intent(), CLIPS, set())["criteria"]["no_repeat"])

    def test_assumptions(self):
        c = case(expect={"assumptions": True})
        self.assertFalse(ev.score_case(c, intent(), CLIPS)["criteria"]["assumptions"])
        self.assertTrue(ev.score_case(c, intent(assumptions=["9:16 for TikTok"]), CLIPS)["criteria"]["assumptions"])
        self.assertIsNone(ev.score_case(c, intent(mode="ask", question="Which?", reason="expensive"), CLIPS)["criteria"]["assumptions"])

    def test_reply_checks(self):
        self.assertEqual(ev.reply_problems(GOOD, "en"), [])
        self.assertIn("reply not in es", ev.reply_problems(GOOD, "es"))
        self.assertEqual(ev.reply_problems("Listo, ¿quieres que haga el video ahora?", "es"), [])
        self.assertEqual(ev.reply_problems("好的，我现在制作视频。", "zh"), [])
        self.assertIn("reply not in zh", ev.reply_problems(GOOD, "zh"))
        self.assertIn("reply not in en", ev.reply_problems("好的", "en"))
        self.assertTrue(any(p.startswith("reply 121 words") for p in ev.reply_problems("word " * 121, "en")))
        self.assertIn("jargon: pipeline", ev.reply_problems("The pipeline is ready.", "en"))
        self.assertIn("jargon: render_scenes", ev.reply_problems("I set render_scenes for you.", "en"))
        self.assertIn("jargon: decided_so_far", ev.reply_problems("Your decided_so_far says 9:16.", "en"))
        self.assertIn("empty reply", ev.reply_problems(" ", "en"))

    def test_full_auto_button_spam(self):
        full = case(mode="full", expect={"decision": "act"})
        self.assertTrue(ev.score_case(full, intent(), CLIPS)["criteria"]["no_spam"])
        self.assertFalse(ev.score_case(full, intent("render_scene"), CLIPS)["criteria"]["no_spam"])
        one = {"current": "clips", "next": {"action": "render_scenes", "scene_ids": ["s1"]}}
        self.assertTrue(ev.score_case(full, intent("render_scene"), one)["criteria"]["no_spam"])
        chars = {"current": "characters", "next": {"action": "generate_character_references", "character_ids": ["a", "b"]}}
        self.assertFalse(ev.score_case(full, intent("generate_character_reference"), chars)["criteria"]["no_spam"])
        self.assertFalse(ev.score_case(full, intent(reply="Press the button for each scene one by one."), CLIPS)["criteria"]["no_spam"])

    def test_score_is_passed_over_applicable(self):
        r = ev.score_case(case(), intent("generate_shots", reply="The pipeline is ready."), CLIPS)
        # action and reply fail; decision passes; no_repeat, assumptions, no_spam do not apply.
        self.assertEqual(r["score"], round(1 / 3, 4))


class SummaryTest(unittest.TestCase):
    def rows(self):
        c = case()
        a = ev.result_row(c, CLIPS, intent(), ev.score_case(c, intent(), CLIPS), [])
        asked = intent(mode="ask", question="Which?", reason="options")
        b = ev.result_row({**c, "id": "u"}, CLIPS, asked, ev.score_case(c, asked, CLIPS), [])
        q = case(id="q", expect={"decision": "ask", "ask_reasons": ["missing_input"], "actions": ["unknown"]})
        qi = intent("unknown", mode="ask", question="About what?", reason="missing_input")
        d = ev.result_row(q, {"current": "brief"}, qi, ev.score_case(q, qi, {"current": "brief"}), [])
        return [a, b, d, {"id": "s", "skipped": True}]

    def test_summary(self):
        s = ev.summarize(self.rows())
        self.assertEqual((s["cases"], s["passed"], s["skipped"]), (3, 2, 1))
        # Two rows expected no question; one asked anyway.
        self.assertEqual(s["unnecessary_question_rate"], 0.5)
        self.assertEqual(s["per_stage"]["brief"], 100.0)
        # The unneeded question fails one of its three applicable criteria (decision).
        self.assertEqual(self.rows()[1]["score"], round(2 / 3, 4))
        self.assertEqual(s["per_stage"]["clips"], round(100 * (1 + 2 / 3) / 2, 1))
        self.assertEqual(s["per_criterion"]["decision"], round(100 * 2 / 3, 1))
        self.assertEqual(s["overall"], round(100 * (1 + 2 / 3 + 1) / 3, 1))

    def test_a_failed_request_scores_zero(self):
        r = ev.result_row(case(), CLIPS, {}, None, ["HTTP 500: boom"])
        self.assertEqual((r["score"], r["ok"]), (0.0, False))

    def test_compare(self):
        rows = self.rows()
        previous = {"rows": rows, "summary": ev.summarize(rows)}
        self.assertEqual(ev.compare(previous, previous), [])
        worse = copy.deepcopy(rows)
        worse[0].update(ok=False, score=0.5, problems=["asked again about music"])
        report = {"rows": worse, "summary": ev.summarize(worse)}
        lines = ev.compare(report, previous)
        self.assertTrue(lines[0].startswith("REGRESSION t:"), lines)
        self.assertTrue(any(line.startswith("overall") for line in lines), lines)
        # The same case on a project at another stage is not compared.
        moved = copy.deepcopy(worse)
        for r in moved:
            r["stage"] = "edit"
        self.assertEqual([l for l in ev.compare({"rows": moved, "summary": previous["summary"]}, previous) if l.startswith(("REGRESSION", "worse"))], [])


class SafetyTest(unittest.TestCase):
    def test_only_chat_and_the_decision_log_are_sent(self):
        for method, path in [("GET", "/stages"), ("GET", "/decisions"), ("POST", "/chat"), ("PUT", "/decisions/music"), ("DELETE", "/decisions/voice%3Ac1")]:
            ev.guard(method, path)
        for method, path in [("POST", "/videos/generate"), ("POST", "/render-jobs"), ("POST", "/scenes/s1/render"), ("POST", "/autopilot/start"),
                             ("POST", "/characters/sheets"), ("DELETE", "/scenes/s1"), ("PUT", "/settings/app"), ("POST", "/decisions/music"),
                             ("GET", "/settings/providers"), ("POST", "/chat/../render-jobs"), ("PUT", "/decisions/a/b")]:
            with self.assertRaises(ev.RefusedRequest, msg=f"{method} {path}"):
                ev.guard(method, path)

    def test_seeding_and_restoring_leave_the_log_as_it_was(self):
        log = {"aspect_ratio": {"key": "aspect_ratio", "value": "9:16", "source": "user"},
               "tone": {"key": "tone", "value": "warm", "source": "assistant"}}
        client = FakeClient(log)
        before = client.decisions()
        added = ev.seed_decisions(client, before, {"aspect_ratio": "16:9", "music": "piano"})
        self.assertEqual(added, ["music"])  # a decided key is left alone
        self.assertEqual(client.log["aspect_ratio"]["value"], "9:16")
        # During the case the assistant remembers something new and replaces its own entry.
        client.log["language"] = {"key": "language", "value": "en", "source": "assistant"}
        client.log["tone"] = {"key": "tone", "value": "playful", "source": "assistant"}
        notes = ev.restore_decisions(client, before, added)
        self.assertEqual({k: v["value"] for k, v in client.log.items()}, {"aspect_ratio": "9:16", "tone": "warm"})
        self.assertIn("removed language the assistant remembered", notes)
        self.assertIn("restored tone (now marked as yours)", notes)
        self.assertTrue(all(m in ("PUT", "DELETE") and p.startswith("/decisions/") for m, p, _ in client.sent))

    def test_an_unreadable_log_stops_the_case_before_any_change(self):
        class Broken(ev.Client):
            def request(self, method, path, body=None):
                ev.guard(method, path)
                if method != "GET":
                    raise AssertionError(f"changed the log: {method} {path}")
                return None

        with self.assertRaises(RuntimeError):
            ev.run_case(Broken("http://unused"), case(seed={"music": "piano"}, settled=["music"]), CLIPS)


class FakeBackend(ev.Client):
    """A backend in memory: a project at the clips stage whose assistant asks about the
    music it was already told. Records every request, through the real guard."""

    def __init__(self):
        super().__init__("http://unused")
        self.log, self.sent, self.chats = {}, [], []

    def request(self, method, path, body=None):
        ev.guard(method, path)
        self.sent.append((method, path))
        if path == "/stages":
            return {"current": "clips", "summary": "Scene videos: ready", "next": {"action": "render_scenes", "scene_ids": ["s1", "s2"]}}
        if path == "/decisions" and method == "GET":
            return {"decisions": list(copy.deepcopy(self.log).values())}
        key = ev.urllib.parse.unquote(path.rsplit("/", 1)[-1])
        if method == "PUT":
            self.log[key] = {"key": key, "value": body["value"], "source": "user"}
            return self.log[key]
        if method == "DELETE":
            self.log.pop(key, None)
            return None
        self.chats.append(body)
        decision = {"mode": "act", "assumptions": []}
        if "music" in self.log and not body.get("full"):
            decision = {"mode": "ask", "question": "Which music should play?", "reason": "options", "key": "music", "options": [{"label": "Piano"}]}
        self.log["notes"] = {"key": "notes", "value": "likes it short", "source": "assistant"}  # remembered
        return {"intent": {"action": "render_scenes", "reply": GOOD, "decision": decision}}


class RunTest(unittest.TestCase):
    def test_a_run_scores_reports_and_compares(self):
        import tempfile
        backend = FakeBackend()
        with tempfile.TemporaryDirectory() as tmp, contextlib.redirect_stdout(io.StringIO()) as out:
            report = os.path.join(tmp, "now.json")
            args = ev.argparse.Namespace(only="", out=report, compare="")
            code = ev.run_stages(backend, args)
            with open(report) as f:
                data = ev.json.load(f)
            again = ev.run_stages(backend, ev.argparse.Namespace(only="", out="", compare=report))
        rows = {r["id"]: r for r in data["rows"]}
        self.assertEqual(code, 1)  # the music question fails a case
        self.assertTrue(rows["brief-idea-full"]["skipped"])
        self.assertIn("asked again about music", rows["clips-seeded-music-manual"]["problems"])
        self.assertTrue(rows["clips-render-full"]["ok"], rows["clips-render-full"]["problems"])
        self.assertEqual(data["summary"]["skipped"] + data["summary"]["cases"], len(ev.STAGE_CASES))
        self.assertIn("unnecessary-question rate", out.getvalue())
        self.assertIn("no regressions", out.getvalue())
        self.assertEqual(again, 1)
        # The app's mode fields are sent as the chat panel sends them.
        modes = {(bool(c.get("auto")), bool(c.get("full"))) for c in backend.chats}
        self.assertEqual(modes, {(False, False), (True, False), (True, True)})
        # Only chat, reads and the case's own decision-log changes; the log ends as it began.
        self.assertEqual(backend.log, {})
        self.assertFalse([r for r in backend.sent if r[0] == "POST" and r[1] != "/chat"])


class CasesTest(unittest.TestCase):
    def test_built_in_cases_are_valid(self):
        self.assertEqual(ev.validate_cases(ev.STAGE_CASES), [])
        covered = {s for c in ev.STAGE_CASES for s in c["stages"]}
        for stage in ("brief", "script", "characters", "shots", "scene_pictures", "clips", "edit", "export", "any"):
            self.assertIn(stage, covered)
        self.assertEqual({c["mode"] for c in ev.STAGE_CASES}, {"manual", "auto", "full"})

    def test_bad_definitions_are_caught(self):
        bad = [
            case(id="a", stages=["nowhere"]),
            case(id="b", mode="turbo"),
            case(id="c", expect={"actions": ["fly"]}),
            case(id="d", expect={"decision": "maybe"}),
            case(id="e", seed={"Bad Key": "x"}, settled=["Bad Key"]),
            case(id="f", seed={"music": "piano"}),
            case(id="g", mode="full", expect={"decision": "ask", "ask_reasons": ["options"]}),
            case(id="g", messages=[""]),
            case(id="h", expect={"language": "klingon"}),
            case(id="i", extra=True),
        ]
        problems = " ".join(ev.validate_cases(bad))
        for cid in "abcdefghi":
            self.assertIn(cid + ":", problems)
        self.assertIn("duplicate id", problems)

    def test_case_applies_to_its_stage(self):
        self.assertTrue(ev.case_applies(case(), {"current": "clips"}))
        self.assertFalse(ev.case_applies(case(), {"current": "edit"}))
        self.assertTrue(ev.case_applies(case(stages=["any"]), {"current": "edit"}))

    def test_dry_run_passes_and_catches_a_broken_case(self):
        with contextlib.redirect_stdout(io.StringIO()) as out:
            self.assertEqual(ev.dry_run(), 0)
            self.assertEqual(ev.dry_run([case(id="x", stages=["nowhere"])]), 1)
        self.assertIn("nothing was sent", out.getvalue())


if __name__ == "__main__":
    unittest.main()
