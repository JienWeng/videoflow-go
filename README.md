## Download and use

For a beginner, use the [GitHub downloads](https://github.com/JienWeng/videoflow-go/releases): extract the ZIP for your computer and open **Start VideoFlow**. The app opens in your browser. Add your OpenRouter key in Settings; AI generation needs OpenRouter credits. No Go or Node installation is required.

The preview is unsigned. FFmpeg, FFprobe and a caption font are included, so transcription, video QA and caption burning need no separate dependency installation. The exact media sources and licenses are included in each archive. See [beginner instructions](docs/QUICKSTART.txt).

To build a download, build the frontend, run `python3 scripts/build_media.py` on the target platform, then run `python3 scripts/package.py --target <platform-architecture>`. Media build tools are documented in the builder; users do not need them. A tagged release runs checks and publishes four macOS/Linux archives with checksums. Start a local combined server with `bin/videoflow-server --desktop --ui-dir frontend/build`; add `--no-browser` for a service. The server binds to localhost by default; `--host` explicitly sets another interface.

# VideoFlow Go

A streamlined, high-performance, single-binary AI video studio backend built in **Go (Golang)** with an interactive **SvelteKit** frontend.

Forked and optimized from [videoflow](https://github.com/JienWeng/videoflow) to deliver **30x lower memory consumption**, instant database access with SQLite WAL mode, and seamless zero-dependency deployment.

---

## Performance Highlights

* **Single 17 MB Executable**: Contains the entire REST API, SSE streaming broker, background worker pool, media runner, and SQLite database engine.
* **~12.9 MB Idle RAM**: Over 30x less memory than the Python/FastAPI runtime (~400MB–1.2GB).
* **Instant Cold Boot**: Starts in `< 50 ms`.
* **Zero Dependency Setup**: No Python, no UV, no virtualenvs, no C++ compilation required.

---

## Architecture

```text
┌────────────────────────────────────────────────────────┐
│           Single Go Binary (videoflow-server)          │
│                                                        │
│  [ REST API Router ]      [ SSE Event Broker ]         │
│         │                          │                   │
│         ▼                          ▼                   │
│  [ Embedded SQLite ]      [ Background Worker Pool ]   │
│  (WAL mode, in-process)   (Goroutines, no Redis/Celery)│
│         │                          │                   │
│         ▼                          ▼                   │
│  [ Static Storage Server] [ FFmpeg Process Runner ]    │
└────────────────────────────────────────────────────────┘
                           ▲
                           │ HTTP / SSE
                           ▼
              [ SvelteKit Frontend UI ]
```

---

## Quick Start

### 1. Build and Run the Go Backend

```bash
make build
./bin/videoflow-server
```
The server will start listening on `http://127.0.0.1:8000`.

### 2. Run the SvelteKit Frontend

In a second terminal:

```bash
cd frontend
npm install
npm run dev -- --host 127.0.0.1
```

Open [http://localhost:5173](http://localhost:5173) in your browser.

---

## Cross-Compilation

To produce standalone executables for other operating systems:

```bash
# Build for all platforms (Linux amd64/arm64, macOS Apple Silicon/Intel, Windows)
make build-all
```

Outputs will be saved in `bin/`:
* `bin/videoflow-server-linux-amd64`
* `bin/videoflow-server-darwin-arm64` (macOS Apple Silicon)
* `bin/videoflow-server-darwin-amd64` (macOS Intel)
* `bin/videoflow-server-windows-amd64.exe` (Windows)

---

## Configuration

Set environment variables in `.env` or in your shell:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `8000` | HTTP API listen port |
| `STORAGE_ROOT` | `storage` | Path to store generated videos and images |
| `DATABASE_PATH` | `db.sqlite` | SQLite database file |
| `OPENROUTER_API_KEY` | `""` | OpenRouter key for all AI generation and analysis |
| `OPENROUTER_TEXT_MODEL` | `openai/gpt-4o-mini` | Default text agent model |
| `OPENROUTER_VISION_MODEL` | `qwen/qwen3-vl-30b-a3b-instruct` | Default vision model |
| `OPENROUTER_IMAGE_MODEL` | `openai/gpt-image-2` | Default image/reference model |
| `OPENROUTER_VIDEO_MODEL` | `google/veo-3.1-lite` | Default video model |

---

All AI work uses OpenRouter. Save the key in Settings or set `OPENROUTER_API_KEY`; saved settings take effect without restarting. See [OpenRouter setup](handbook/03-providers.md) for model catalogs, paid checks, and FFmpeg requirements.

## User Handbook

Explore the complete guide to using VideoFlow Go in the [`handbook/`](handbook/README.md) directory:

| Part | Description |
|---|---|
| [1. How VideoFlow works](handbook/01-how-it-works.md) | Core concepts, workflow philosophy, and automated steps |
| [2. Install from zero](handbook/02-installation.md) | Prerequisites, launch, health checks, and shutdown |
| [3. Configure providers](handbook/03-providers.md) | API keys, model setup, agents, and media engine selection |
| [4. Projects](handbook/04-projects.md) | Creating, switching, exporting, importing, and backing up projects |
| [5. First video](handbook/05-first-video.md) | Step-by-step workflow starting from an initial text concept |
| [6. Characters and assets](handbook/06-characters-assets.md) | Setting up character identities, reference images, and styles |
| [7. Scenes and shots](handbook/07-scenes-shots.md) | Writing scripts, building scenes, and managing shot breakdowns |
| [8. Studio](handbook/08-studio.md) | Working with the interactive canvas and chat assistant |
| [9. Renders and outputs](handbook/09-renders.md) | Handling render jobs, quality checks, retries, and downloads |
| [10. Captions and editor](handbook/10-captions-editor.md) | Speech transcription, timing adjustments, and burnt-in captions |
| [11. Troubleshooting](handbook/11-troubleshooting.md) | Diagnostics and resolution for common setup & render issues |
| [12. Reference](handbook/12-reference.md) | Technical reference, route map, environment variables, and CLI commands |

