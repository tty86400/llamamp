# POWER — LLamAmp agent onboarding

You are working on LLamAmp, a Kiroween 2026 "Resurrection" entry: the
MP3-era desktop player rebuilt as a single pure-Go binary with a classic
Winamp-style skin.

## Where to look first

1. `.kiro/steering/product.md` — what this is and is not.
2. `.kiro/steering/tech.md` — stack, non-negotiables, verification commands.
3. `.kiro/steering/structure.md` — layering rules and where things go.

## Non-negotiables (violating these will fail review)

- Pure Go only: no cgo dependencies, ever. The single-binary story breaks.
- Tools come from mise (`mise.toml` pins go 1.27.4 + bun 1.3.5): run
  builds and checks with `mise exec -- <cmd>`.
- `src/mygo.ts` is generated — change Go doc comments and re-run
  `mygo generate`, never hand-edit.
- The frontend receives everything through one `tick` event; do not add
  polling or a second event channel.
- `.kiro/` is part of the deliverable (hackathon rules): never gitignore
  it or prune it when tidying.

## How to verify your work

```bash
mise exec -- go vet ./... && mise exec -- npm run typecheck && mise exec -- npm run build
```

Then smoke-run `build/windows-amd64/llamamp.exe` for a few seconds.

## When adding features

Write the spec first under `.kiro/specs/<feature>/` (requirements,
design, tasks) and keep `tasks.md` checkboxes honest as you go.
