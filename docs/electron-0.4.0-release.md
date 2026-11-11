VideoFlow 0.4.0 brings the latest Studio and Settings redesign to the downloadable Electron app.

- Canvas + contextual Details/Chat, inline character and identity editing, visible AI session selection, and a focused mobile/browser layout.
- Settings grouped into General, AI connections and Advanced; simpler navigation throughout the app.
- Native Codex/Claude sessions for text planning, fal.ai image/video routing, selectable BytePlus video, saved character voices, script captions and optional HyperFrames composition workflow.
- Fixes for stale Cancel values, wrong-scope chat actions and delayed responses after session switching. Desktop startup now finds standard user CLI install locations.

Choose **arm64** for Apple Silicon Macs and **x64** for Intel Macs. On Linux choose **amd64/x64** for Intel/AMD or **arm64** for ARM. Quit the older app before replacing it; projects remain in the existing user data folder. Windows installers are not included.

**Mac downloads are ad-hoc previews, not Apple-notarized releases.** macOS may require System Settings → Privacy & Security → Open Anyway. Managed Macs may prevent that approval. The package does not disable Gatekeeper. Linux includes DEB, AppImage and tar.gz downloads. SHA256SUMS.txt accompanies the installers.

The app includes its current Go backend, interface, FFmpeg/FFprobe and fonts. Media binaries and their corresponding source/license bundle are reused only after checksum verification and a check that the locked media build sources are unchanged; the application backend and frontend are rebuilt from this release commit.

For AI planning, install and sign in to the official Codex or Claude Code CLI on the computer running the app. These optional account tools are not bundled. Select/create a session in Studio. Add media provider credentials under Settings → AI connections. Paid generation still requires explicit Run/review controls; automated release checks do not spend generation credits. HyperFrames additionally needs its documented optional runtime.
