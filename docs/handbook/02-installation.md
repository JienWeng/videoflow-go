# 2. Install from zero

[Handbook](README.md) · Previous: [How it works](01-how-it-works.md) · Next: [Projects](04-projects.md)

VideoFlow can run as a desktop app, in Docker, or from source. Pick one below.

## Choose how to run it

| You want to | Use |
|---|---|
| Just make videos on your computer | The desktop app (below) |
| Run it on a server or keep it running | Docker (below) |
| Change the code | Run from source (below) |

VideoFlow is a Go backend and a SvelteKit frontend. It needs FFmpeg (with FFprobe) to prepare media; the desktop app and the Docker image include it. Text planning uses a locally signed-in Claude Code or Codex session; video and image services are connected in **Settings** with your own keys. See [Provider routing](../PROVIDERS.md).

## 1. The desktop app

Download the latest release from the repository's Releases page, open it, and follow the [desktop quick start](../ELECTRON-START-HERE.txt). Nothing else needs installing.

## 2. Docker

```sh
git clone https://github.com/JienWeng/videoflow-go.git
cd videoflow-go
docker compose build
docker compose up -d --no-build
```

Open [http://localhost:8080](http://localhost:8080). Your projects and files live in the `videoflow-data` volume, and agent sign-ins in `videoflow-agent-auth`, so they survive restarts and rebuilds. Optional keys can be set as environment variables in `compose.yml` (for example `FISH_API_KEY`, `ELEVENLABS_API_KEY`), or entered later in Settings.

## 3. Run from source

You need Go (the version in `go.mod`), Node 24, and FFmpeg with FFprobe on your `PATH`.

```sh
git clone https://github.com/JienWeng/videoflow-go.git
cd videoflow-go
make build
./bin/videoflow-server          # the API, on port 8000 unless PORT is set
```

In a second terminal:

```sh
cd frontend
npm ci
npm run dev -- --host 127.0.0.1
```

Open [http://localhost:5173](http://localhost:5173).

## Captions and fonts

The first transcription downloads the chosen Whisper model; start with `tiny` or `base`. FFmpeg must include the `ass` filter (check with `ffmpeg -filters`). For Chinese captions, put `NotoSansCJKsc-Bold.otf` in `storage/fonts/`.

## Stop and restart

Wait for operations to finish. Docker: `docker compose stop`, then `docker compose up -d --no-build` to start again. From source: press Ctrl+C in each terminal. Back up the database and storage together before upgrades; see [Projects](04-projects.md). For startup errors, use [Troubleshooting](11-troubleshooting.md).
