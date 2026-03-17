# RepoG

AI-powered knowledge base for your GitHub repositories.

[![CI](https://github.com/hackastak/RepoG/actions/workflows/ci.yml/badge.svg)](https://github.com/hackastak/RepoG/actions/workflows/ci.yml)

## What is RepoG?

RepoG is a CLI tool that syncs your GitHub repositories to a local knowledge base, generates embeddings for the code, and allows you to perform semantic search, ask questions, and get AI-powered recommendations across your entire codebase.

## Prerequisites

- **Go**: 1.23 or higher
- **C Compiler**: GCC or Clang (required for SQLite)
- **GitHub Account**: A fine-grained Personal Access Token (PAT)
- **Google AI Studio Account**: A Gemini API key

## Installation

### From Source (Recommended)

```bash
go install github.com/hackastak/repog/cmd/repog@latest
```

### Local Development

1. Clone the repository:
   ```bash
   git clone https://github.com/hackastak/RepoG.git
   cd RepoG
   ```

2. Build:
   ```bash
   go build -o repog ./cmd/repog
   ```

3. Install locally:
   ```bash
   go install ./cmd/repog
   ```

## Setup

Run the initialization command to set up your credentials:

```bash
repog init
```

> **GitHub Token Permissions**
> You need a fine-grained Personal Access Token with the following permissions:
> - **Repository access**: All repositories (or select specific ones)
> - **Repository permissions**:
>   - `Contents`: Read-only
>   - `Metadata`: Read-only

## Usage

### Sync Repositories
Download repository metadata and file content to the local database.

```bash
repog sync --owned --starred
# Syncing repositories...
# ✔ Found 142 repositories (23 owned, 119 starred)
# ✔ Synced metadata for 142 repos
# ✔ Downloaded content for 5 new repos
```

### Generate Embeddings
Process the synced code into vector embeddings for AI search.

```bash
repog embed
# Generating embeddings...
# ✔ Processed 150 chunks from hackastak/RepoG
# ✔ Processed 89 chunks from hackastak/dotfiles
# ✔ Embedded 239 total chunks
```

### Semantic Search
Search your codebase using natural language concepts, not just keywords.

```bash
repog search "machine learning frameworks"
# Results for "machine learning frameworks":
# 1. hackastak/ml-experiments (0.89)
#    - experiments/tf_setup.py: Imports TensorFlow and configures GPU
# 2. hackastak/notes (0.82)
#    - ml/frameworks.md: Comparison of PyTorch vs TensorFlow
```

### Ask Questions (RAG)
Ask complex questions that require synthesizing information from multiple files.

```bash
repog ask "Which of my repos uses Redis?"
# Based on your repositories, the following projects use Redis:
# - `backend-api`: Uses `ioredis` for caching session data (src/lib/cache.ts)
# - `worker-queue`: Uses Redis for job queue management via BullMQ (src/queue.ts)
```

### Get Recommendations
Find repositories relevant to a specific task or technology.

```bash
repog recommend "building a REST API"
# Recommended repositories for "building a REST API":
# 1. hackastak/express-starter - A boilerplate for Express.js APIs
# 2. hackastak/fastify-demo - Example of a high-performance Fastify server
# 3. hackastak/todo-api - A simple REST API reference implementation
```

### Summarize a Repository
Get a high-level AI summary of what a specific repository does.

```bash
repog summarize hackastak/RepoG
# Summary for hackastak/RepoG:
# RepoG is a CLI tool for AI-powered knowledge management of GitHub repositories.
# It features a monorepo structure with a core logic package and a CLI interface.
# Key technologies include TypeScript, SQLite for local storage, and Google's Gemini API for embeddings and reasoning.
```

### Check Status
View the current state of your local knowledge base.

```bash
repog status
# RepoG Status
# ─────────────────────────────────────────────
#
#   Repositories
#     Total:                       142
#     Owned:                        23
#     Starred:                     119
#     Embedded:                    142
#     Pending embed:                 0
#
#   Knowledge Base
#     Chunks:                    12,450
#     Embeddings:                12,450
#
#   Last Sync
#     Status:                completed
#     Date:                2 hours ago
#
#   Last Embed
#     Date:                2 hours ago
#
#   GitHub API
#     Remaining:         4,982 / 5,000
#     Resets:               58 minutes
#
#   Database
#     Path:          ~/.repog/repog.db
#     Size:                    45.2 MB
#
# ─────────────────────────────────────────────
# Generated at 14:30:00
```

## GitHub API Rate Limits
RepoG respects the GitHub API rate limit of 5,000 requests per hour for authenticated users. It automatically handles rate limiting by pausing or retrying requests as needed.

## Data & Privacy
- **Local First**: All repository data and embeddings are stored locally in `~/.repog/repog.db`.
- **Credentials**: API keys are stored securely in your system's keychain (macOS Keychain, Windows Credential Manager, or Linux Secret Service).
- **Privacy**: No code or data is sent to any server other than:
  - **GitHub API**: To fetch repository data.
  - **Google Gemini API**: To generate embeddings and answers.

## Development

```bash
# Build
go build -o repog ./cmd/repog

# Run tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Lint (requires golangci-lint)
golangci-lint run
```

## License

MIT © [SMILESTACKLABS](https://github.com/hackastak)
