# CullSnap 📸

![Go Version](https://img.shields.io/github/go-mod/go-version/abhishekmitra/CullSnap)
![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen)

**CullSnap** is a high-performance, native photo culling application built for speed. Designed for photographers who need to quickly sift through thousands of RAW/JPG images, CullSnap eliminates the "loading time" friction of traditional editors.

## 🚀 Why CullSnap?

Processing a shoot with 2,000+ photos shouldn't mean waiting for previews to render. CullSnap focuses on one thing: **Velocity**.

-   **Zero-Latency Grid**: Customized virtualized table architecture for instant scrolling.
-   **Lighting Fast Preview**: Optimized caching pipeline for high-res images.
-   **"Traffic Light" Workflow**: Intuitive Green (Keep) / Red (Reject) selection system.
-   **Export Ready**: One-click export of selected photos to a separate directory.

## ✨ Features

-   **Virtual Infinite Grid**: Handle directories with 10k+ photos without lag.
-   **Instant Shortcuts**: `S` (Select), `X` (Reject), `Space` (Preview).
-   **RAW Support**: Native handling of standard image formats (JPG, PNG) and RAW previews (via embedded thumbs).
-   **Session Memory**: Remembers your selections and progress even after restarting.
-   **Export Workflow**: Automatically copies "Key" shots to a `Session_YYYYMMDD` folder.

## 🛠️ Installation

### Prerequisites
-   **Go 1.22+**: [Download Go](https://go.dev/dl/)
-   **Fyne Dependencies**: [Fyne Prerequisites](https://developer.fyne.io/started/) (GCC usually required).

### Quick Start
```bash
# Clone the repository
git clone https://github.com/abhishekmitra/CullSnap.git
cd CullSnap

# Run directly
go run cmd/cullsnap/main.go

# Build binary
go build -o cullsnap cmd/cullsnap/main.go
```

## 🎮 Usage Guide

1.  **Open Folder**: Click the Folder icon to load a directory.
2.  **Cull**:
    -   Use `Arrow Keys` to navigate.
    -   Press `S` or `Shift+S` to **Keep** (Green Border).
    -   Press `X` to **Reject** (Red Dim).
3.  **Review**: Grid provides instant visual feedback on your selections.
4.  **Export**: Click the **Save Icon** in the toolbar to copy all "Kept" photos to a new folder.

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on how to submit pull requests, report issues, and layout the codebase.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
