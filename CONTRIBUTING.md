# Contributing to aist

Thanks for your interest in aist! Issues, discussions and pull requests are welcome in English or Chinese (欢迎使用中文或英文).

## Ways to Contribute

- **Report a bug** — open a [bug report](https://github.com/llaoj/aist/issues/new?template=bug_report.yml).
- **Suggest a feature** — open a [feature request](https://github.com/llaoj/aist/issues/new?template=feature_request.yml). For larger changes, please discuss in an issue before writing code.
- **Report a security vulnerability** — do **not** open a public issue; follow [SECURITY.md](SECURITY.md).
- **Improve docs** — typo fixes and clarifications are always welcome.

## Development Setup

Requirements: Go (version in `go.mod`), `git`, Linux or macOS.

```bash
git clone https://github.com/llaoj/aist.git
cd aist
make build        # build client and server
make test         # go test ./...
make dev          # run the server locally for development
```

See [docs/contributing/development.md](docs/contributing/development.md) for the repository layout, build details and end-to-end tests, and [docs/contributing/architecture.md](docs/contributing/architecture.md) / [docs/reference/api-and-errors.md](docs/reference/api-and-errors.md) for the client–server contract.

## Before Opening a Pull Request

Run these checks locally; CI runs the same ones:

```bash
gofmt -l -s .        # must print nothing (fix with: make format)
go vet ./...
go test ./...
make naming-check
```

Code conventions:

- Follow the naming rules in [docs/contributing/naming.md](docs/contributing/naming.md). Use the domain vocabulary defined there and do not introduce synonyms or legacy names.
- Keep the client thin: model access, keys, blacklist and audit belong to the server. The client never calls an LLM provider directly.
- Don't duplicate logic across packages; shared helpers go to `internal/util`.
- Comments explain *why*, not *what*.
- Add or update tests for behavior changes.
- Never commit secrets. Use placeholders such as `<token>` in docs and examples.

## Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <summary>

feat(audit): record shell command type in audit logs
fix(client): honor proxy environment variables for model requests
```

Common types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`. The summary may be in English or Chinese.

## Pull Request Process

1. Fork the repository and create a branch from `main`.
2. Keep each PR focused on one change; update docs alongside code.
3. Fill in the pull request template and link related issues.
4. A maintainer will review it. Please respond to feedback; PRs inactive for 30 days may be closed.

## Contributor License Agreement

aist is licensed under [AGPL-3.0](LICENSE), and the maintainer also offers commercial licenses. To keep that possible, contributors must sign the [Contributor License Agreement](CLA.md) before their first pull request can be merged. You keep the copyright to your work; the CLA grants the maintainer a license to use it, and guarantees your contribution stays available under AGPL-3.0 or another OSI-approved license.

When you open your first pull request, the CLA bot will comment with instructions. Sign by replying:

```text
I have read the CLA Document and I hereby sign the CLA
```

You only need to sign once. If the check doesn't update, comment `recheck`.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold it.
