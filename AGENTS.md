# RepoG Agent Guide

This guide is designed for AI coding agents and human developers working on the RepoG project. It outlines the project structure, development workflows, and coding standards.

## Project Overview

RepoG is a CLI tool for AI-powered knowledge management of GitHub repositories. It is structured as a monorepo using `pnpm` workspaces.

### Structure
- `packages/cli`: The command-line interface application.
- `packages/core`: The core logic, including GitHub API interaction and AI processing.

## Development Workflow

### Installation
Ensure you have Node.js (v22+) and pnpm installed.

```bash
pnpm install
```

### Building
Build all packages:
```bash
pnpm build
```

Build a specific package:
```bash
pnpm --filter @repog/cli build
pnpm --filter @repog/core build
```

### Linting
Run linting across the repository:
```bash
pnpm lint
```

## Testing

The project uses `vitest` for testing.

### Running Tests
Run all tests:
```bash
pnpm test
```

Run tests for a specific package:
```bash
pnpm --filter @repog/core test
```

### Running Specific Tests
To run a single test file:
```bash
pnpm test packages/core/src/some-file.test.ts
```

To run a specific test case by name:
```bash
pnpm test -t "name of the test"
```

## Code Style & Conventions

### General
- **Language**: TypeScript (Strict mode).
- **Module System**: ESM (ECMAScript Modules).
- **Formatting**: Adhere to Prettier default settings.
- **Linting**: Follow ESLint rules configured in the project.

### Naming Conventions
- **Files/Directories**: `kebab-case` (e.g., `user-profile.ts`, `data-processing/`).
- **Classes/Interfaces/Types**: `PascalCase` (e.g., `UserProfile`, `RepoConfig`).
- **Variables/Functions**: `camelCase` (e.g., `getUserProfile`, `repoList`).
- **Constants**: `UPPER_SNAKE_CASE` (e.g., `MAX_RETRY_COUNT`).

### Imports
- Use specific named imports over default imports where possible.
- Group imports:
    1.  Node.js built-ins (`fs`, `path`).
    2.  External dependencies (`react`, `lodash`).
    3.  Internal project imports (`@repog/core`).
    4.  Relative imports (`./utils`).

### Typing
- Avoid `any` type; use `unknown` if the type is truly uncertain, or define strict interfaces.
- Prefer `interface` for object definitions and `type` for unions/intersections.
- Explicitly type function return values for clarity.

### Error Handling
- Use `try/catch` blocks for asynchronous operations that may fail (API calls, file I/O).
- Throw descriptive `Error` objects or custom error classes.
- Log errors appropriately using the project's logging utilities (if available) or `console.error` for CLI output.

### Asynchronous Code
- Prefer `async/await` over raw Promises (`.then()`, `.catch()`).
- Ensure all promises are handled (awaited or returned).

## Native Dependencies
This project uses native modules (`better-sqlite3`, `keytar`). 
- When running in development or testing, ensure these are rebuilt if Node versions change:
  ```bash
  pnpm rebuild better-sqlite3 keytar
  ```
- These are excluded from the build bundle in `packages/cli/tsup.config.ts` and must be available in `node_modules` at runtime.

## Git Workflow
- **Commit Messages**: Follow Conventional Commits specification.
    - `feat: add new command`
    - `fix: resolve issue with API`
    - `docs: update readme`
    - `chore: update dependencies`

## Strict Constraints
- **NO AUTOMATIC COMMITS:** Under no circumstances should the agent execute `git commit`. The agent may stage changes using `git add`, but the final commit must be performed by the user manually.

