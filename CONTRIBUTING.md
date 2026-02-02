# Contributing to CullSnap

First off, thank you for considering contributing to CullSnap! It's people like you that make tools tool better.

## 🛠️ Development Setup

1.  **Language**: Go 1.22 or later.
2.  **Framework**: [Fyne v2](https://github.com/fyne-io/fyne).
3.  **Database**: SQLite (via `modern-go` or `mattn` driver).

### getting Started
1.  Clone the repo: `git clone https://github.com/abhishekmitra/CullSnap.git`
2.  Install dependencies: `go mod tidy`
3.  Run the app: `go run cmd/cullsnap/main.go`

## 📂 Project Structure

-   `cmd/cullsnap`: Main entry point.
-   `internal/ui`: All Fyne UI components (Grid, Layout, Picker).
-   `internal/model`: Core data structures (Photo, Session).
-   `internal/scanner`: File system scanning logic.
-   `internal/storage`: SQLite persistence layer.

## 📏 Coding Standards

-   **Formatting**: We use `gofmt`. Please run `go fmt ./...` before committing.
-   **Linting**: We recommend `staticcheck` or `golangci-lint`.
-   **Comments**: Public functions should have GoDoc style comments.

## 🚀 Pull Request Process

1.  Create a feature branch: `git checkout -b feature/amazing-feature`.
2.  Commit your changes: `git commit -m 'Add some amazing feature'`.
3.  Push to the branch: `git push origin feature/amazing-feature`.
4.  Open a Pull Request.

## 🐛 Reporting Bugs

Please include:
-   OS Version (macOS, Windows, Linux).
-   Steps to reproduce.
-   Expected vs Actual behavior.
-   Any logs from `cullsnap.log`.
