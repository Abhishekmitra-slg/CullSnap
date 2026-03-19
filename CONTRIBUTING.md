# Contributing to CullSnap

Thanks for your interest in contributing to CullSnap! Whether it's a bug fix, new feature, or documentation improvement, contributions are welcome.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md) in all interactions.

## CLA (Contributor License Agreement)

CullSnap uses a dual license model (AGPLv3 + Commercial). Because of this, all contributors must sign a Contributor License Agreement before their first PR can be merged.

The CLA is managed via [CLA Assistant](https://cla-assistant.io/) and will automatically comment on your pull request when you open one. Simply follow the link in the comment to sign. This is a one-time requirement.

## Getting Started

### Prerequisites

- Go 1.25+
- Node.js 22+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0`
- FFmpeg (optional, for video features)

### Setup

```bash
git clone https://github.com/Abhishekmitra-slg/CullSnap.git
cd CullSnap
cd frontend && npm install && cd ..
make dev  # runs in watch mode
```

### Running Tests

```bash
make test
```

### Linting

```bash
make lint  # runs golangci-lint
```

## Pull Request Process

1. Fork the repo and create a feature branch from `main`.
2. Write tests for new functionality. Coverage must stay above 95%.
3. Ensure `make lint` and `make test` pass locally before pushing.
4. PR title should follow conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, etc.
5. All PRs require at least one review from a maintainer.
6. CI must pass (lint, test, build, security scan).

## Code Style

- **Go**: Follow golangci-lint rules (see `.golangci.yml`).
- **TypeScript/React**: Standard React patterns, functional components.
- **Commit messages**: Use [conventional commits](https://www.conventionalcommits.org/) format.

## Reporting Bugs

Use the [GitHub issue templates](https://github.com/Abhishekmitra-slg/CullSnap/issues/new/choose). For security vulnerabilities, see [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the AGPLv3 license. You also grant the maintainer the right to offer your contributions under a commercial license, as outlined in the CLA.
