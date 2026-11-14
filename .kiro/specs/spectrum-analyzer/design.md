# Spectrum Analyzer — Design

## Context

The audio chain is: `source` (mp3/wav decoder) → `normalized` (mono→stereo
+ resample to 44.1 kHz) → oto player. The analyzer taps the stream where
bytes become audible — at the output of `normalized`, per emitted frame.

## Architecture

```
normalized.Read() ──tapFn(l, r)──▶ ringBuffer (mono downmix, 4096 slots)
                                        │
                          Spectrum.Analyse() every 50 ms (push loop)
                                        │
                              32 bands + 32 peaks ──▶ tick event ──▶ canvas
```

## Key decisions

### Tap at the playback boundary, not the decoder

A decoder-side tap would analyse audio that hasn't reached the speakers
(decoding runs ahead). The tap fires from `normalized.Read` as oto consumes
frames, so the ring buffer always holds *now-playing* audio. This is why
`normalized` carries a `tapFn func(left, right int16)` field.

### Mono downmix in the ring

Stereo adds no visual information at bar granularity; mixing halves the
data. `ringBuffer.write` clamps (l+r) into 16-bit space to avoid overflow.

### 2048-point window, Hamming

2048 samples ≈ 46 ms at 44.1 kHz — short enough that the bar motion tracks
the beat, long enough for ~21 Hz frequency resolution. The Hamming window
(-26 dB sidelobes) keeps a loud bass bin from bleeding into its neighbors.

### Hand-rolled radix-2 FFT

gonum's `dsp/fourier` pulls a large dependency for one fixed-size
transform we call at 20 Hz. The iterative radix-2 implementation is ~90
lines, allocation-light (one output slice per call), and auditable. Size
is a compile-time constant (`fftSize`), which removes runtime validation.

### dB mapping, not magnitude

Raw FFT magnitudes are perceptually wrong (music is heavily bass-weighted
in linear magnitude). Each band averages its bin magnitudes, converts to
dB (`20·log10`), and maps [-50 dB, +25 dB] onto [0, 1] with a `^1.2`
curve for the upper register. Silence then genuinely rests at zero.

### Log-spaced band edges, precomputed

Band edges are computed once at `NewSpectrum` from `fMin·(fMax/fMin)^(i/32)`
and snapped to FFT bins, guaranteeing every band covers ≥ 1 bin. The
window, edges, and peak array live on `Spectrum` — the hot path allocates
only the FFT output and the sample copy.

## Concurrency

- `ringBuffer` has its own mutex: the playback goroutine writes, the push
  loop reads. Locks are held for O(n) memcpy only.
- `Spectrum.peaks` is touched only inside `Analyse` (push-loop goroutine),
  so it needs no lock.
- The engine mutex is *not* involved in analysis; the tap and ring operate
  independently, so audio never blocks on UI pushes.

## Risks / trade-offs

- **Linear-interpolation resampling** slightly aliases above 20 kHz for
  48 kHz sources — acceptable for a visualizer and for playback at this
  project's fidelity bar.
- **4096-slot ring (2× window)** tolerates read/write races across tick
  boundaries; `latest(n)` wraps correctly even when the writer laps the
  reader by less than a window.
- **20 Hz polling of `Analyse`** does one FFT per tick ≈ 2048·log₂(2048)
  butterflies ≈ trivially cheap; the 5% CPU budget is unachievable to
  exceed with this algorithm.
