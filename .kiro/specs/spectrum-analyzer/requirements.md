# Spectrum Analyzer — Requirements

## Overview

A real-time visualizer that turns the currently playing audio into the
classic Winamp bar-graph spectrum: 32 log-spaced frequency bands, lit
cell-by-cell from bottom to top, with falling peak caps.

## User stories

1. **As a listener, I see the music's spectrum move in real time** so the
   player feels alive.
   - The visualizer reflects what is *currently audible*, with no visible
     lag above ~100 ms.
   - Update rate is ≥ 20 frames per second.
2. **As a listener, I see frequency content distributed sensibly** so bass
   sits left and treble sits right.
   - Bands are log-spaced from ~50 Hz to ~16 kHz.
   - Loud music never pushes every band to full height; the display maps
     dB ranges, not raw magnitudes.
3. **As a nostalgic user, I see the classic peak caps** so the visualizer
   matches the original.
   - Each band has a peak marker that rises instantly and falls gradually
     (≈ 3% per frame).
4. **As a user on a quiet track, I see the bars rest** so idle silence
   looks idle.
   - Near-silence produces near-zero bars, not noise-floor flicker.

## Acceptance criteria

- [ ] Spectrum updates at the 50 ms tick cadence with the rest of the UI.
- [ ] Play a 440 Hz tone: only low-mid bands light; play pink noise: all
      bands respond proportionally.
- [ ] Pause: bars freeze (no processing of silence into fake motion).
- [ ] CPU cost of the analyzer stays under 5% of one core at 44.1 kHz.
- [ ] Works identically for mp3 and wav sources (post-normalization tap).

## Non-goals

- Stereo (L/R split) visualization.
- Skin/configurable band count — 32 is the baseline.
- Oscilloscope mode (the original's second view).
