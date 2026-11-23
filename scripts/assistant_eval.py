#!/usr/bin/env python3
"""Score the Studio assistant against a live VideoFlow backend.

RUN BY HAND ONLY. Every chat message is one turn of the AI session selected in the
app (your Claude Code or Codex sign-in), so a run spends real session usage: about one
turn per case. Never run it in CI or from a script; CI runs the deterministic half
(go test ./internal/api -run 'ChatPrompt|ChatDecision').

Suites
  --suite basic   (default) everyday requests: checks the button (action) the
                  assistant prepares and the reply (language, length, no jargon).
  --suite stages  scenario cases per pipeline stage, built on GET /stages of the
                  project open in the app. Cases for other stages are skipped, so
                  run it on projects at different stages to cover them all.

Usage
  python3 scripts/assistant_eval.py --suite stages [--api http://localhost:8080/api]
          [--only text] [--out report.json] [--compare previous.json]
  python3 scripts/assistant_eval.py --suite stages --dry-run   # offline self-check
  python3 -m unittest scripts/test_assistant_eval.py           # offline unit tests

What it touches
  Only POST /chat (which prepares a button and runs nothing), GET /stages,
  /decisions, /characters and /scenes, and PUT/DELETE /decisions/{key} for the
  decisions a case seeds. Any other request is refused by the script itself
  (see ALLOWED_REQUESTS). A seeded decision is only added when the key is not in the
  log yet, and is removed after the case; decisions the assistant remembers during a
  case are removed or restored too. Each chat turn also records a proposal in the
  project's assistant session, as any chat message does.

Stages rubric, per case (each criterion passes, fails or does not apply)
  action       the button fits the stage (stages.next.action where the case says NEXT)
  decision     act/ask/inform as expected; in Full auto it may only ask to confirm a
               new story's plan, before a destructive step, or for a missing input
  no_repeat    no question about something decided_so_far already settles
  assumptions  when it chose a default, it says what it assumed
  reply        the user's language, at most 120 words, no internal jargon
  no_spam      Full auto: no per-step or per-scene buttons, one batch step instead
Case score = passed / applicable criteria. Stage and overall scores = mean case score
x 100. Unnecessary-question rate = cases where the assistant asked (including
questions the server overruled) / cases where no question was expected.
"""
import argparse, json, re, sys, time, urllib.error, urllib.parse, urllib.request

JARGON = re.compile(r"\b(proposal|native session|pipeline|intent|action card|director)\b", re.I)
# Internal names leaking into a reply: decided_so_far, render_scenes, stages.next…
INTERNAL = re.compile(r"\b[a-z]+_[a-z_]+\b|\bstages\.\w+|\bdecision\.\w+")
SPANISH = re.compile(r"[¿¡ñáéíóú]|\b(qué|cómo|quieres|estilo|video|gato|escena|escenas|listo|vamos)\b", re.I)
CJK = re.compile(r"[㐀-鿿]")
# Full auto must not hand the user one button per step or scene.
BUTTON_SPAM = re.compile(r"\b(press|click|tap)\b[^.]*\b(each|every|one by one|per scene|for each)\b|\bone (button|click) per\b|\bper-(scene|step) buttons?\b", re.I)
MAX_WORDS = 120

# ---------------------------------------------------------------------------
# HTTP: the only requests this script may send.

ALLOWED_REQUESTS = [
    ("GET", re.compile(r"^/(stages|decisions|characters|scenes)$")),
    ("POST", re.compile(r"^/chat$")),
    ("PUT", re.compile(r"^/decisions/[^/]+$")),
    ("DELETE", re.compile(r"^/decisions/[^/]+$")),
]


class RefusedRequest(Exception):
    """A request outside ALLOWED_REQUESTS: a run endpoint, a paid step, anything else."""


def guard(method, path):
    """Raise RefusedRequest unless the request is one the evaluation may send."""
    if not any(method == m and rx.match(path) for m, rx in ALLOWED_REQUESTS):
        raise RefusedRequest(f"the assistant evaluation never sends {method} {path}")


class Client:
    def __init__(self, api, timeout=600):
        self.api, self.timeout = api.rstrip("/"), timeout

    def request(self, method, path, body=None):
        guard(method, path)
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.api + path, data=data, method=method, headers={"content-type": "application/json"})
        with urllib.request.urlopen(req, timeout=self.timeout) as r:
            raw = r.read()
        return json.loads(raw) if raw else None

    def get(self, path):
        try:
            return self.request("GET", path) or []
        except RefusedRequest:
            raise
        except Exception:
            return []

    def chat(self, body):
        return self.request("POST", "/chat", body)

    def decisions(self):
        """The decision log by key. Raises when it cannot be read: restoring the log
        from a snapshot that failed to load would delete the user's decisions."""
        out = self.request("GET", "/decisions")
        if not isinstance(out, dict) or not isinstance(out.get("decisions"), list):
            raise RuntimeError("GET /decisions gave no decision list")
        return {d["key"]: d for d in out["decisions"]}

    def put_decision(self, key, value):
        return self.request("PUT", "/decisions/" + urllib.parse.quote(key, safe=""), {"value": value})

    def delete_decision(self, key):
        return self.request("DELETE", "/decisions/" + urllib.parse.quote(key, safe=""))


# ---------------------------------------------------------------------------
# Basic suite (unchanged cases).

# (message, allowed actions, extra checks). needs: "char", "char2", "scene".
CASES = [
    ("hi", {"unknown"}, {}),
    ("what do I do next", {"unknown", "open_editor", "generate_script", "generate_scenes", "render_scene"}, {}),
    ("make a video about a dog who learns to surf", {"generate_script"}, {}),
    ("FOLLOWUP:english, 15 seconds, 2 scenes, cartoon style, for tiktok", {"generate_script"}, {"reply_has": "Got it", "brief_style": True}),
    ("make {char} bigger in the first scene", {"refine_scene"}, {"needs": ["char", "scene"]}),
    ("add voice to my video", {"open_editor"}, {"panel": "voice", "needs": ["scene"]}),
    ("{char} should say hello", {"open_editor"}, {"panel": "voice", "needs": ["char", "scene"]}),
    ("add music", {"open_editor"}, {"panel": "music", "needs": ["scene"]}),
    ("add subtitles", {"caption", "open_editor"}, {"needs": ["scene"]}),
    ("translate the subtitles to spanish", {"open_editor"}, {"panel": "captions", "needs": ["scene"]}),
    ("export the video as mp4", {"open_editor"}, {"panel": "export", "needs": ["scene"]}),
    ("render it", {"render_scene"}, {"needs": ["scene"]}),
    ("why did my video fail?", {"unknown", "open_editor", "retry_render"}, {}),
    ("delete everything", {"unknown"}, {}),
    ("make a picture of a tokyo street at night", {"generate_image"}, {}),
    ("dress {char} all in black", {"update_character"}, {"changes_has": "black", "needs": ["char"]}),
    ("give {char2} a red hat", {"update_character"}, {"changes_has": "hat", "needs": ["char2"]}),
    ("redo {char}'s reference picture", {"generate_character_reference"}, {"needs": ["char"]}),
    ("check if the storyboard matches the style", {"audit_storyboard"}, {"needs": ["scene"]}),
    ("make it longer", {"generate_scenes", "generate_script"}, {}),
    ("how much will this cost?", {"unknown"}, {}),
    ("asdfgh", {"unknown"}, {}),
    ("undo that", {"unknown", "open_editor"}, {}),
    ("hazme un video de un gato que baila", {"generate_script"}, {"spanish": True}),
    ("¿cómo exporto mi video?", {"open_editor"}, {"panel": "export", "spanish": True, "needs": ["scene"]}),
]


def run_basic(client, args):
    brief, rows, passed = None, [], 0
    names = [c.get("name", "") for c in client.get("/characters") if c.get("name")]
    have = {"char": len(names) >= 1, "char2": len(names) >= 2, "scene": bool(client.get("/scenes"))}
    fill = {"char": names[0] if names else "", "char2": names[1] if len(names) > 1 else ""}
    cases, skipped = [], []
    for message, allowed, checks in CASES:
        message = message.format(**fill)
        if args.only.lower() not in message.lower():
            continue
        missing = [n for n in checks.get("needs", []) if not have[n]]
        (skipped if missing else cases).append((message, allowed, checks))
    if skipped:
        print(f"Skipping {len(skipped)} case(s) this project can't answer yet (no characters or scenes)")
    for message, allowed, checks in cases:
        body = {"message": message, "history": [], "selected": None}
        if message.startswith("FOLLOWUP:"):
            message = message[len("FOLLOWUP:"):]
            body = {"message": message, "history": [], "selected": None, "creation_brief": brief}
        started = time.time()
        problems = []
        try:
            intent = client.chat(body)["intent"]
        except urllib.error.HTTPError as e:
            intent, problems = {}, [f"HTTP {e.code}: {e.read().decode()[:200]}"]
        except Exception as e:  # network or timeout
            intent, problems = {}, [str(e)]
        if intent.get("creation_brief"):
            brief = intent["creation_brief"]
        action, reply = intent.get("action"), intent.get("reply") or ""
        if intent and action not in allowed:
            problems.append(f"action {action}, want {'/'.join(sorted(allowed))}")
        if "panel" in checks and action == "open_editor" and intent.get("editor_panel") != checks["panel"]:
            problems.append(f"panel {intent.get('editor_panel')}, want {checks['panel']}")
        if "changes_has" in checks and checks["changes_has"] not in json.dumps(intent.get("character_changes") or {}).lower():
            problems.append("character change misses " + checks["changes_has"])
        if checks.get("reply_has") and checks["reply_has"] not in reply:
            problems.append("reply misses " + checks["reply_has"])
        if checks.get("brief_style") and not (intent.get("creation_brief") or {}).get("style_preset_id"):
            problems.append("style word not mapped to a style")
        if checks.get("spanish") and not SPANISH.search(reply):
            problems.append("reply not in Spanish")
        words = len(reply.split())
        if words > MAX_WORDS:
            problems.append(f"reply {words} words (max {MAX_WORDS})")
        if JARGON.search(reply):
            problems.append("jargon: " + JARGON.search(reply).group(0))
        ok = intent and not problems
        passed += bool(ok)
        rows.append({"message": message, "action": action, "panel": intent.get("editor_panel"), "words": words, "seconds": round(time.time() - started), "ok": bool(ok), "problems": problems, "reply": reply})
        print(("PASS " if ok else "FAIL ") + f"{message!r} -> {action} ({words} words)" + ("" if ok else "  · " + "; ".join(problems)), flush=True)
    print(f"\n{passed}/{len(cases)} passed")
    if args.out:
        with open(args.out, "w") as f:
            json.dump(rows, f, indent=1, ensure_ascii=False)
    return 0 if passed == len(cases) else 1


# ---------------------------------------------------------------------------
# Stages suite: case definitions.

STAGES = ["brief", "characters", "voices", "script", "shots", "scene_pictures", "clips", "review", "edit", "export"]
MODES = {"manual": {}, "auto": {"auto": True}, "full": {"auto": True, "full": True}}
DECISION_EXPECTATIONS = {"act", "ask", "inform", "act_or_inform", "not_ask", "any"}
ASK_REASONS = {"missing_input", "expensive", "options", "confirm_plan", "destructive"}
# Full auto may only ask these (internal/api/chat_decision.go fullAutoAskAllowed).
FULL_AUTO_ASK = {"confirm_plan", "destructive", "missing_input"}
ACTIONS = {
    "unknown", "generate_script", "generate_image", "generate_character_reference", "generate_character_references",
    "update_character", "generate_scenes", "generate_shots", "storyboard", "audit_storyboard", "generate_assets",
    "plan_assets", "refine_scene", "refine_shot", "delete_scene", "style_ingest", "render_scene", "render_scenes",
    "render_shot", "retry_render", "finish_video", "check_video", "stop_waiting", "switch_video_service",
    "clear_failed_videos", "set_music", "open_editor",
}
# A chat action fits the stage map's next step when it does that step, or a batch of it.
NEXT_EQUIVALENT = {
    "generate_script": {"generate_script"},
    "generate_character_references": {"generate_character_references"},
    "open_voices": {"open_editor", "unknown"},  # a navigation step: no chat button
    "generate_shots": {"generate_shots"},
    "generate_assets": {"generate_assets", "storyboard"},
    "storyboard": {"storyboard", "generate_assets"},
    "render_scene": {"render_scene", "render_scenes", "finish_video"},
    "render_scenes": {"render_scenes", "finish_video"},
    "check_video": {"check_video", "stop_waiting"},
    "watch_videos": {"unknown", "open_editor"},
    "finish_video": {"finish_video"},
    "open_editor": {"open_editor"},
}
# The single-item step a batch replaces; offering it in Full auto is per-step spam.
SPAM_SINGLE = {"generate_character_references": "generate_character_reference", "render_scenes": "render_scene"}
# Words that show a question is about a settled decision-log key.
KEY_TOPICS = {
    "aspect_ratio": r"format|aspect|vertical|horizontal|portrait|landscape|square|9:16|16:9|1:1",
    "platform": r"platform|tiktok|youtube|reels|shorts|instagram",
    "language": r"language|english|spanish|chinese|idioma|语言",
    "music": r"music|soundtrack|song|piano|música|音乐",
    "conversation_mode": r"narrat|dialogue|voice-?over|talking",
    "style_id": r"style|look|anime|cartoon|realistic|3d|watercolor|estilo",
    "target_duration": r"how long|length|duration|seconds|minutes",
    "tone": r"tone|mood",
    "audience": r"audience|who is it for",
}
DECISION_KEY = re.compile(r"^[a-z][a-z0-9_]*(:[A-Za-z0-9_.\-]+)*$")

# Each case: id, stages it applies to ("any" = every stage), mode, messages (the last is
# scored; earlier ones carry the creative brief forward), expect, and optional seed
# (decisions the case adds to the log) and settled (log keys that must not be asked
# about when present). expect: actions (a list; "NEXT" = the stage map's next step),
# decision (see DECISION_EXPECTATIONS), ask_reasons (reasons a question may give),
# assumptions (a default was chosen, so it must be stated), language, panel.
STAGE_CASES = [
    # Any stage.
    {"id": "any-next-manual", "stages": ["any"], "mode": "manual", "messages": ["what's next?"],
     "expect": {"actions": ["NEXT", "unknown"], "decision": "not_ask", "ask_reasons": ["missing_input"], "language": "en"}},
    {"id": "any-next-auto", "stages": ["any"], "mode": "auto", "messages": ["what should I do now?"],
     "expect": {"actions": ["NEXT", "unknown"], "decision": "not_ask", "ask_reasons": ["missing_input"], "language": "en"}},
    {"id": "any-continue-full", "stages": ["any"], "mode": "full", "messages": ["just continue"],
     "expect": {"actions": ["NEXT", "unknown"], "decision": "not_ask", "ask_reasons": ["missing_input", "confirm_plan"], "language": "en"}},
    {"id": "any-hi", "stages": ["any"], "mode": "manual", "messages": ["hi"],
     "expect": {"actions": ["unknown"], "decision": "inform", "language": "en"}},
    {"id": "any-next-chinese", "stages": ["any"], "mode": "manual", "messages": ["下一步做什么？"],
     "expect": {"actions": ["NEXT", "unknown"], "decision": "not_ask", "ask_reasons": ["missing_input"], "language": "zh"}},
    # Brief: no story yet.
    {"id": "brief-idea-full", "stages": ["brief"], "mode": "full", "messages": ["make a video about a dog who learns to surf"],
     "expect": {"actions": ["generate_script"], "decision": "not_ask", "ask_reasons": ["confirm_plan"], "assumptions": True, "language": "en"}},
    {"id": "brief-vague-manual", "stages": ["brief"], "mode": "manual", "messages": ["make a video"],
     "expect": {"actions": ["unknown", "generate_script"], "decision": "ask", "ask_reasons": ["missing_input", "options"], "language": "en"}},
    {"id": "brief-seeded-music-manual", "stages": ["brief"], "mode": "manual", "seed": {"music": "slow sparse piano", "conversation_mode": "narration"},
     "messages": ["make a 20 second video about a cat who cooks pasta"], "settled": ["music", "conversation_mode"],
     "expect": {"actions": ["generate_script"], "decision": "not_ask", "ask_reasons": ["confirm_plan"], "assumptions": True, "language": "en"}},
    {"id": "brief-followup-auto", "stages": ["brief"], "mode": "auto",
     "messages": ["a video about a robot learning to paint", "english, 15 seconds, 2 scenes, cartoon style, for tiktok"],
     "expect": {"actions": ["generate_script"], "decision": "not_ask", "ask_reasons": ["confirm_plan"], "language": "en"}},
    {"id": "brief-spanish-auto", "stages": ["brief"], "mode": "auto", "messages": ["hazme un video de un gato que baila"],
     "expect": {"actions": ["generate_script"], "decision": "not_ask", "ask_reasons": ["confirm_plan"], "assumptions": True, "language": "es"}},
    # Script: the brief is settled, no scenes yet.
    {"id": "script-write-manual", "stages": ["script"], "mode": "manual", "messages": ["write the story"],
     "settled": ["language", "aspect_ratio", "platform", "style_id", "target_duration"],
     "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["confirm_plan", "missing_input"], "language": "en"}},
    {"id": "script-write-full", "stages": ["script"], "mode": "full", "messages": ["go ahead"],
     "settled": ["language", "aspect_ratio", "platform", "style_id", "target_duration"],
     "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["confirm_plan", "missing_input"], "language": "en"}},
    # Characters without pictures.
    {"id": "characters-pictures-full", "stages": ["characters"], "mode": "full", "messages": ["make the character pictures"],
     "expect": {"actions": ["NEXT"], "decision": "act", "language": "en"}},
    {"id": "characters-pictures-manual", "stages": ["characters"], "mode": "manual", "messages": ["make pictures for all my characters"],
     "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["expensive"], "language": "en"}},
    # Shots.
    {"id": "shots-plan-auto", "stages": ["shots"], "mode": "auto", "messages": ["plan the shots"],
     "expect": {"actions": ["NEXT"], "decision": "act", "language": "en"}},
    {"id": "shots-next-full", "stages": ["shots"], "mode": "full", "messages": ["keep going"],
     "expect": {"actions": ["NEXT", "render_scenes", "finish_video"], "decision": "act", "language": "en"}},
    # Scene pictures.
    {"id": "pictures-make-manual", "stages": ["scene_pictures"], "mode": "manual", "messages": ["make the missing pictures"],
     "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["expensive"], "language": "en"}},
    {"id": "pictures-next-full", "stages": ["scene_pictures"], "mode": "full", "messages": ["keep going"],
     "expect": {"actions": ["NEXT", "render_scenes", "finish_video"], "decision": "act", "language": "en"}},
    # Scene videos.
    {"id": "clips-render-full", "stages": ["clips"], "mode": "full", "messages": ["make the videos"],
     "expect": {"actions": ["NEXT"], "decision": "act", "language": "en"}},
    {"id": "clips-seeded-music-manual", "stages": ["clips"], "mode": "manual", "seed": {"music": "slow sparse piano", "aspect_ratio": "9:16"},
     "messages": ["make the videos"], "settled": ["music", "aspect_ratio"],
     "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["expensive"], "language": "en"}},
    {"id": "clips-cost-manual", "stages": ["clips"], "mode": "manual", "messages": ["how much will the videos cost?"],
     "expect": {"actions": ["NEXT", "unknown"], "decision": "not_ask", "language": "en"}},
    {"id": "clips-spanish-auto", "stages": ["clips"], "mode": "auto", "messages": ["haz los videos de las escenas"],
     "expect": {"actions": ["NEXT"], "decision": "not_ask", "ask_reasons": ["expensive"], "language": "es"}},
    # Edit: every clip is made, not joined yet.
    {"id": "edit-finish-full", "stages": ["edit", "review"], "mode": "full", "messages": ["finish the video"],
     "expect": {"actions": ["finish_video"], "decision": "act", "language": "en"}},
    {"id": "edit-finish-manual", "stages": ["edit", "review"], "mode": "manual", "messages": ["join the scenes into one video"],
     "expect": {"actions": ["finish_video", "open_editor"], "decision": "not_ask", "language": "en"}},
    {"id": "edit-spanish-manual", "stages": ["edit", "review"], "mode": "manual", "messages": ["termina el video"],
     "expect": {"actions": ["finish_video", "open_editor"], "decision": "not_ask", "language": "es"}},
    # Export.
    {"id": "export-mp4-manual", "stages": ["export"], "mode": "manual", "messages": ["export the video as mp4"],
     "expect": {"actions": ["open_editor"], "panel": "export", "decision": "act_or_inform", "language": "en"}},
    {"id": "export-full", "stages": ["export"], "mode": "full", "messages": ["download the final video"],
     "expect": {"actions": ["open_editor"], "panel": "export", "decision": "act_or_inform", "language": "en"}},
]


def validate_cases(cases):
    """Return the problems of the case definitions ([] when they are sound)."""
    problems, seen = [], set()
    for c in cases:
        cid = c.get("id") or "?"
        if cid in seen:
            problems.append(f"{cid}: duplicate id")
        seen.add(cid)
        unknown = set(c) - {"id", "stages", "mode", "messages", "expect", "seed", "settled"}
        if unknown:
            problems.append(f"{cid}: unknown fields {sorted(unknown)}")
        stages = c.get("stages") or []
        if not stages or any(s != "any" and s not in STAGES for s in stages):
            problems.append(f"{cid}: stages {stages}")
        if c.get("mode") not in MODES:
            problems.append(f"{cid}: mode {c.get('mode')}")
        msgs = c.get("messages")
        if not msgs or not all(isinstance(m, str) and m.strip() for m in msgs):
            problems.append(f"{cid}: messages")
        e = c.get("expect") or {}
        unknown = set(e) - {"actions", "decision", "ask_reasons", "assumptions", "language", "panel"}
        if unknown:
            problems.append(f"{cid}: unknown expect fields {sorted(unknown)}")
        actions = e.get("actions") or []
        if not actions or any(a != "NEXT" and a not in ACTIONS for a in actions):
            problems.append(f"{cid}: actions {actions}")
        if e.get("decision") not in DECISION_EXPECTATIONS:
            problems.append(f"{cid}: decision {e.get('decision')}")
        if any(r not in ASK_REASONS for r in e.get("ask_reasons", [])):
            problems.append(f"{cid}: ask_reasons {e.get('ask_reasons')}")
        if e.get("decision") == "ask" and c.get("mode") == "full" and not set(e.get("ask_reasons", [])) & FULL_AUTO_ASK:
            problems.append(f"{cid}: expects a question Full auto does not allow")
        if e.get("language") not in ("en", "es", "zh"):
            problems.append(f"{cid}: language {e.get('language')}")
        for key, value in (c.get("seed") or {}).items():
            if not DECISION_KEY.match(key) or not str(value).strip() or len(str(value)) > 500:
                problems.append(f"{cid}: seed {key}={value!r}")
        for key in c.get("settled") or []:
            if not DECISION_KEY.match(key):
                problems.append(f"{cid}: settled key {key}")
        for key in set(c.get("seed") or {}) - set(c.get("settled") or []):
            problems.append(f"{cid}: seeds {key} but does not check it is not asked again")
    return problems


def case_applies(case, stage_map):
    """Whether the case fits the project's current stage."""
    current = (stage_map or {}).get("current")
    return "any" in case["stages"] or current in case["stages"]


# ---------------------------------------------------------------------------
# Stages suite: scoring.

def reply_language_ok(reply, lang):
    if lang == "zh":
        return bool(CJK.search(reply))
    if lang == "es":
        return bool(SPANISH.search(reply)) and not CJK.search(reply)
    return not CJK.search(reply) and not re.search(r"[¿¡]", reply)


def reply_problems(reply, lang):
    problems = []
    if not reply.strip():
        problems.append("empty reply")
    if not reply_language_ok(reply, lang):
        problems.append(f"reply not in {lang}")
    words = len(reply.split()) if lang != "zh" else len(CJK.findall(reply)) // 2
    if words > MAX_WORDS:
        problems.append(f"reply {words} words (max {MAX_WORDS})")
    for rx in (JARGON, INTERNAL):
        m = rx.search(reply)
        if m:
            problems.append("jargon: " + m.group(0))
    return problems


def allowed_actions(expect, stage_map):
    """The actions that pass criterion 1, with NEXT resolved from the stage map."""
    allowed = set()
    for a in expect["actions"]:
        if a != "NEXT":
            allowed.add(a)
            continue
        nxt = ((stage_map or {}).get("next") or {}).get("action")
        allowed |= NEXT_EQUIVALENT.get(nxt, {nxt} if nxt else {"unknown", "open_editor"})
    return allowed


def asked(decision):
    """Whether the assistant asked: a question shown, or one the server overruled."""
    return decision.get("mode") == "ask" or bool(decision.get("overruled"))


def settled_question(decision, settled_keys):
    """The settled key a question is about, or ""."""
    if not asked(decision) or not settled_keys:
        return ""
    if decision.get("overruled") == "already_decided":
        return decision.get("key") or "a decided key"
    if decision.get("key") in settled_keys:
        return decision["key"]
    text = " ".join([decision.get("question") or ""] + [o.get("label", "") for o in decision.get("options") or []]).lower()
    for key in settled_keys:
        if KEY_TOPICS.get(key) and re.search(KEY_TOPICS[key], text):
            return key
    return ""


def score_case(case, intent, stage_map, settled_keys=()):
    """Score one case's final reply. Returns {criteria: {name: True/False/None}, problems, score, asked, question_expected}."""
    e, mode = case["expect"], case["mode"]
    decision = intent.get("decision") or {}
    action, reply = intent.get("action") or "unknown", intent.get("reply") or ""
    criteria, problems = {}, []

    # 1. The button fits the stage.
    allowed = allowed_actions(e, stage_map)
    criteria["action"] = action in allowed
    if not criteria["action"]:
        problems.append(f"action {action}, want {'/'.join(sorted(allowed))}")
    if e.get("panel") and action == "open_editor" and intent.get("editor_panel") != e["panel"]:
        criteria["action"] = False
        problems.append(f"panel {intent.get('editor_panel')}, want {e['panel']}")

    # 2. Act or ask, as the case and the mode allow.
    want, got = e["decision"], decision.get("mode") or "inform"
    allowed_question = question_allowed(case, decision)
    if decision.get("overruled") == "full_auto":
        criteria["decision"] = False
        problems.append("asked in Full auto (the server overruled it)")
    elif got == "ask":
        criteria["decision"] = allowed_question
        if not allowed_question:
            problems.append(f"asked ({decision.get('reason') or 'no reason'}): {(decision.get('question') or '')[:80]}")
    elif want == "ask":
        criteria["decision"] = False
        problems.append(f"decided ({got}) where a question was needed")
    else:
        criteria["decision"] = want not in ("act", "inform") or got == want
        if not criteria["decision"]:
            problems.append(f"decision {got}, want {want}")

    # 3. No question about what the decision log settles.
    settled = [k for k in (case.get("settled") or []) if k in settled_keys]
    if settled:
        about = settled_question(decision, settled)
        criteria["no_repeat"] = not about
        if about:
            problems.append(f"asked again about {about}")
    else:
        criteria["no_repeat"] = None

    # 4. Assumptions stated when it chose a default.
    if e.get("assumptions") and got != "ask":
        criteria["assumptions"] = bool([a for a in decision.get("assumptions") or [] if str(a).strip()])
        if not criteria["assumptions"]:
            problems.append("chose defaults without stating them")
    else:
        criteria["assumptions"] = None

    # 5. Reply: language, length, plain words.
    rp = reply_problems(reply, e["language"])
    criteria["reply"] = not rp
    problems += rp

    # 6. Full auto: one batch step, no per-step buttons.
    if mode == "full":
        nxt = (stage_map or {}).get("next") or {}
        items = nxt.get("scene_ids") or nxt.get("character_ids") or []
        spam = SPAM_SINGLE.get(nxt.get("action")) == action and len(items) > 1
        spam = spam or bool(BUTTON_SPAM.search(reply))
        criteria["no_spam"] = not spam
        if spam:
            problems.append("per-step buttons in Full auto")
    else:
        criteria["no_spam"] = None

    applicable = [v for v in criteria.values() if v is not None]
    return {
        "criteria": criteria, "problems": problems,
        "score": round(sum(applicable) / len(applicable), 4) if applicable else 1.0,
        "asked": asked(decision), "unnecessary_question": asked(decision) and not allowed_question,
    }


def question_allowed(case, decision):
    """Whether a question shown to the user is one the case allows: the case may ask,
    with one of its reasons, and in Full auto only for a reason Full auto allows. A
    question the server had to overrule was never allowed."""
    want, reason = case["expect"]["decision"], decision.get("reason") or ""
    if decision.get("mode") != "ask" or decision.get("overruled") or want in ("act", "inform", "act_or_inform"):
        return False
    if case["mode"] == "full" and reason not in FULL_AUTO_ASK:
        return False
    reasons = case["expect"].get("ask_reasons") or []
    if want == "not_ask":
        return reason in reasons
    return not reasons or reason in reasons


def summarize(rows):
    """Per-stage, per-criterion and overall scores and the unnecessary-question rate."""
    scored = [r for r in rows if not r.get("skipped")]
    by_stage, by_criterion = {}, {}
    for r in scored:
        by_stage.setdefault(r["stage"], []).append(r["score"])
        for name, v in r["criteria"].items():
            if v is not None:
                by_criterion.setdefault(name, []).append(1.0 if v else 0.0)
    mean = lambda xs: round(100 * sum(xs) / len(xs), 1) if xs else None
    no_question = [r for r in scored if r["expect_decision"] != "ask"]
    unnecessary = [r for r in no_question if r["unnecessary_question"]]
    return {
        "cases": len(scored), "passed": sum(1 for r in scored if r["ok"]), "skipped": len(rows) - len(scored),
        "overall": mean([r["score"] for r in scored]),
        "per_stage": {k: mean(v) for k, v in sorted(by_stage.items())},
        "per_criterion": {k: mean(v) for k, v in sorted(by_criterion.items())},
        "unnecessary_question_rate": round(len(unnecessary) / len(no_question), 3) if no_question else None,
    }


def compare(report, previous):
    """Regressions against an earlier report: lines to print (empty when none)."""
    lines = []
    # A case is compared with the same case at the same stage (runs on other projects differ).
    before = {(r["id"], r.get("stage")): r for r in previous.get("rows", []) if not r.get("skipped")}
    for r in report.get("rows", []):
        old = before.get((r["id"], r.get("stage")))
        if r.get("skipped") or not old:
            continue
        if old["ok"] and not r["ok"]:
            lines.append(f"REGRESSION {r['id']}: passed before, now {'; '.join(r['problems'])}")
        elif r["score"] < old["score"]:
            lines.append(f"worse {r['id']}: {old['score']:.2f} -> {r['score']:.2f}")
    s, p = report.get("summary", {}), previous.get("summary", {})
    if s.get("overall") is not None and p.get("overall") is not None and s["overall"] < p["overall"]:
        lines.append(f"overall {p['overall']} -> {s['overall']}")
    if s.get("unnecessary_question_rate") is not None and p.get("unnecessary_question_rate") is not None and s["unnecessary_question_rate"] > p["unnecessary_question_rate"]:
        lines.append(f"unnecessary-question rate {p['unnecessary_question_rate']} -> {s['unnecessary_question_rate']}")
    return lines


# ---------------------------------------------------------------------------
# Stages suite: running a case against the live backend.

def seed_decisions(client, existing, seed):
    """Add the case's decisions that are not in the log (existing) yet. Returns the
    keys added; a key already decided is left as it is and counts as settled."""
    added = []
    for key, value in (seed or {}).items():
        if key in existing:
            continue
        client.put_decision(key, value)
        added.append(key)
    return added


def restore_decisions(client, before, added):
    """Undo the case's changes to the decision log: remove what it seeded and what the
    assistant remembered, and put back values the assistant replaced. Returns notes."""
    notes, now = [], client.decisions()
    for key in now:
        if key in added or key not in before:
            client.delete_decision(key)
            if key not in added:
                notes.append(f"removed {key} the assistant remembered")
    for key, old in before.items():
        cur = now.get(key)
        if cur is None or cur.get("value") != old.get("value"):
            client.put_decision(key, old["value"])
            notes.append(f"restored {key}" + (" (now marked as yours)" if old.get("source") != "user" else ""))
    return notes


def run_case(client, case, stage_map):
    before = client.decisions()
    added = seed_decisions(client, before, case.get("seed"))
    settled_keys = set(before) | set(added)
    intent, problems, brief, started = {}, [], None, time.time()
    try:
        for message in case["messages"]:
            body = {"message": message, "history": [], "selected": None, **MODES[case["mode"]]}
            if brief:
                body["creation_brief"] = brief
            intent = (client.chat(body) or {}).get("intent") or {}
            brief = intent.get("creation_brief") or brief
            if intent.get("action") in ("clear_failed_videos", "set_music"):
                problems.append(f"the reply did {intent['action']} while planning")
    except urllib.error.HTTPError as e:
        problems.append(f"HTTP {e.code}: {e.read().decode()[:200]}")
    except RefusedRequest:
        raise
    except Exception as e:  # network or timeout
        problems.append(str(e))
    finally:
        notes = restore_decisions(client, before, added)
    result = score_case(case, intent, stage_map, settled_keys) if intent else None
    return result_row(case, stage_map, intent, result, problems, round(time.time() - started), notes)


def result_row(case, stage_map, intent, result, problems, seconds=0, notes=()):
    """The report row of a case: its rubric result plus any problem running it."""
    if result is None:  # no reply at all
        result = {"criteria": {}, "problems": [], "score": 0.0, "asked": False, "unnecessary_question": False}
    problems = list(problems) + result["problems"]
    return {
        "id": case["id"], "stage": (stage_map or {}).get("current"), "mode": case["mode"], "message": case["messages"][-1],
        "action": intent.get("action"), "panel": intent.get("editor_panel"), "decision": intent.get("decision") or {},
        "criteria": result["criteria"], "score": result["score"], "ok": bool(intent) and not problems, "problems": problems,
        "asked": result["asked"], "unnecessary_question": result["unnecessary_question"], "expect_decision": case["expect"]["decision"],
        "reply": intent.get("reply") or "", "seconds": seconds, "log_notes": list(notes),
    }


def print_table(rows):
    print(f"{'case':28} {'stage':14} {'mode':6} {'action':30} {'decision':8} score  problems")
    for r in rows:
        if r.get("skipped"):
            continue
        d = (r["decision"] or {}).get("mode", "-") + ("*" if (r["decision"] or {}).get("overruled") else "")
        print(f"{r['id'][:28]:28} {str(r['stage'])[:14]:14} {r['mode']:6} {str(r['action'])[:30]:30} {d:8} {r['score']:.2f}   {'; '.join(r['problems'])[:120]}")


def print_summary(summary):
    print(f"\n{summary['passed']}/{summary['cases']} cases passed, {summary['skipped']} skipped (other stages)")
    print(f"overall score: {summary['overall']}")
    for stage, score in summary["per_stage"].items():
        print(f"  stage {stage}: {score}")
    for name, score in summary["per_criterion"].items():
        print(f"  {name}: {score}")
    print(f"unnecessary-question rate: {summary['unnecessary_question_rate']}")


def run_stages(client, args):
    stage_map = client.request("GET", "/stages") or {}
    if not stage_map.get("current"):
        print("GET /stages gave no current stage; is the backend running with a project open?")
        return 2
    print(f"Project stage: {stage_map['current']} — {stage_map.get('summary', '')}")
    rows = []
    for case in STAGE_CASES:
        if args.only.lower() not in (case["id"] + " " + " ".join(case["messages"])).lower():
            continue
        if not case_applies(case, stage_map):
            rows.append({"id": case["id"], "skipped": True, "stages": case["stages"]})
            continue
        row = run_case(client, case, stage_map)
        rows.append(row)
        print(("PASS " if row["ok"] else "FAIL ") + f"{row['id']} -> {row['action']} ({row['seconds']}s)" + ("" if row["ok"] else "  · " + "; ".join(row["problems"])), flush=True)
    summary = summarize(rows)
    print()
    print_table(rows)
    print_summary(summary)
    report = {"suite": "stages", "when": time.strftime("%Y-%m-%dT%H:%M:%S"), "stage": stage_map.get("current"), "summary": summary, "rows": rows}
    regressions = []
    if args.compare:
        with open(args.compare) as f:
            regressions = compare(report, json.load(f))
        print("\nCompared with " + args.compare + ": " + ("no regressions" if not regressions else ""))
        for line in regressions:
            print("  " + line)
    if args.out:
        with open(args.out, "w") as f:
            json.dump(report, f, indent=1, ensure_ascii=False)
    return 0 if summary["passed"] == summary["cases"] and not regressions else 1


# ---------------------------------------------------------------------------
# Dry run: the case definitions and the rubric, checked offline.

# The stage map's next step at each stage, as GET /stages gives it, for the dry run.
DRY_NEXT = {
    "brief": {"action": "generate_script"},
    "characters": {"action": "generate_character_references", "character_ids": ["c1", "c2"]},
    "voices": {"action": "open_voices"},
    "script": {"action": "generate_script"},
    "shots": {"action": "generate_shots", "scene_ids": ["s1"]},
    "scene_pictures": {"action": "generate_assets", "scene_ids": ["s1"]},
    "clips": {"action": "render_scenes", "scene_ids": ["s1", "s2"]},
    "review": {"action": "watch_videos"},
    "edit": {"action": "finish_video"},
    "export": {"action": "open_editor", "editor_panel": "export"},
}


def synthetic_intent(case, stage_map, good):
    """A reply that should pass the case (good) or fail every criterion it can."""
    e = case["expect"]
    allowed = sorted(allowed_actions(e, stage_map))
    reply = {"en": "Done. I'll make it now; the button shows the cost.", "es": "Listo, ¿quieres que haga el video?", "zh": "好的，下一步是制作视频。"}[e["language"]]
    if e["decision"] == "ask":
        reason = (e.get("ask_reasons") or ["missing_input"])[0]
        decision = {"mode": "ask", "question": "What should the video be about?", "reason": reason, "options": [{"label": "A dog", "value": "a dog"}], "assumptions": []}
    else:
        decision = {"mode": "inform" if e["decision"] == "inform" else "act", "assumptions": ["9:16 for TikTok"]}
    if good:
        return {"action": allowed[0], "reply": reply, "decision": decision, "editor_panel": e.get("panel")}
    return {"action": "delete_scene" if "delete_scene" not in allowed else "unknown", "reply": "The pipeline intent is ready: press each button per scene. " * 20,
            "decision": {"mode": "ask", "question": "Which format?", "reason": "options", "key": "aspect_ratio", "assumptions": []}}


def dry_run(cases=None):
    """Check the case definitions, and that the rubric passes a good reply and fails a
    bad one for every case at every stage it applies to. Sends nothing."""
    cases = STAGE_CASES if cases is None else cases
    problems = ["case " + p for p in validate_cases(cases)]
    checked = 0
    for case in cases if not problems else []:
        for stage in STAGES if "any" in case["stages"] else case["stages"]:
            stage_map = {"current": stage, "next": DRY_NEXT[stage]}
            settled = set(case.get("settled") or [])
            good = score_case(case, synthetic_intent(case, stage_map, True), stage_map, settled)
            bad = score_case(case, synthetic_intent(case, stage_map, False), stage_map, settled)
            if good["problems"] or good["score"] != 1.0 or good["unnecessary_question"]:
                problems.append(f"rubric {case['id']}@{stage}: a good reply scores {good['score']}: {good['problems']}")
            if bad["score"] >= 0.5 or not bad["problems"]:
                problems.append(f"rubric {case['id']}@{stage}: a bad reply scores {bad['score']}")
            checked += 1
    rows = [result_row(c, {"current": "dry"}, synthetic_intent(c, {"current": "dry", "next": None}, True), None, []) for c in cases]
    summarize(rows)  # the summary code runs on rows shaped like a live run's
    for p in problems:
        print("PROBLEM " + p)
    print(f"{len(cases)} stage cases, {checked} case x stage rubric checks: " + ("OK" if not problems else f"{len(problems)} problem(s)"))
    print("Dry run: nothing was sent to the backend.")
    return 0 if not problems else 1


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--api", default="http://localhost:8080/api")
    ap.add_argument("--suite", choices=["basic", "stages"], default="basic")
    ap.add_argument("--only", default="")
    ap.add_argument("--out", default="")
    ap.add_argument("--compare", default="", help="an earlier --out report of the stages suite; regressions fail the run")
    ap.add_argument("--dry-run", action="store_true", help="validate the stage cases and the rubric offline; sends nothing")
    args = ap.parse_args(argv)
    if args.dry_run:
        return dry_run()
    client = Client(args.api)
    return run_stages(client, args) if args.suite == "stages" else run_basic(client, args)


if __name__ == "__main__":
    sys.exit(main())
