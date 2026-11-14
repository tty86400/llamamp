// LLamAmp UI. The Go engine pushes UIState 20×/s over one event; this file
// renders it and forwards user actions back through the generated client.
import { Service, events, type TrackMeta, type UIState } from "./mygo";

const $ = <T extends HTMLElement>(selector: string) => document.querySelector<T>(selector)!;

// ---------- DOM handles ----------
const els = {
  title: $<HTMLDivElement>("#title"),
  time: $<HTMLDivElement>("#time"),
  kbps: $<HTMLDivElement>("#kbps"),
  khz: $<HTMLDivElement>("#khz"),
  mode: $<HTMLDivElement>("#mode"),
  seek: $<HTMLDivElement>("#seek"),
  seekFill: $<HTMLDivElement>("#seekFill"),
  seekKnob: $<HTMLDivElement>("#seekKnob"),
  volume: $<HTMLDivElement>("#volume"),
  volumeFill: $<HTMLDivElement>("#volumeFill"),
  volumeKnob: $<HTMLDivElement>("#volumeKnob"),
  volLabel: $<HTMLSpanElement>("#volLabel"),
  statusState: $<HTMLSpanElement>("#statusState"),
  statusTrack: $<HTMLSpanElement>("#statusTrack"),
  plList: $<HTMLDivElement>("#plList"),
  plCount: $<HTMLSpanElement>("#plCount"),
  spectrum: $<HTMLCanvasElement>("#spectrum"),
};

// ---------- state ----------
let tracks: TrackMeta[] = [];
let shownDuration = 0;
let shownPosition = 0;
let elapsedMode = true; // LED clock: elapsed vs remaining, like Winamp

// ---------- helpers ----------
function fmtTime(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) sec = 0;
  const s = Math.floor(sec) % 60;
  const m = Math.floor(sec / 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function setTitle(text: string) {
  els.title.textContent = text || "LLamAmp";
  els.title.classList.remove("scroll");
  if (els.title.scrollWidth > els.title.clientWidth + 2) {
    els.title.classList.add("scroll");
  }
}

// ---------- playlist ----------
function renderPlaylist(current: number, state: string) {
  els.plList.replaceChildren(
    ...tracks.map((t, i) => {
      const row = document.createElement("div");
      row.className = "pl-row" + (i === current && state !== "stopped" ? " current" : "");
      const num = document.createElement("span");
      num.className = "pl-num";
      num.textContent = `${i + 1}.`;
      const name = document.createElement("span");
      name.className = "pl-name";
      name.textContent = t.artist ? `${t.artist} — ${t.title}` : t.title;
      const len = document.createElement("span");
      len.className = "pl-len";
      len.textContent = t.duration > 0 ? fmtTime(t.duration) : "--:--";
      row.append(num, name, len);
      row.addEventListener("dblclick", () => playIndex(i));
      return row;
    }),
  );
  els.plCount.textContent = tracks.length > 0 ? `· ${tracks.length} item(s)` : "";
}

async function playIndex(i: number) {
  if (i < 0 || i >= tracks.length) return;
  const status = await Service.load(i);
  if (status.error) return;
  await Service.play();
}

// ---------- transport ----------
$("#play").addEventListener("click", () => Service.play());
$("#pause").addEventListener("click", () => Service.pause());
$("#stop").addEventListener("click", () => Service.stop());
$("#prev").addEventListener("click", () => hop(-1));
$("#next").addEventListener("click", () => hop(1));
$("#eject").addEventListener("click", pickFolder);

async function hop(delta: number) {
  const rows = els.plList.children.length;
  if (rows === 0) return;
  // wrap around like the original
  const from = tracks.findIndex((_, i) => els.plList.children[i]?.classList.contains("current"));
  const target = ((from === -1 ? 0 : from + delta) + rows) % rows;
  await playIndex(target);
}

async function pickFolder() {
  const reply = await Service.pickFolder();
  tracks = reply.tracks ?? [];
  renderPlaylist(-1, "stopped");
  els.statusTrack.textContent = tracks.length > 0 ? `${tracks.length} track(s) ready` : "no audio files found";
}

// ---------- sliders ----------
function bindSlider(el: HTMLDivElement, onFrac: (frac: number) => void) {
  const apply = (clientX: number) => {
    const rect = el.getBoundingClientRect();
    const frac = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
    onFrac(frac);
  };
  const down = (e: MouseEvent) => {
    apply(e.clientX);
    const move = (ev: MouseEvent) => apply(ev.clientX);
    const up = () => {
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };
  el.addEventListener("mousedown", down);
}

bindSlider(els.seek, async (frac) => {
  if (shownDuration <= 0) return;
  shownPosition = frac * shownDuration;
  drawSeek();
  const st = await Service.seek(frac * shownDuration);
  if (typeof st.position === "number") shownPosition = st.position;
});

let volumePct = 75;
bindSlider(els.volume, (frac) => {
  volumePct = Math.round(frac * 100);
  drawVolume();
  void Service.setVolume(volumePct);
});

function drawSeek() {
  const frac = shownDuration > 0 ? Math.min(1, shownPosition / shownDuration) : 0;
  els.seekFill.style.width = `${frac * 100}%`;
  els.seekKnob.style.left = `calc(${frac * 100}% - ${Math.round(frac * 12)}px)`;
}

function drawVolume() {
  const frac = volumePct / 100;
  els.volumeFill.style.width = `${frac * 100}%`;
  els.volumeKnob.style.left = `calc(${frac * 100}% - ${Math.round(frac * 12)}px)`;
  els.volLabel.textContent = String(volumePct);
}

drawVolume();

// LED clock click toggles elapsed/remaining
els.time.addEventListener("click", () => {
  elapsedMode = !elapsedMode;
});

// ---------- spectrum renderer ----------
const ctx = els.spectrum.getContext("2d")!;
const ROWS = 16;
const GAP = 2;

// classic ramp: green → yellow → red by row
function barColor(row: number): string {
  const t = row / (ROWS - 1);
  if (t < 0.5) {
    const k = t / 0.5;
    return `rgb(${Math.round(30 + 180 * k)}, ${Math.round(190 + 40 * k)}, 40)`;
  }
  const k = (t - 0.5) / 0.5;
  return `rgb(${Math.round(210 + 45 * k)}, ${Math.round(230 - 180 * k)}, 40)`;
}

function drawSpectrum(bands: number[], peaks: number[]) {
  const w = els.spectrum.width;
  const h = els.spectrum.height;
  ctx.clearRect(0, 0, w, h);
  const n = bands.length;
  const cellH = (h - GAP * (ROWS - 1)) / ROWS;
  const colW = w / n;
  const barW = Math.max(3, Math.floor(colW) - 3);

  for (let b = 0; b < n; b++) {
    const lit = Math.round((bands[b] ?? 0) * ROWS);
    const x = Math.round(b * colW) + 1;
    for (let r = 0; r < lit; r++) {
      // rows paint bottom-up
      const y = h - (r + 1) * (cellH + GAP) + GAP;
      ctx.fillStyle = barColor(r);
      ctx.fillRect(x, y, barW, cellH);
    }
    // falling peak cap
    const capRow = Math.round((peaks[b] ?? 0) * ROWS);
    if (capRow > 0) {
      const y = h - capRow * (cellH + GAP);
      ctx.fillStyle = "#e8f4ff";
      ctx.fillRect(x, y, barW, 2);
    }
  }
}

// ---------- tick event: the single data path ----------
events.tick.on((s: UIState) => {
  shownPosition = s.position;
  shownDuration = s.duration;

  // LED clock: elapsed or remaining (negative style), Winamp style
  els.time.textContent = elapsedMode ? fmtTime(s.position) : "-" + fmtTime(s.duration - s.position);

  // title: artist — title, else file title
  const t = s.track;
  if (t && t.id) {
    setTitle(t.artist ? `${t.artist} — ${t.title}` : t.title);
  }
  els.statusTrack.textContent = t && t.id ? t.path : "no folder loaded";

  // state-dependent accents
  els.statusState.textContent = s.state.toUpperCase();
  $("#play").classList.toggle("active", s.state === "playing");
  $("#pause").classList.toggle("active", s.state === "paused");
  $("#stop").classList.toggle("active", s.state === "stopped");

  drawSeek();
  drawSpectrum(s.bands, s.peaks);
  highlightPlaylist(s.index, s.state);
});

let lastCurrent = -1;
function highlightPlaylist(index: number, state: string) {
  if (index === lastCurrent) return;
  lastCurrent = index;
  renderPlaylist(index, state);
}

// ---------- boot ----------
const initial = await Service.snapshot();
shownPosition = initial.position;
shownDuration = initial.duration;
volumePct = 75;
setTitle("LLamAmp — it really whips the llama's ass");

// try restoring a playlist folder if one was used before? Keep it simple:
// the user picks a folder with EJECT, Winamp-style.
renderPlaylist(-1, "stopped");
