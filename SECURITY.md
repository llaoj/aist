# Security Policy

aist executes shell commands and proxies LLM requests on behalf of operators, so we take security reports seriously.

## Supported Versions

Only the latest release (`v*`) receives security fixes; the client and the server share one version number. Please upgrade before reporting.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions or pull requests.**

Report privately via [GitHub Security Advisories](https://github.com/llaoj/aist/security/advisories/new) (Security → Report a vulnerability), or email [security@aistrun.cc](mailto:security@aistrun.cc).

Please include:

- affected component (client / server / install scripts) and version;
- steps to reproduce or a proof of concept;
- impact as you understand it.

## What to Expect

- Acknowledgement within **3 business days**.
- An initial assessment within **7 days**.
- We will keep you informed while preparing a fix, coordinate a disclosure date with you, and credit you in the advisory unless you prefer to stay anonymous.

## Scope

Examples of issues we are particularly interested in:

- bypassing command confirmation, the command blacklist or audit logging;
- leaking upstream API keys, auth tokens or other users' data from the server;
- authentication or authorization flaws in the server;
- tampering with client downloads (`run.sh`, checksums, release artifacts).

Out of scope:

- commands the user explicitly confirmed and executed;
- issues that require a compromised machine or a malicious server operator;
- known limitations listed in [docs/security/overview.md](docs/security/overview.md) and [docs/administration/audit-logs.md](docs/administration/audit-logs.md) (for example, audit records come from client-reported messages).
