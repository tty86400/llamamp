# Product

LLamAmp is a desktop music player for local MP3/WAV files, built as a
Kiroween 2026 "Resurrection" entry: it brings the MP3-era desktop player —
Winamp — back to life on modern machines.

## What it is

- A single pure-Go binary (no cgo, no web server install, no account).
- A classic Winamp-style skin: gold pinstripe title, green LED clock,
  bar-graph spectrum analyzer with falling peak caps, bronze bevel buttons.
- Local-first: the user picks a folder, the player scans it, done. No
  accounts, no cloud, no telemetry.

## What it is not

- Not a Winamp clone or fork: no skin file parsing, no plugin system, and
  no Winamp trademarks. The name and look are an homage.
- Not a streaming client. Everything is on disk.

## Who it is for

People with local music libraries who miss owning their playback: the
"dead" habit of playing files from your own disk. Judging criteria this
project targets: potential value (local-first players still matter),
implementation of Kiro (spec-driven engine work, hooks for CI-style
checks, steering for consistency), and quality/design (the retro skin).

## Success looks like

- Double-click a playlist row, audio plays within a second.
- The spectrum moves with the music in real time.
- `mygo build` produces one executable that runs on a clean Windows box.
