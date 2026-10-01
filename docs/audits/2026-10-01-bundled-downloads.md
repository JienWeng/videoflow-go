# Bundled macOS and Linux downloads

The second OpenRouter preview includes native FFmpeg and FFprobe, caption fonts for English/Chinese, license notices and the exact corresponding media sources inside each ZIP. No Go, Node, Python, FFmpeg or font installation is required by users. An internet connection and an OpenRouter key with credits remain required for AI calls. The app runs on loopback; there is no server deployment.

## Build and integrity

`scripts/build_media.py` builds pinned FFmpeg 9.0.2, libass, x264, FreeType, FriBidi and HarfBuzz sources on the target architecture. Linux media executables are statically linked. macOS executables may link only OS-provided libraries/frameworks. Fontconfig and other system font providers are disabled; the app supplies the packaged Noto font directory to the subtitles filter. No nonfree FFmpeg components are enabled.

Source downloads are locked by SHA256. The initial FFmpeg archive was also verified against the official release signature with fingerprint `FCF986EA15E6E293A5644F10B4322F04D67658D8`. The archive includes exact source tarballs, font files, license files, build instructions and the actual build script in `media-sources.zip`. FFmpeg is invoked as a separate executable, not linked into the Go app.

The packager requires all media tools, both fonts, all dependency/OFL notices and complete matching sources. It rejects stale or incomplete bundles. Native GitHub runners build macOS Intel/Apple Silicon and Linux AMD64/ARM64. Publication waits for all four platform jobs.

## Verification

The source/build and bundle checks cover missing tools, missing dependency licenses, corrupted source archives and stale builders. The Go regression verifies that the bundled caption font directory is supplied. Go race tests and vet run with the native bundled FFmpeg at the front of PATH.

Each extracted ZIP is launched with an otherwise empty PATH and isolated user data. The check verifies the browser interface, API and bundled FFmpeg discovery, and confirms no key/database/storage has been packaged. It also checks native linkage, encodes a real audiovisual fixture, burns English/Chinese captions using the bundled fonts, verifies the changed frame checksum, extracts WAV audio, and probes the resulting media.

Final platform check results are recorded in the GitHub release workflow and the task response. Paid OpenRouter generation is not performed by these packaging checks. The preview is not Apple Developer ID signed or notarized; macOS may require the system's Open Anyway flow for a trusted download.

References: [FFmpeg download and signature verification](https://ffmpeg.org/download.html), [FFmpeg distribution terms](https://ffmpeg.org/legal.html), [native GitHub runner platforms](https://github.com/actions/runner-images).
