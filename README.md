# VideoFlow Go

VideoFlow is a desktop app for creating AI-assisted videos. It includes the app, local engine, and media tools; you do not need to install Go, Node.js, Python, or FFmpeg to use it.

## Use the app

1. [Download VideoFlow](https://github.com/JienWeng/videoflow-go/releases) for your computer.
2. Install and open it: use the DMG on macOS or DEB on Ubuntu/Debian. For other Linux distributions, use the AppImage or portable archive.
3. In **Settings**, add an [OpenRouter API key](https://openrouter.ai/keys). AI generation requires internet access and account credits.
4. Create a project and start making a video.

New to VideoFlow? Follow the [desktop quick start](docs/DESKTOP-QUICKSTART.txt), then use the [VideoFlow handbook](docs/handbook/README.md) for projects, providers, scenes, renders, and troubleshooting.

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

- [Browser launcher quick start](docs/QUICKSTART.txt) — for the legacy native ZIP launcher.
- [Release notes](docs/release-notes.md)
