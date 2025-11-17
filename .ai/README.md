# AI Assistant Documentation

Progressive loading documentation system for jp-go-resilience package.

## Quick Start

**Entry point:** Read `llms.md` first - it provides a complete map of all available standards.

## Structure

```
.ai/
├── llms.md                     # Package navigation map (start here)
├── README.md                   # This file
├── context.md                  # Current active work (gitignored)
├── memory.md                   # Stable knowledge (gitignored)
├── tasks/                      # Active work scratchpad (gitignored)
├── common -> ~/code/ai-common  # Symlink to shared common standards
└── project-standards/          # Package-specific patterns (if needed)
```

## Common vs Package Standards

**Common standards** (in `common/`):

- Portable patterns usable across all Go projects
- Accessed via symlink: `.ai/common -> ~/code/ai-common`
- Includes: Go patterns, testing, architecture, documentation

**Package standards** (in `project-standards/`):

- Specific to jp-go-resilience package only
- Currently minimal since this package IS a standard itself

## Working Files

**context.md**: Current active work

- Updated during development
- References current changes, PRs, issues
- Cleared when no longer relevant
- Gitignored (not committed)

**memory.md**: Stable package knowledge

- Design decisions and rationale
- Backward compatibility notes
- Gotchas and lessons learned
- Gitignored (not committed)

**tasks/**: Scratchpad for active work

- Planning documents
- Analysis notes
- Gitignored (not committed)

## Usage Pattern

1. Read `llms.md` to see available standards
2. Load `common/common-llms.md` for common standards map
3. Load specific files based on current task:
   - Go patterns → `common/standards/go/`
   - Testing → `common/standards/testing/`
   - Documentation → `common/standards/documentation/`

## References

- Common standards integration: `common/README.md`
- Package entry: `CLAUDE.md`
- Usage guide: `common/standards/go/jp-go-resilience.md` (how to USE this package, if exists)
