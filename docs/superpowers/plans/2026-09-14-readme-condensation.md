# README Condensation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the operational manual in the README with a concise repository entry point.

**Architecture:** `README.md` remains the single project landing page. It retains the repository purpose, recipe example, family coverage, layout, a contribution link with local checks, and legal/versioning links. Detailed operations remain in existing focused documentation and workflows.

**Tech Stack:** Markdown, existing Go and `dispelnet` command documentation.

## Global Constraints

- Do not change recipes, captures, workflows, tools, or release behavior.
- Retain valid links to `docs/ADDING_SUPPORT.md`, `docs/VERSIONING.md`, `LICENSE`, and `NOTICE`.
- Keep the README self-contained for understanding the repository, but concise.

---

### Task 1: Rewrite and verify the README

**Files:**
- Modify: `README.md`
- Reference: `docs/ADDING_SUPPORT.md`, `docs/VERSIONING.md`, `LICENSE`, `NOTICE`

**Interfaces:**
- Consumes: the repository layout and command names already documented in the current README.
- Produces: a concise Markdown landing page with working local links.

- [x] **Step 1: Replace the README content**

Keep these sections, in this order: title and purpose, recipe example, supported families table, repository layout, contributing pointer with the three validation commands, and licence/versioning links. Remove explanations of recipe restrictions, CI topology, provenance policy, releases, release verification, and maintainer settings.

- [x] **Step 2: Render-check Markdown and links**

Run: `rg -n '\]\((docs/ADDING_SUPPORT\.md|docs/VERSIONING\.md|LICENSE|NOTICE)\)' README.md && test -f docs/ADDING_SUPPORT.md && test -f docs/VERSIONING.md && test -f LICENSE && test -f NOTICE`

Expected: all four links appear in `README.md`, and each target exists.

- [x] **Step 3: Review the concise page**

Run: `sed -n '1,220p' README.md && ! rg -n '^## (What CI checks|Provenance|Releases|Repository settings)' README.md`

Expected: the page contains only the approved landing-page material and no CI, provenance, release-verification, or maintainer-settings sections.

- [x] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: condense repository README"
```
