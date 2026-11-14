# LLamAmp 🦙🎛️

**LLamAmp resurrects the MP3-era desktop player for the modern desktop.**
One pure-Go binary, a classic Winamp-style skin, a real-time FFT spectrum
analyzer, and local-first playback of your own files. No accounts, no
cloud, no telemetry — your disk, your music, your machine.

> *"It really whips the llama's ass."* — an homage, never a clone.

![category](https://img.shields.io/badge/Kiroween%202026-Resurrection-9b30d9)
![stack](https://img.shields.io/badge/built%20with-MyGo%20%2B%20oto-00e0d0)

## Kiroween 2026 — Category: Resurrection

Winamp defined desktop music for a generation, then streamed away. LLamAmp
brings the *dead* habit back — playing files from your own disk — rebuilt
with today's innovations:

| Then (1997–2013)                      | Now (LLamAmp)                          |
| ------------------------------------- | -------------------------------------- |
| C++ installer, plugins, DLL hell      | one 10 MB pure-Go binary, zero deps    |
| cgo multimedia frameworks             | WASAPI via oto, cross-compilable       |
| hand-tuned assembly DSP              | auditable 90-line radix-2 FFT          |
| skin engine nobody wants to maintain  | one CSS file, no images                |
| install & pray                        | `mise exec -- npm run build` and run   |

## Features

- 🎵 Plays local **MP3** and **WAV** (ID3 tags for title/artist/album)
- 📊 Real-time spectrum analyzer: 32 log-spaced bands, classic falling
  peak caps, green→yellow→red LED ramp
- ⏱️ LED clock with elapsed/remaining toggle (click it, like the original)
- 📜 Playlist panel: pick a folder, double-click to play, auto-advance
  loops the list
- 🎚️ Seek bar and volume slider with drag support
- 🪟 Window position/size remembered across runs

## Quick start

### Prerequisites

- [mise](https://mise.jdx.dev) (manages go + bun versions for you)
- Windows 10/11 (WebView2 ships with Windows 11; Linux/macOS untested)

### Build from source

```bash
git clone https://github.com/tty86400/llamamp.git
cd llamamp
mise exec -- npm install
mise exec -- npm run build
```

Outputs under `build/windows-amd64/`:

- `llamamp.exe` — the whole player, 10.7 MB, nothing else to install
- `llamamp-setup-0.1.0-amd64.exe` — NSIS installer

### Use it

1. Launch — click **EJECT** and choose a folder with MP3/WAV files.
2. Double-click a playlist row (or press **▶**).
3. Watch the llama-level spectrum. That's it.

## Architecture

```
mp3/wav source ─▶ normalize (mono→stereo, resample 44.1k)
                        │  tapFn per frame
                        ▼
              ring buffer (mono downmix)
                        │  every 50 ms
              radix-2 FFT (2048, Hamming) ─▶ 32 log bands ─▶ tick event ─▶ canvas
                        ▼
               oto player (WASAPI, no cgo)
```

The Go backend owns audio and state; the TypeScript frontend renders one
`tick` event 20×/s — spectrum, transport, and track info travel together,
so the UI never polls. `mygo generate` produces the typed client
(`src/mygo.ts`) from the bound Go service.

## How Kiro was used

Kiro assets ship in the repo — the **`/.kiro` directory is part of the
deliverable** and is deliberately not gitignored.

### Spec-driven development

`.kiro/specs/spectrum-analyzer/` carries the full requirements → design →
tasks chain for the hardest subsystem:

- **requirements.md** — four user stories with measurable acceptance
  criteria (≤100 ms perceived lag, ≥20 fps, silence rests at zero).
- **design.md** — the decisions that mattered: why the tap sits at the
  playback boundary instead of the decoder (analysing *audible* audio),
  why mono downmix in the ring, why 2048/Hamming, why a hand-rolled FFT
  beats pulling gonum for one transform, and why dB mapping — not raw
  magnitudes — makes silence look silent.
- **tasks.md** — the checklist actually worked through, with parked
  follow-ups kept honest.

### Agent hooks (`.kiro/hooks/`)

- `go-vet-on-save.json` — `PostFileSave` on `*.go` runs `go vet ./...`;
  engine bugs surface at save time, not run time.
- `typecheck-on-stop.json` — `Stop` triggers `npm run typecheck` as a
  lightweight CI gate when an agent session ends.

### Steering (`.kiro/steering/`)

- **product.md** — what LLamAmp is and refuses to be (an homage, not a
  clone; no trademarks).
- **tech.md** — non-negotiables the agent must not violate: pure Go only,
  mise-managed tools, one data path to the UI, mutex discipline.
- **structure.md** — layering rules (`audio/` never imports `mygo`), and
  where each kind of change belongs.

`POWER.md` at the root onboards any agent session in one page.

### Vibe coding

The engine's state machine, the normalization chain, and the classic-skin
CSS came from conversational iteration — see the commit history for the
dialogue between "make silence rest at zero" specs and the DSP that
honoured them.

## Roadmap

- [ ] FLAC & OGG decoders (spec draft: add a `source` implementation)
- [ ] Oscilloscope mode (parked in the spectrum spec)
- [ ] Classic Winamp skin file (`.wsz`) reader
- [ ] 10-band EQ

## Development

```bash
mise exec -- go vet ./...        # static checks (Go)
mise exec -- npm run typecheck   # frontend types
mise exec -- npm run dev         # live-reload development
```

Tool versions are pinned in `mise.toml` (go 1.27.4, bun 1.3.5); never
install them outside mise.

## License

[MIT](LICENSE) — OSI-approved, as Kiroween requires.
