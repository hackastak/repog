# Changelog

All notable changes to RepoG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-03-17

Initial public release of RepoG, rewritten in Go.

### Added

- **CLI Commands**
  - `repog init` - Interactive setup with credential validation
  - `repog sync` - Sync owned and/or starred repositories from GitHub
  - `repog embed` - Generate vector embeddings for synced repositories
  - `repog search` - Semantic search across your codebase
  - `repog ask` - Natural language Q&A with RAG
  - `repog recommend` - Find repositories relevant to a task
  - `repog summarize` - AI-generated repository summaries
  - `repog status` - View knowledge base statistics and API quota

- **Core Features**
  - Local SQLite database with sqlite-vec for vector storage
  - Google Gemini integration for embeddings (`gemini-embedding-2-preview`, 768 dimensions)
  - Google Gemini LLM for Q&A, recommendations, and summaries
  - Secure credential storage via system keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
  - GitHub API rate limit handling

- **Distribution**
  - Homebrew tap (`brew install hackastak/tap/repog`)
  - Pre-built binaries for macOS (amd64, arm64) and Linux (amd64, arm64)
  - Install from source via `go install`

- **Developer Experience**
  - CI pipeline with test coverage requirements
  - GoReleaser for automated releases

[0.1.0]: https://github.com/hackastak/repog/releases/tag/v0.1.0
