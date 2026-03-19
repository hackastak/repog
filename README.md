# RepoG

AI-powered knowledge base for your GitHub repositories.

[![CI](https://github.com/hackastak/RepoG/actions/workflows/ci.yml/badge.svg)](https://github.com/hackastak/RepoG/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/hackastak/repog)](https://goreportcard.com/report/github.com/hackastak/repog)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## What is RepoG?

RepoG is a CLI tool that syncs your GitHub repositories to a local knowledge base, generates vector embeddings, and enables semantic search, Q&A, and AI-powered recommendations across your entire codebase.

**Key Features:**
- Sync owned and starred repositories to a local SQLite database
- Generate vector embeddings using Google Gemini
- Semantic search across all your code using natural language
- Ask questions and get AI-synthesized answers (RAG)
- Get repository recommendations for specific tasks
- Summarize repositories with AI

## Installation

### Homebrew (macOS)

```bash
brew install hackastak/tap/repog
```

### Download Binary

Download the latest release for your platform from the [Releases page](https://github.com/hackastak/repog/releases).

### From Source

Requires Go 1.22+ and a C compiler (GCC or Clang) for CGO.

```bash
go install github.com/hackastak/repog/cmd/repog@latest
```

## Quick Start

### 1. Get Your API Keys

You'll need two API keys:

**GitHub Personal Access Token (PAT)**
1. Go to [GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens](https://github.com/settings/tokens?type=beta)
2. Create a new token with:
   - **Repository access**: All repositories (or select specific ones)
   - **Permissions**: `Contents: Read-only`, `Metadata: Read-only`

**Google Gemini API Key**
1. Go to [Google AI Studio](https://aistudio.google.com/apikey)
2. Create a new API key

### 2. Initialize RepoG

```bash
repog init
```

This will prompt you for your API keys and store them securely in your system keychain.

### 3. Sync Your Repositories

```bash
repog sync --owned --starred
```

### 4. Generate Embeddings

```bash
repog embed
```

### 5. Start Searching

```bash
repog search "authentication middleware"
repog ask "Which repos use PostgreSQL?"
repog recommend "building a CLI tool"
```

## Commands

| Command | Description |
|---------|-------------|
| `repog init` | Configure API keys and initialize the database |
| `repog sync` | Sync repository metadata and content |
| `repog embed` | Generate vector embeddings for synced repos |
| `repog search <query>` | Semantic search across your codebase |
| `repog ask <question>` | Ask questions with AI-synthesized answers |
| `repog recommend <task>` | Get repository recommendations |
| `repog summarize <repo>` | AI summary of a specific repository |
| `repog status` | View knowledge base statistics |

### Sync Options

```bash
repog sync --owned           # Sync only your own repositories
repog sync --starred         # Sync only starred repositories
repog sync --owned --starred # Sync both (default)
```

## Data & Privacy

- **Local First**: All data is stored locally in `~/.repog/repog.db`
- **Secure Credentials**: API keys are stored in your system keychain (macOS Keychain, Windows Credential Manager, or Linux Secret Service)
- **Privacy**: Code is only sent to:
  - **GitHub API**: To fetch repository metadata and content
  - **Google Gemini API**: To generate embeddings and AI responses

## GitHub API Rate Limits

RepoG respects GitHub's rate limit of 5,000 requests per hour for authenticated users. Use `repog status` to check your remaining quota.

## Contributing

We welcome contributions! Here's how to get started:

### Development Setup

1. Fork and clone the repository:
   ```bash
   git clone https://github.com/YOUR_USERNAME/repog.git
   cd repog
   ```

2. Install dependencies:
   - Go 1.22+
   - C compiler (GCC or Clang)
   - golangci-lint (for linting)

3. Build and test:
   ```bash
   go build -o repog ./cmd/repog
   go test ./...
   ```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with race detection and coverage
go test -race -coverprofile=coverage.out ./...

# Run tests for a specific package
go test ./internal/db/...

# View coverage report
go tool cover -html=coverage.out
```

### Linting

```bash
golangci-lint run
```

### Submitting Changes

1. Create a feature branch from `main`
2. Make your changes
3. Ensure tests pass and linting is clean
4. Submit a pull request

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New features
- `fix:` Bug fixes
- `docs:` Documentation changes
- `test:` Adding or updating tests
- `refactor:` Code changes that neither fix bugs nor add features
- `chore:` Maintenance tasks

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Keep functions focused and small
- Write table-driven tests where appropriate
- Return errors rather than panicking

### Reporting Issues

- Use the [GitHub issue tracker](https://github.com/hackastak/repog/issues)
- Include steps to reproduce, expected behavior, and actual behavior
- Include your Go version and OS

## License

MIT License - see [LICENSE](LICENSE) for details.

---

Built with [sqlite-vec](https://github.com/asg017/sqlite-vec) and [Google Gemini](https://ai.google.dev/).
