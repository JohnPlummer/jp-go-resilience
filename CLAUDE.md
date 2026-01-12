# CLAUDE.md

Configuration for Claude Code when working with jp-go-resilience package.

## Standards

Use `/ai-common` skill to load development standards and patterns as needed.

## Package Purpose

jp-go-resilience provides resilience patterns for Go projects with:

- Circuit breakers for protecting downstream services
- Retry logic with exponential backoff
- Health checking and status monitoring
- Configurable failure thresholds and timeouts

## Development Guidelines

This is a **shared package** used across multiple projects. Changes must be:

- Backward compatible
- Well-tested
- Generic (not project-specific)
- Documented in examples
