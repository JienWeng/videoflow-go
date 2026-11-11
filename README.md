# VideoFlow Go

VideoFlow is a desktop app for creating AI-assisted videos. It includes the app, local engine, and media tools; you do not need to install Go, Node.js, Python, or FFmpeg to use it.

## Use the app

1. Download the [repaired browser-launcher preview](https://github.com/JienWeng/videoflow-go/releases/tag/v0.3.2-browser-preview.1) for your Mac or Linux processor.
2. Extract the entire ZIP and open **Start VideoFlow**. Keep its terminal open; the app opens in your browser at `http://127.0.0.1:8000`. Older Electron DMG/DEB packages remain under [all releases](https://github.com/JienWeng/videoflow-go/releases).
3. In **Settings**, configure the keys required by your version. Current source uses fal.ai for new image/video routes and local Claude Code/Codex sessions for text and OpenRouter for vision/transcription; direct BytePlus video is selectable. See [provider setup](docs/PROVIDERS.md). Older v0.3.2 packages use OpenRouter only. AI generation requires internet access and provider credits.
4. Create a project and start making a video.

New to VideoFlow? Follow the [desktop quick start](docs/DESKTOP-QUICKSTART.txt), then use the [VideoFlow handbook](docs/handbook/README.md) for projects, providers, scenes, renders, and troubleshooting.

## Installation and later testing

[Download llms.txt](https://raw.githubusercontent.com/JienWeng/videoflow-go/main/llms.txt) for local installation, updates, data storage and a staged test checklist. Future native ZIPs include the guide; it is documentation, not an installer.

The v0.3.2 browser-launcher preview includes the November fixes and passed native checks on all four platforms. Older Electron releases predate those fixes. To run from source instead, use `docker compose up --build -d` and open `http://127.0.0.1:8080`. See the [character-to-export workflow](docs/BROWSER-WORKFLOW.md).

## Run from source

For contributors and local development:

```sh
make build
./bin/videoflow-server
```

In another terminal, start the frontend:

```sh
cd frontend
npm install
npm run dev -- --host 127.0.0.1
```

Open [http://localhost:5173](http://localhost:5173). For Docker web deployment or desktop packaging, see [build and deployment instructions](docs/ELECTRON-AND-WEB.md).

## Other guides

- [Local sessions, character consistency, voices, script captions and composition editing](docs/LOCAL-DIRECTOR.md)

- [Browser launcher quick start](docs/QUICKSTART.txt) — for the legacy native ZIP launcher.
- [Release notes](docs/release-notes.md)
