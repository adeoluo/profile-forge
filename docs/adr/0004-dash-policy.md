# ADR 0004: Reject dashes in config text, normalize them in fetched text

## Status

Accepted.

## Context

My writing style for public material excludes em dashes and en dashes. The
generated README combines text I wrote (the config) with text fetched from
GitHub (repository descriptions), and the rule should hold for the output as
a whole.

## Decision

- **Config text:** the raw file is scanned before decoding. Any U+2014,
  U+2013, or equivalent HTML entity is an error that lists line, column, and
  which character was found. The fix belongs in the source.
- **Fetched text:** descriptions are normalized rather than rejected, because
  they belong to other repositories and may be outside the config author's
  control at generation time. A dash surrounded by spaces becomes a comma
  and a space; an unspaced dash becomes a hyphen.
- **Output:** the final Markdown is scanned again and generation fails if any
  dash remains, for example from a custom template.

## Consequences

- The three layers make the guarantee independent of where text came from.
- Normalizing descriptions can produce slightly awkward punctuation; the
  better fix is to edit the repository description, after which the next
  generation picks it up.
