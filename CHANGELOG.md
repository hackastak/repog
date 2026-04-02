# Changelog

All notable changes to RepoG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-04-01

Major release introducing multi-provider support for embeddings and LLM generation.

### Added

- **Multi-Provider Support**
  - OpenAI embeddings (`text-embedding-3-small`, `text-embedding-3-large`, `text-embedding-ada-002`)
  - OpenAI LLM (`gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-3.5-turbo`)
  - Anthropic LLM (`claude-sonnet-4-20250514`, `claude-3-5-sonnet-20241022`, `claude-3-haiku-20240307`)
  - Voyage AI embeddings (`voyage-3`, `voyage-3-lite`, `voyage-code-3`)
  - OpenRouter LLM (access to 100+ models via unified API)
  - Ollama local embeddings (`nomic-embed-text`, `mxbai-embed-large`, `all-minilm`, `snowflake-arctic-embed`)
  - Ollama local LLM (Llama, Mistral, Qwen, DeepSeek, Gemma, and more)

- **New Commands**
  - `repog reconfig` - Change embedding/LLM providers without losing synced data

- **Dynamic Chunking**
  - Automatic chunk size calculation based on model token limits
  - Custom max token limit option during `init` and `reconfig`
  - Model-specific token limits and dimensions for all embedding providers

- **Enhanced Sync**
  - Default behavior syncs both owned and starred repos when no flags specified

### Changed

- Provider abstraction layer for pluggable embedding and LLM backends
- Interactive model selection with fallback options during provider changes
- Improved chunking strategy to avoid embedding API token limit errors

### Fixed

- Chunking strategy edge cases that caused embedding errors
- Go linter version compatibility issues in CI

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

[0.2.0]: https://github.com/hackastak/repog/releases/tag/v0.2.0
[0.1.0]: https://github.com/hackastak/repog/releases/tag/v0.1.0
