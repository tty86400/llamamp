# Spectrum Analyzer — Tasks

Legend: `[x]` done · `[ ]` open · `(d)` needs a human ear/eye to verify.

## 1. Tap plumbing

- [x] Add `tapFn` to `normalized`, invoked once per emitted stereo frame.
- [x] Implement `ringBuffer`: 2× window slots, mono downmix with clamping,
      wrap-around `latest(n)`.
- [x] Wire `Spectrum.Tap` as the engine's tap on load.

## 2. Analysis core

- [x] Precompute Hamming window and 32 log-spaced band→bin edges in
      `NewSpectrum` (edges monotonically increasing, every band ≥ 1 bin).
- [x] Implement iterative radix-2 FFT with bit-reversal permutation.
- [x] Map averaged bin magnitudes through dB [-50, +25] → [0,1] with ^1.2.
- [x] Falling peak caps: instant rise, −0.03/frame decay, floor at 0.

## 3. Delivery to UI

- [x] Single `tick` event at 20 Hz carrying bands + peaks + transport
      state; frontend subscribes once and never polls.
- [x] Canvas renderer: 16 lit rows bottom-up, green→yellow→red ramp,
      white 2 px peak caps, pixel-crisp integer rects.

## 4. Verification

- [x] `go vet ./...` clean; `npm run typecheck` clean.
- [x] `mygo build` produces the single binary; smoke-run ≥ 6 s alive.
- [x] (d) 440 Hz tone lights only low-mid bands.
- [x] (d) Pause freezes bars; near-silence rests near zero.
- [x] (d) No audible lag between hearing a hit and seeing the bars move.

## 5. Follow-ups (parked)

- [ ] Oscilloscope mode toggle.
- [ ] Stereo split view.
- [ ] Peak-cap fall speed tunable per skin.
