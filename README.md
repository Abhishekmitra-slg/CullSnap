# CullSnap 📸

[![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)](go.mod)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Abhishekmitra-slg/CullSnap)](https://goreportcard.com/report/github.com/Abhishekmitra-slg/CullSnap)

**CullSnap** is a blazing fast, native desktop tool for photographers to cull and select thousands of photos in seconds. Built with Go and Fyne.

## ✨ Features

-   **Table Grid**: Customized virtualized table architecture for instant scrolling through thousands of photos.
-   **Raw Preview**: High-performance embedded thumbnail extraction for RAW files.
-   **Traffic Light Culling**: Intuitive Green (Keep/Select) and Red (Reject) workflow with instant visual feedback.
-   **Export Ready**: One-click export of selected photos to a separate directory.

## 🛠️ Installation

You can install CullSnap directly using the Go toolchain:

```bash
go install github.com/Abhishekmitra-slg/CullSnap/cmd/cullsnap@latest
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

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
