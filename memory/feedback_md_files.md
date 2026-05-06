---
name: Read CLAUDE.md and EXPERIENCES.md before any task
description: Always read area-specific CLAUDE.md and EXPERIENCES.md before making code or config changes
type: feedback
---

Before starting any task in this repo, read:
1. The area-specific CLAUDE.md (`core/CLAUDE.md`, `support/CLAUDE.md`, or `experiments/CLAUDE_EXPERIMENTS.md`) depending on where the task is
2. `EXPERIENCES.md` — especially the pre-flight checklist at the bottom

**Why:** The user pointed out that I was reading only the files being modified, not the governance documents. This risks missing cross-cutting prohibitions, the Kafka partition-reader invariant, the HTTP boundary rule, and known failure modes documented in EXPERIENCES.md.

**How to apply:** Do this even for documentation and config changes. For Go code changes it is mandatory. The root `CLAUDE.md` makes this explicit — follow it.
