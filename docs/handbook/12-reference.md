# 12. Reference

[Handbook](README.md) · Previous: [Troubleshooting](11-troubleshooting.md)

## Vocabulary

| Term | Meaning |
|---|---|
| Provider | External service hosting a model |
| Connection | Saved LLM protocol, URL, credential source, and model defaults |
| Agent | A specific planning or review role with a schema-validated response |
| Engine | Image/video/vision model configuration exposed in Settings |
| Character bible | Stored appearance, personality, visual rules, and voice rules |
| Asset | Uploaded or generated file with metadata |
| Storyboard / 分镜图 | Contact sheet showing planned scene beats |
| RenderSpec | Structured request passed from application logic to a video adapter |
| Operation | Background planning/image/caption task tracked through `/ops` |
| Render job | Provider generation attempt, polled by the worker |
| Output | Downloaded video and associated thumbnail, QA, and caption data |

## Configuration map

| Variable | Role |
|---|---|
| `OPENCODE_GO_API_KEY` | Current default text-agent connection credential |
| `OPENROUTER_API_KEY` | Built-in OpenRouter credential; fixed endpoint https://openrouter.ai/api/v1 |
| `OPENROUTER_TEXT_MODEL` | OpenRouter text model default |
| `OPENROUTER_IMAGE_MODEL`, `OPENROUTER_VIDEO_MODEL` | OpenRouter media defaults, used when no saved model override exists |
| `DEFAULT_IMAGE_PROVIDER`, `DEFAULT_VIDEO_PROVIDER` | Initial media defaults; saved settings may override them, so confirm the effective route in the Create preflight panel |
| `DATABASE_URL`, `STORAGE_ROOT` | Local database and media locations |
| `POLL_INTERVAL_S`, `POLL_TIMEOUT_S` | Render polling timing |
| `LLM_TIMEOUT_S`, `LLM_MAX_RETRIES` | Text request timeout and validation retries |
| `OPENROUTER_VISION_MODEL` | Vision model default; timed transcription model is saved in Settings |
| `VITE_API_BASE` | Frontend API base override; configured for the frontend process/build |

Configuration is read from environment variables (the `environment:` block of `compose.yml` shows the ones the app uses). Saved database settings can override environment defaults.

## Navigation and API

| Screen | Route | Main API areas |
|---|---|---|
| Studio (story, scenes, editor) | `/` (the editor is `/?view=edit`) | `/graph`, `/chat`, `/scenes`, `/compositions` |
| My videos | `/render` | `/render-jobs`, `/outputs` |
| Library (characters, assets) | `/library` | `/characters`, `/assets`, `/style` |
| Voices | `/voices` | `/voice/state`, `/voice/profiles`, `/voice/samples` |
| Projects | `/projects` | `/projects` |
| Costs | `/costs` | `/costs` |
| Settings | `/settings` | `/settings/providers`, `/settings/app` |

Use the running Go backend for request schemas. Posting generation endpoints can incur provider usage. Health and model listing are not substitutes for an end-to-end generation check.

## Maintainer checks

From the repository root:

```sh
go test ./...
make build
```

From `frontend`:

```sh
npm run check
npm run build
```

### Assistant evaluation

The assistant is checked in two layers.

**Deterministic, in CI.** `go test ./internal/api` includes golden tests of what each chat turn is given and how its answer is post-processed, with a fake planner (no Codex or Claude CLI is started, nothing is paid for):

- `internal/api/chat_eval_test.go` builds eight projects from saved state (empty; brief only; characters without pictures; characters without voices; scenes ready to render; clips made, not edited; edited, not exported; exported) and sends one turn in manual, auto and Full auto. The turn must carry the stage map of `GET /stages`, the decision log (`decided_so_far`), the mode's rules, stay under 24 KB, and hold no key.
- `internal/api/chat_decision_matrix_test.go` runs assistant mode × the planner's decision × decision log × stage × the user's words through the same function every turn ends with.

**Live and scored, by hand only.** `scripts/assistant_eval.py --suite stages` sends scenario messages to a running backend, for the stage the open project is in (cases for other stages are skipped), in manual, auto and Full auto, and scores each reply: the right step for the stage, acting or asking as it should, no question about something already decided, stated assumptions, the user's language in a short plain reply, and no per-step buttons in Full auto.

```sh
python3 scripts/assistant_eval.py --suite stages --out run.json
python3 scripts/assistant_eval.py --suite stages --out next.json --compare run.json
python3 scripts/assistant_eval.py --suite stages --dry-run   # offline check of cases and rubric
python3 -m unittest scripts/test_assistant_eval.py           # offline unit tests
```

Each case is one or two turns of the AI session selected in Settings (your Claude Code or Codex sign-in), so a run uses real session usage; run it yourself, never in CI. The script only calls `/chat` (which prepares a button and runs nothing), reads `/stages`, `/decisions`, `/characters` and `/scenes`, and adds then removes the decisions a case seeds; it refuses any other request. It prints a table per case, scores per stage and per criterion, the overall score and the unnecessary-question rate, and `--compare` lists regressions against an earlier report. Run it on projects at different stages to cover them all. The default `--suite basic` runs the older everyday-request checks.
