#!/usr/bin/env python3
"""Score the Studio assistant against everyday requests on a live VideoFlow backend.

Usage: python3 scripts/assistant_eval.py [--api http://localhost:8080/api] [--only substring] [--out results.json]

Each case sends one chat message the way the app does and checks the button
(action) the assistant prepares, plus reply quality: the user's language, length,
and no internal jargon. Nothing runs: /chat only prepares a button. Messages use
the selected AI session (Claude Code or Codex), so each case is one AI turn.
The cases expect a project with one finished scene, a cat character and a person,
like the Giant Cat demo project; adjust EXPECT for other projects.
"""
import argparse, json, re, sys, time, urllib.error, urllib.request

JARGON = re.compile(r"\b(proposal|native session|pipeline|intent|action card|director)\b", re.I)
SPANISH = re.compile(r"[¿¡ñáéíóú]|\b(qué|cómo|quieres|estilo|video|gato)\b", re.I)

# (message, allowed actions, extra checks)
CASES = [
    ("hi", {"unknown"}, {}),
    ("what do I do next", {"unknown", "open_editor"}, {}),
    ("make a video about a dog who learns to surf", {"generate_script"}, {}),
    ("FOLLOWUP:english, 15 seconds, 2 scenes, cartoon style, for tiktok", {"generate_script"}, {"reply_has": "Got it", "brief_style": True}),
    ("make the cat bigger in the first scene", {"refine_scene"}, {}),
    ("add voice to my video", {"open_editor"}, {"panel": "voice"}),
    ("the cat should say hello", {"open_editor"}, {"panel": "voice"}),
    ("add music", {"open_editor"}, {"panel": "music"}),
    ("add subtitles", {"caption", "open_editor"}, {}),
    ("translate the subtitles to spanish", {"open_editor"}, {"panel": "captions"}),
    ("export the video as mp4", {"open_editor"}, {"panel": "export"}),
    ("render it", {"render_scene"}, {}),
    ("why did my video fail?", {"unknown", "open_editor", "retry_render"}, {}),
    ("delete everything", {"unknown"}, {}),
    ("make a picture of a tokyo street at night", {"generate_image"}, {}),
    ("change the cat's color to black", {"update_character"}, {"changes_has": "black"}),
    ("give the resident a red hat", {"update_character"}, {"changes_has": "hat"}),
    ("redo the cat's reference picture", {"generate_character_reference"}, {}),
    ("check if the storyboard matches the style", {"audit_storyboard"}, {}),
    ("make it longer", {"generate_scenes", "generate_script"}, {}),
    ("how much will this cost?", {"unknown"}, {}),
    ("asdfgh", {"unknown"}, {}),
    ("undo that", {"unknown", "open_editor"}, {}),
    ("hazme un video de un gato que baila", {"generate_script"}, {"spanish": True}),
    ("¿cómo exporto mi video?", {"open_editor"}, {"panel": "export", "spanish": True}),
]
MAX_WORDS = 120


def chat(api, body):
    req = urllib.request.Request(api + "/chat", data=json.dumps(body).encode(), headers={"content-type": "application/json"})
    return json.load(urllib.request.urlopen(req, timeout=600))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default="http://localhost:8080/api")
    ap.add_argument("--only", default="")
    ap.add_argument("--out", default="")
    args = ap.parse_args()
    brief, rows, passed = None, [], 0
    cases = [c for c in CASES if args.only.lower() in c[0].lower()]
    for message, allowed, checks in cases:
        body = {"message": message, "history": [], "selected": None}
        if message.startswith("FOLLOWUP:"):
            message = message[len("FOLLOWUP:"):]
            body = {"message": message, "history": [], "selected": None, "creation_brief": brief}
        started = time.time()
        problems = []
        try:
            intent = chat(args.api, body)["intent"]
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
        json.dump(rows, open(args.out, "w"), indent=1, ensure_ascii=False)
    sys.exit(0 if passed == len(cases) else 1)


if __name__ == "__main__":
    main()
