# Loop Localization Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Parameterize the hive loop so all environment-specific values come from `loop/config.env`, removing all hardcoded paths, external service calls, and upstream push commands.

**Architecture:** A new `loop/config.env` file becomes the single source of truth for deployment-specific values (git remote, org, repo paths, feature flags). All scripts source it. All prompts reference it. No hardcoded paths remain in prompts or scripts.

**Tech Stack:** Bash (scripts), plain text (prompts), markdown (state.md, CLAUDE.md)

**Spec:** `docs/superpowers/specs/2026-04-06-loop-localization-design.md` (v1.1.0)

**Git rules:**
- Branch: `feat/loop-localization` on `transpara-ai` remote
- Never commit to `main`, never push to `origin` (lovyou-ai upstream)
- PR to `transpara-ai/hive` when complete
