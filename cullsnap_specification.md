# CullSnap Comprehensive Specification

**Version**: 1.0 (Final)
**Date**: 2026-02-01
**Objective**: Build a high-performance photo culling application locally on macOS using Go and Fyne.

---

## 1. Project Overview
CullSnap is a native macOS application designed for photographers to rapidly review, select, and export large volumes of photos. It prioritizes speed, keyboard efficiency, and reliability.

## 2. Technology Stack
- **Language**: Go (Golang) 1.23+
- **GUI Framework**: [Fyne v2](https://fyne.io/)
- **Image Processing**: `github.com/disintegration/imaging` (High quality resizing), `github.com/rwcarlsen/goexif` (EXIF extraction)
- **Database**: `modernc.org/sqlite` (Embedded SQL database, CGO-free preferred)
- **Logging**: `log/slog` (Structured logging)

## 3. Architecture
The application follows a modular "Clean Architecture" approach:
- **`cmd/cullsnap`**: Entry point. Initializes Logger, Database, and UI.
- **`internal/model`**: Domain objects (`Photo`, `Session`).
- **`internal/storage`**: SQLite persistence layer.
- **`internal/ui`**: Fyne UI components (`Layout`, `Grid`, `Viewer`).
- **`internal/scanner`**: High-performance directory crawler (Worker Pool pattern).
- **`internal/image`**: Image decoding and thumbnail generation.
- **`internal/export`**: File copying logic.
- **`internal/logger`**: Logging configuration.

## 4. Feature Requirements (Updated)

### 4.1. Core Workflow
- **Directory Scanning**:
  - Recursively scan a selected directory for JPG/JPEG images.
  - **Visual Feedback**: Display an **infinite progress bar** (Blue) at the top/bottom while scanning.
  - **Context Label**: Display the **Current Folder Name** clearly in the UI header.
- **Thumbnail Grid (Left Panel)**:
  - Display photos in a responsive grid.
  - **Virtualization**: Use `widget.List` to handle 10,000+ photos efficiently.
  - **Async Loading**: Thumbnails load in background goroutines.
  - **Visual States**:
    - **Selected**: Highlight with a **Primary Color (Blue) Border**.
    - **Exported**: Display a **Green Checkmark Icon** overlay (Top-Right) if the photo has been previously exported.
- **High-Res Viewer (Right Panel)**:
  - Display the currently clicked photo.
  - **Full Resolution**: detailed view (not upscaled thumbnail).
  - Handle Loading/Error states visually.
- **Selection**:
  - Toggle selection with **`S`** or **`Shift+S`**.
  - **Persistence**: Selections are saved to SQLite immediately. Closing/reopening the app retains selections.

### 4.2. Navigation & History
- **Open Folder**: Standard System Dialog.
- **Manual Path**: Dialog to manually type/paste a full path (for network shares/hidden dirs).
- **Recent Folders**:
  - Store the last **30** accessed directories in SQLite.
  - Access via a **History Icon** (Clock) in the toolbar.
  - Auto-scrolling list if many items.

### 4.3. Export ("Print")
- Toolbar Action: **Print Icon**.
- **Dialog Options**:
  - **Browse**: Select destination via system picker.
  - **Enter Path**: Type destination path manually.
- **Behavior**:
  - Automatically create a subfolder named `Session_YYYYMMDD_HHMMSS` inside the destination.
  - Copy selected photos to this subfolder.
- **Post-Export**:
  - Mark exported photos as **Exported** in SQLite.
  - Update UI immediately (Green Checkmarks).
  - Prevent accidental re-selection (Visual indication helps user).

### 4.4. Reliability & Help
- **Logging**:
  - Write detailed logs to `cullsnap.log` in the working directory.
  - Toolbar Action: **Log Icon** opens the log file in default OS viewer.
- **Help**:
  - Toolbar Action: **Help Icon** (?) opens a dialog explaining controls and icons.
- **Threading**:
  - ALL UI updates from background threads MUST use `fyne.CurrentApp().Driver().DoFromGoroutine(func(){ ... }, false)` to prevent crashes.

## 5. Database Schema
Use SQLite. Tables:
- `selections`: `(path TEXT PRIMARY KEY, session_id TEXT, selected_at DATETIME)`
- `recents`: `(path TEXT PRIMARY KEY, accessed_at DATETIME)` (Limit 30 via application logic)
- `exported`: `(path TEXT PRIMARY KEY, exported_at DATETIME)`

## 6. Implementation Prompt
*If you were to provide this to an AI to build from scratch, use the following prompt:*

> "Build a desktop photo culling app 'CullSnap' using Go and Fyne v2.
>
> **Core Features:**
> 1.  **Split View**: Left panel for Thumbnail Grid, Right panel for Full-Res Viewer.
> 2.  **Performance**: Handle 10,000+ images. use async loading and virtualized lists (widget.List).
> 3.  **Selection**: click to view, `S`/`Shift+S` to partial-select. Visually highlight selected items with a **Blue Border**.
> 4.  **Persistence**: usage `modernc.org/sqlite`. Save selections, 'Exported' status, and 'Recent Folders' (Last 30).
> 5.  **Export**: Button to copy selected files. Allow **Manual Path Entry** or **Browse**. Create a `Session_{Timestamp}` subfolder. Mark exported files with a **Green Checkmark** in the grid.
> 6.  **Navigation**: Browse folder, Manual Path Entry, and Recents Menu. Show **Current Path** label in UI.
> 7.  **UX**: Show **Infinite Progress Bar** during scans. Add **Help** and **Log** buttons.
> 8.  **Strict Threading**: Use `DoFromGoroutine` for all UI updates from background workers.
>
> **Architecture**:
> - Separate `ui`, `scanner`, `storage`, `export` packages.
> - `cmd/main.go` wires dependencies.
> - Ensure `make run` works out of the box."

---
**End of Specification**
