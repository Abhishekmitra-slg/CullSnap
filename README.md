# CullSnap

**CullSnap** is a high-performance, native macOS photo culling application built with [Go](https://go.dev/) and [Fyne](https://fyne.io/). It is designed for photographers who need to rapidly review, select, and export thousands of photos with speed and keyboard efficiency.

## 🚀 Features

- **Blazing Fast**: Handles directories with 10,000+ images using virtualized scrolling and async loading.
- **Efficient Culling**:
    - **Split View**: Thumbnails on the left, full-resolution viewer on the right.
    - **Keyboard Shortcuts**: Press `S` to toggle selection instanty.
- **Persistence**:
    - Selections are saved automatically (backed by SQLite).
    - Remembers which photos you've already exported (visual green checkmark).
    - "Recent Folders" menu for quick access (Last 30 folders).
- **Export Engine**:
    - "Send to Print" feature copies selected photos to a timestamped `Session_...` folder.
    - Prevents accidental duplicates.

## 🛠️ Technology Stack

- **Go 1.23+**
- **Fyne v2** (GUI Framework)
- **SQLite** (`modernc.org/sqlite` - CGO-free embedded DB)
- **Imaging** (`disintegration/imaging` - High-quality resizing)

## 📦 Installation & Build

### Prerequisites
- **Go**: Version 1.23 or higher.
- **C Compiler**: Required by Fyne for OS interaction (e.g., Xcode Command Line Tools on macOS).

### Build from Source
```bash
# Clone the repository
git clone https://github.com/yourusername/CullSnap.git
cd CullSnap

# Install dependencies and build
make build
```

The binary will be created in `bin/CullSnap`.

## 🖥️ Usage

1.  **Run the App**:
    ```bash
    make run
    ```
2.  **Open a Folder**: Click the 📂 Folder icon or use the History menu.
3.  **Select Photos**:
    - Click a thumbnail to view it in high resolution.
    - Press **`S`** (or `Shift+S`) to toggle selection (Blue Border).
4.  **Export**:
    - Click the 🖨️ Print icon.
    - Choose a destination folder.
    - Selected photos are copied to a new subfolder, and marked as exported (Green Checkmark).

## 🗂️ Project Structure

- `cmd/cullsnap`: Application entry point.
- `internal/ui`: Fyne UI layout and components.
- `internal/scanner`: High-performance directory crawler.
- `internal/storage`: SQLite database logic.
- `internal/image`: Thumbnail generation and EXIF handling.

## 📝 License
[MIT](LICENSE)
