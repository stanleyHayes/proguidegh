# Skill Observation Log

Observations captured during task-oriented work.

**Status key:** OPEN = not yet actioned | ACTIONED (YYYY-MM-DD) = skill updated/created | DECLINED (YYYY-MM-DD) = user decided not to pursue — resolved statuses always carry their resolution date

---

## 2026-08-14

### Observation 1: Fresh-server proof before visual acceptance

**Status:** OPEN
**Date:** 2026-08-14
**Session context:** Product-wide responsive redesign with production builds and browser QA
**Skill:** redesign-existing-projects
**Type:** open-source
**Phase/Area:** Visual verification

**Issue:** A browser screenshot appeared to show the new implementation but was served by a stale development process on the expected port. Build success alone did not prove that the browser was rendering the current output.

**Suggested improvement:** Add a visual-QA preflight that identifies existing listeners, verifies the served build fingerprint or a newly added selector in computed styles, and only then accepts screenshots as evidence.

**Principle:** Visual acceptance must prove that the rendered page comes from the current build before evaluating its quality.
