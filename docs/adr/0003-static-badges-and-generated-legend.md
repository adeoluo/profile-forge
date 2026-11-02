# ADR 0003: Static badges and a legend generated from category colors

## Status

Accepted.

## Context

Profile READMEs commonly use dynamic shields.io badges (follower counts,
release versions) and hand-colored mermaid diagrams. Two problems follow:

- Dynamic badges are fetched by shields.io from the GitHub API at view time.
  They are rate limited and cached by GitHub's image proxy, and in practice
  some render as errors or stale values.
- Diagram colors are chosen by hand in several places. When a category
  changes color in one diagram and not another, or a color has no legend
  entry, readers have to guess what it means.

## Decision

- Use only static badge URLs (`https://img.shields.io/badge/<label>-<message>-<color>`).
  Every number shown (public repositories, flagships, releases, top language
  share, release tags) is computed at generation time.
- Each category owns its colors. The path chart, the ecosystem chart, the
  legend swatches, and release badge colors are all generated from that one
  definition. The legend lists every category with its color name, a swatch
  badge rendered in the fill color, the description, and the repository count.

## Consequences

- Numbers are as fresh as the last generation, not the last page view. The
  `check --readme` command exists to catch staleness in CI or before a push.
- Static badge text must be escaped (`-` to `--`, `_` to `__`, then URL path
  escaping); `StaticBadge` has table tests for this.
- `--check-links` can verify every badge renders, since static URLs are
  deterministic.
