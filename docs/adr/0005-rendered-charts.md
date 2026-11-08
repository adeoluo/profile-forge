# ADR 0005: Optionally render the charts to SVG images

## Status

Accepted.

## Context

The profile has three charts: the path flowchart, the ecosystem graph and the
language pie. As mermaid blocks, GitHub draws them client side in its own flat
theme, which cannot be changed from the Markdown (GitHub ignores most
configuration and strips styles). Every repository the profile links to embeds
its architecture diagrams as SVG files rendered with one shared mermaid config
(palette, drop shadows, fonts). The profile was the one place where the
diagrams looked different.

## Decision

- `--diagrams-dir DIR` switches the three charts to images. Generate writes
  `DIR/<chart>.mmd`, runs the renderer for each (by default
  `npx -y @mermaid-js/mermaid-cli`, configurable with `--mermaid-cmd`) with the
  optional `--mermaid-config`, and the template embeds
  `<img src="DIR/<chart>.svg">` instead of the block.
- Without the flag, output is unchanged: mermaid blocks, no external
  dependency.
- The chart sources are still validated with the same syntax checks as inline
  blocks before anything is rendered.
- `check --diagrams-dir DIR --readme README.md` compares the README and the
  `.mmd` files on disk with what generate would write. It does not re-render,
  so it stays fast and needs no browser; an image can only be stale if its
  source is.

## Consequences

- One visual style across the profile and every repository.
- Rendering needs Node and a headless browser (mermaid-cli downloads one).
  This is a generate-time cost only; readers load plain SVG files.
- The renderer is behind an `Env` function, so tests use a fake that writes a
  placeholder SVG and assert on the exact command line.
