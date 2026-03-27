---
layout: default
title: Privacy Policy
---

# Privacy Policy

**Last updated:** March 24, 2026

## Overview

CullSnap is a desktop photo and video culling application. Your privacy is important to us. This policy explains what data CullSnap accesses and how it is handled.

## Data CullSnap Accesses

### Local Files
CullSnap reads photos and videos from directories you explicitly open. Files are only accessed from folders you select — CullSnap never scans your system without your action.

### Google Drive (Optional)
When you connect Google Drive, CullSnap requests read-only access to your Drive files. CullSnap:
- Downloads photos from folders you select to a local cache for culling
- Stores your OAuth authentication token securely in your operating system's keychain (macOS Keychain, Windows Credential Manager, or Linux Secret Service)
- Never modifies, deletes, or uploads files to your Google Drive
- Never shares your Google Drive data with any third party

You can disconnect Google Drive at any time from Settings, which deletes the stored token and local cache.

### iCloud Photos (Optional, macOS)
When you use the iCloud Photos integration, CullSnap communicates with the Photos app on your Mac to export albums locally. CullSnap does not access iCloud servers directly.

## Data Storage

All data stays on your device:
- **Thumbnails:** Cached in `~/.cache/CullSnap/thumbs/`
- **Cloud mirrors:** Cached in `~/.cache/CullSnap/cloud/`
- **Device imports:** Cached in `~/.cache/CullSnap/imports/`
- **Selections and ratings:** Stored in a local SQLite database
- **OAuth tokens:** Stored in your OS keychain (never in plaintext)

CullSnap does not operate any servers. No telemetry, analytics, or usage data is collected or transmitted.

## Network Access

CullSnap makes network requests only for:
1. **Google Drive API** — When you explicitly use the Cloud Albums feature
2. **GitHub Releases API** — To check for application updates (configurable in Settings)
3. **FFmpeg download** — One-time download of FFmpeg binary for video support

No other network connections are made.

## Third-Party Services

CullSnap uses the Google Drive API under Google's Terms of Service. When you authenticate with Google, you are subject to [Google's Privacy Policy](https://policies.google.com/privacy).

## Data Deletion

- Disconnect cloud sources in Settings to delete cached tokens and mirrored files
- Clear thumbnail cache, cloud mirrors, and device imports individually in Settings
- Uninstall CullSnap and delete `~/.cache/CullSnap/` to remove all local data

## Contact

For privacy questions, open an issue at [github.com/Abhishekmitra-slg/CullSnap](https://github.com/Abhishekmitra-slg/CullSnap/issues) or email the developer contact listed in the repository.
