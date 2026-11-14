# Tech

## Stack

- **Framework:** [MyGo](https://github.com/egoist/mygo) v0.9 — desktop apps
  in pure Go. Go owns the backend; a TypeScript/Vite frontend renders in
  the OS webview (WebView2 on Windows). `mygo generate` produces typed
  TS clients from bound Go services; `mygo build` emits a single binary
  plus an NSIS installer.
- **Audio output:** [oto/v3](https://github.com/ebitengine/oto) — WASAPI on
  Windows, no cgo.
- **Decoding:** go-mp3 (streaming, seekable) and go-audio/wav (in-memory;
  WAV files are small enough that streaming buys nothing). ID3 tags via
  dhowden/tag.
- **Spectrum:** hand-rolled radix-2 FFT (2048 points, Hamming window),
  32 log-spaced bands. No DSP dependency — the algorithm is ~90 lines and
  every frame is inspected by us.

## Non-negotiables

- **Pure Go only.** Any dependency that needs cgo breaks cross-compilation
  and the single-binary story. Check before adding a library.
- **Go ≥ 1.27.3** (MyGo requirement) and **bun ≥ 1.3** (mygo build). Both
  pinned in `mise.toml`; never install them outside mise.
- **One data path to the UI.** The engine pushes a single `tick` event at
  20 Hz carrying spectrum + transport state. The frontend never polls.
- **Mutex discipline.** The engine mutex guards player, source, and state;
  the service mutex guards playlist/index. Never call the engine while
  holding the service lock or vice versa (deadlock).
- **Audio bytes are 16-bit LE interleaved stereo @ 44.1 kHz** at the oto
  boundary; the `normalized` wrapper is the only place format conversion
  happens (mono→stereo, linear-interpolation resampling).

## Verification commands

Run through mise so tool versions match:

```bash
mise exec -- go vet ./...        # static checks (Go)
mise exec -- npm run typecheck   # tsc --noEmit (frontend)
mise exec -- npm run build       # full build: vite + embed + single exe
```

Windows specifics: paths may use `\`; use `os.PathSeparator` when joining.
Audio device access needs no elevation. WebView2 ships with Windows 11.
