# jp-go-resilience - AI Documentation

Progressive loading map for AI assistants working with jp-go-resilience package.

**Entry Point**: This file should be referenced from CLAUDE.md.

## Package Overview

**Purpose**: Resilience patterns for Go projects

**Key Features**:

- Circuit breakers (prevent cascading failures)
- Retry logic with exponential backoff
- Health checking and monitoring
- Configurable thresholds and timeouts
- Integration with standard error handling

## Always Load

- `.ai/llms.md` (this file)

## Load for Complex Tasks

- `.ai/memory.md` - Design decisions, gotchas, backward compatibility notes
- `.ai/context.md` - Current changes (if exists and is current)

## Common Standards (Portable Patterns)

**See** `.ai/common/common-llms.md` for the complete list of common standards.

Load these common standards when working on this package:

### Core Go Patterns

- `common/standards/go/constructors.md` - New* constructor functions
- `common/standards/go/error-wrapping.md` - Error wrapping with %w
- `common/standards/go/type-organization.md` - Interface and type placement
- `common/standards/go/functional-options.md` - Option pattern for configuration

### Testing

- `common/standards/testing/bdd-testing.md` - Ginkgo/Gomega patterns
- `common/standards/testing/test-categories.md` - Test organization

### Documentation

- `common/standards/documentation/pattern-documentation.md` - Documentation structure
- `common/standards/documentation/code-references.md` - Code examples

## Project Standards (Package-Specific)

This package has minimal package-specific standards since it IS a standard itself.

Any package-specific patterns should go in `.ai/project-standards/`

## Loading Strategy

| Task Type | Load These Standards |
|-----------|---------------------|
| Adding circuit breaker features | constructors.md, functional-options.md, type-organization.md |
| Adding retry logic | constructors.md, error-wrapping.md |
| Writing tests | bdd-testing.md, test-categories.md |
| Documenting patterns | pattern-documentation.md, code-references.md |
| Ensuring compatibility | memory.md (for backward compatibility notes) |

## File Organization

```
jp-go-resilience/
├── CLAUDE.md                   # Entry point
├── .gitignore                  # Ignores context.md, memory.md, tasks/
└── .ai/
    ├── llms.md                 # This file (loading map)
    ├── README.md               # Documentation about .ai setup
    ├── context.md              # Current work (gitignored)
    ├── memory.md               # Stable knowledge (gitignored)
    ├── tasks/                  # Scratchpad (gitignored)
    ├── project-standards/      # Package-specific (if needed)
    └── common -> ~/code/ai-common  # Symlink to shared standards
```

## Key Principles

1. **Backward Compatibility**: Never break existing circuit breaker or retry behavior
2. **Generic Design**: No project-specific resilience patterns in this package
3. **Error Integration**: Works with standard Go error handling
4. **Configurable**: All thresholds and timeouts can be customized
5. **Observable**: Provides health status and failure metrics

## Related Documentation

- Common standard: `common/standards/go/jp-go-resilience.md` - How to USE this package (if exists)
- This is the implementation, that is the usage guide
