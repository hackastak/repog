# Security Policy

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Given that RepoG handles sensitive credentials including GitHub Personal Access Tokens and API keys, we take security issues seriously. If you discover a security vulnerability, please report it privately.

### How to Report

Send security vulnerability reports to: **ahwigint@gmail.com**

Alternatively, you can use GitHub's private vulnerability reporting feature:

1. Go to the [Security tab](https://github.com/SmileStackLabs/repog/security)
2. Click "Report a vulnerability"
3. Fill out the vulnerability details form

### What to Include

Please include the following information in your report:

- **Description**: A clear description of the vulnerability
- **Impact**: What an attacker could achieve by exploiting this vulnerability
- **Reproduction Steps**: Detailed steps to reproduce the issue
- **Affected Versions**: Which versions of RepoG are affected
- **Proposed Fix** (optional): If you have a suggestion for how to fix the issue
- **Environment Details**: OS, Go version, and any other relevant details

### Response Timeline

- **Initial Response**: We aim to acknowledge receipt within 48 hours
- **Status Update**: We will provide a more detailed response within 7 days, including:
  - Confirmation of the issue or explanation if it's not a vulnerability
  - Our planned timeline for a fix
  - Any workarounds or mitigations available
- **Fix Release**: We aim to release a fix within 30 days for critical vulnerabilities

### Disclosure Policy

- **Coordinated Disclosure**: We ask that you do not publicly disclose the vulnerability until we have released a fix
- **Credit**: We will credit researchers who responsibly disclose vulnerabilities (unless you prefer to remain anonymous)
- **Public Disclosure**: Once a fix is released, we will publish a security advisory with details about the vulnerability and the fix

## Supported Versions

We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| Latest  | :white_check_mark: |
| < 1.0   | :x:                |

We recommend always using the latest version of RepoG to ensure you have the latest security fixes.

## Security Best Practices

When using RepoG, please follow these security best practices:

### Credential Management

- **Never commit credentials**: RepoG stores credentials in your system keyring, not in files. Never add API keys or tokens to version control.
- **Use minimal permissions**: Create GitHub Personal Access Tokens with the minimum required scopes (typically `repo` for private repos, `public_repo` for public only).
- **Rotate credentials**: Regularly rotate your GitHub PAT and Gemini API keys.
- **Review access**: Periodically review which applications have access to your GitHub account and API services.

### Database Security

- **Database location**: The RepoG database (`~/.repog/repog.db`) may contain repository metadata and code snippets. Ensure this directory has appropriate filesystem permissions.
- **Backups**: If you back up your home directory, be aware that the database contains repository content that may be sensitive.

### API Key Security

- **Gemini API keys**: Protect your Gemini API key as it provides access to Google's AI services under your account.
- **Key rotation**: If you suspect your API key has been compromised, rotate it immediately via the Google AI Studio console and run `repog init` to update the stored credential.

### Network Security

- **HTTPS**: All API communications (GitHub and Gemini) use HTTPS.
- **Corporate proxies**: If using RepoG behind a corporate proxy, ensure the proxy is configured correctly to avoid credential leakage.

## Known Security Considerations

### SQLite and CGO

RepoG uses SQLite with the sqlite-vec extension via CGO. This means:

- The binary includes compiled C code for SQLite
- SQL injection vulnerabilities are mitigated through prepared statements
- The database file is not encrypted by default

### Keyring Storage

Credentials are stored in your system's native keyring:

- **macOS**: Keychain Access
- **Linux**: Secret Service API (GNOME Keyring, KWallet)
- **Windows**: Windows Credential Manager

The security of stored credentials depends on your system's keyring implementation and your login credentials.

## Security Features

RepoG implements the following security features:

- **Keyring integration**: API keys and tokens are never stored in plaintext files
- **Prepared statements**: All database queries use prepared statements to prevent SQL injection
- **No credential logging**: Credentials are never logged or included in error messages
- **Secure defaults**: Minimal required API permissions are documented

## Contact

For security-related questions or concerns that are not vulnerability reports, you can reach us at:

- Email: ahwigint@gmail.com
- GitHub Discussions: [RepoG Discussions](https://github.com/SmileStackLabs/repog/discussions)

Thank you for helping keep RepoG and its users secure!
