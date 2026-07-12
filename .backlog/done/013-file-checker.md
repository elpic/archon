---
title: "File Checker — Existence and Content Checks"
status: done
priority: high
sprint: 1
labels: [feature, rules]
created: "2026-07-12"
completed: "2026-07-12"
pr: "https://github.com/elpic/archon/pull/9"
---

# File Checker — Existence and Content Checks

Implement file existence and basic content checks.

## Problem

Many rules are about file existence: "must have a Dockerfile", "must have
CI workflow", "must not have `.env` in repo". These are not regex patterns
but structural checks.

## Acceptance criteria

- [x] `FileChecker` implements DirectoryChecker — checks file existence/content
- [x] Parses `File: <glob>` lines from rule markdown (file must exist)
- [x] Parses `NoFile: <glob>` lines from rule markdown (file must NOT exist)
- [x] Parses `Content: <glob> contains <pattern>` for basic content checks
- [x] Returns violations with file path and reason

## Technical notes

- File checks operate on the target directory, not individual files
- A single rule can have multiple file checks
- Use `filepath.Glob` for pattern matching

## Depends on

- #011 (Rule Loader) — needs `Rule` struct with Body field
