# CullSnap 📸

[![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)](go.mod)
[![Wails Version](https://img.shields.io/badge/Wails-v2-red?style=flat&logo=wails)](wails.json)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**CullSnap** is a high-performance, native desktop tool designed for photographers to cull and select thousands of high-resolution photos in seconds. Built with the **Wails Framework** (Go backend + React/Vite frontend) delivering a stunning dark-mode glassmorphism experience.

## ✨ Features

-   **Fast Photo & Video Grid**: Modern grid layout with lazy loading and cached disk thumbnails for buttery-smooth scrolling through 1000s of assets.
-   **Video Support**: Native support for MP4, MOV, WEBM, MKV, and AVI. Includes duration extraction and frame-accurate thumbnail generation.
-   **Native Apple Silicon Support**: Automatically provisions native `arm64` FFmpeg and FFprobe binaries on Apple Silicon Macs, delivering maximum performance without Rosetta 2.
-   **Disk-Based Thumbnail Cache**: Parallel Go goroutines generate 300px JPEG thumbnails to `~/.cullsnap/thumbs/` with secure permissions.
-   **Smart Deduplication**: Pure-Go perceptual hashing automatically groups duplicate/burst photos and selects the sharpest image using a Laplacian Variance algorithm.
-   **JPEG & PNG Processing**: High-performance embedded thumbnail extraction with parallel goroutine generation.
-   **EXIF Metadata**: Frosted-glass overlay card displaying Camera, Lens, ISO, Aperture, Shutter Speed, and Date Taken.
-   **Stable Media Architecture**: Uses a dedicated high-speed server on port `34342` to serve local assets, ensuring smooth video scrubbing and instant image loading.
-   **Custom Export Pathing**: Rename the destination folder on-the-fly during export for better session organization.
-   **Star Ratings**: 1–5 star rating system persisted to SQLite for each photo and video.
-   **Resource Monitoring**: Real-time CPU, RAM, Disk I/O, and Network tracking in the status bar.

## 🏗️ Architecture

CullSnap natively binds a high-performance **Go** backend to a modern **React/Vite** frontend using the **Wails Framework**.

```mermaid
graph LR
    subgraph Frontend [React / Vite UI]
        direction TB
        Grid["Photo Grid\n(CSS Grid + lazy loading)"]
        Viewer["Full-Res Viewer\n+ EXIF Panel"]
        Sidebar["Sidebar Controls\n& Star Ratings"]
    end

    subgraph IPC [Wails Bridge]
        Bindings["TypeScript Bindings\nwailsjs/go/app/App"]
    end

    subgraph Backend [Go Desktop Backend]
        direction TB
        Scanner["Directory Scanner\n(internal/scanner)"]
        ThumbCache["Thumbnail Cache\n(internal/image/thumbcache)"]
        FFmpeg["FFmpeg Engine\n(internal/video/ffmpeg)"]
        Dedupe["Perceptual Hash\n+ Quality Scoring"]
        Storage["SQLite Persistence\n(internal/storage)"]
        OS["System Resources\n(shirou/gopsutil)"]
        MediaServer["Dedicated Media Server\n(Port 34342)"]
    end

    Grid <-->|Fetch Photos / Thumbnails| Bindings
    Viewer <-->|Load Full-Res + EXIF| Bindings
    Sidebar <-->|Export / Dedup / Ratings| Bindings

    Bindings <-->|Scan + Cache Thumbs| Scanner
    Bindings <-->|Parallel Thumb Gen| ThumbCache
    Bindings <-->|Find Duplicates| Dedupe
    Bindings <-->|Persist State| Storage
    Bindings <-->|Read CPU/RAM| OS
```

## 🛠️ Installation

### 🍎 macOS (Recommended)
The easiest way to install CullSnap on macOS and bypass Gatekeeper warnings is via Homebrew:

```bash
brew tap abhishekmitra-slg/tap
brew install --cask cullsnap
```

### 🪟 Windows & 🐧 Linux
Download the latest binary from the [Releases](https://github.com/Abhishekmitra-slg/CullSnap/releases) page.

---

### 🍎 Manual macOS Installation (Troubleshooting)
If you download the `.zip` manually instead of using Homebrew, macOS will flag it with *"Apple could not verify CullSnap is free of malware."*

To run the manually downloaded app:
1. Open your Terminal.
2. Run:
```bash
xattr -cr /Applications/CullSnap.app
```

### Building from Source
Ensure you have [Go](https://go.dev/) and Node/NPM installed. Then install the Wails CLI:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

To build a Native Application Bundle (`.app` for Mac):
```bash
make build
# Output lands in ./build/bin/CullSnap.app
```

To run in Developer Watch-Mode:
```bash
make dev
```

## 🎮 Usage Guide

1.  **Open Folder**: Click **Open Folder** to load a directory from your machine or external drive.
2.  **Deduplicate**: Click **Find Duplicates** to automatically group burst shots and isolate the sharpest unique photos. Previously deduped folders are auto-detected.
3.  **Navigate**: Use `← / →` or `↑ / ↓` arrow keys to traverse through photos.
4.  **Rate**: Click the stars (1–5) on any thumbnail to rate photos.
5.  **Cull**: Press `S` to toggle keeping the photo (Blue Checkmark).
7.  **Review**: The grid provides instant visual feedback — Blue Checkmarks for selections, Green Checkmarks for previously exported files.
8.  **EXIF**: Select any asset to view its metadata in the frosted-glass overlay.
9.  **Export**: Click **Export (N)**. You'll be prompted for a destination directory name to keep your sessions organized. CullSnap will copy all full-resolution originals (and trimmed videos) to the new folder.

## 📁 Project Structure

```
CullSnap/
├── main.go                      # Wails app entry + Dedicated Port 34342 Media Server
├── internal/
│   ├── app/app.go              # Core app logic, all Wails-bound methods
│   ├── video/
│   │   ├── ffmpeg.go           # Native FFmpeg provisioning & thumbnail extraction
│   │   └── ffprobe.go          # Native FFprobe metadata (duration) extraction
│   ├── image/
│   │   ├── thumbnail.go        # EXIF thumbnail extraction + resize fallback
│   │   └── thumbcache.go       # Disk-based thumbnail cache (parallel workers)
│   ├── scanner/scanner.go      # Directory walker with video/photo detection
│   ├── dedupe/                 # Perceptual hashing + quality scoring
│   ├── export/                 # File copier & lossless video clipper
│   ├── model/photo.go          # Unified Media struct (IsVideo, Duration, Path, etc.)
│   ├── storage/                # SQLite persistence
│   └── logger/                 # Structured logging
└── frontend/src/
    ├── App.tsx                 # Main app with 2-phase loading
    ├── components/
    │   ├── Grid.tsx            # Media grid with badges & lazy loading
    │   ├── Viewer.tsx          # Hybrid Image/Video viewer + Clipping tools
    │   └── Sidebar.tsx         # Controls, folders, dedup status
    └── index.css               # Dark theme, glassmorphism, animations
```

## 🤝 Contributing
We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📄 License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
