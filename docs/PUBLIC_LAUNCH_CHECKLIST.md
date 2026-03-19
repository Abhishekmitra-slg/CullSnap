# CullSnap Public Launch Checklist

Post-code changes manual steps to complete on GitHub and external services.

## 1. Git History Cleanup (Before Flipping to Public)

```bash
# Install git-filter-repo
brew install git-filter-repo

# Backup first
cp -r CullSnap CullSnap-backup

# Remove bloated paths from all history
git filter-repo --invert-paths \
  --path frontend/node_modules \
  --path frontend/dist \
  --path coverage.out \
  --path storage.out

# Re-add remote and force push (safe since repo is still private)
git remote add origin https://github.com/Abhishekmitra-slg/CullSnap.git
git push --force --all origin
git push --force --tags origin
```

## 2. CLA Assistant Setup

1. Go to [https://cla-assistant.io/](https://cla-assistant.io/)
2. Sign in with your GitHub account
3. Click "Configure CLA" and select `Abhishekmitra-slg/CullSnap`
4. Set the CLA document URL to: `https://github.com/Abhishekmitra-slg/CullSnap/blob/main/.github/CLA.md`
5. Save — the bot will now comment on every PR from a new contributor

## 3. Branch Protection Rules

Go to **Settings > Branches > Add rule** for `main`:

- [x] Require a pull request before merging
  - [x] Require approvals (1)
  - [x] Dismiss stale pull request approvals when new commits are pushed
- [x] Require status checks to pass before merging
  - Add required checks: `Lint`, `Test`, `Build`, `Frontend Lint`, `Security Scan`
- [x] Require branches to be up to date before merging
- [x] Do not allow bypassing the above settings (even for admins)
- [ ] _(Optional)_ Require signed commits

## 4. Enable GitHub Security Features

Go to **Settings > Code security and analysis**:

- [x] Dependabot alerts — Enable
- [x] Dependabot security updates — Enable
- [x] Secret scanning — Enable
- [x] Push protection — Enable
- [x] Code scanning (CodeQL) — Enable (free for public repos)

## 5. Repository Settings

Go to **Settings > General**:

- **Description**: "A blazing-fast, native desktop photo & video culling tool for photographers. Built with Go + React."
- **Website**: Link to latest release
- **Topics**: `photo-culling`, `photography`, `desktop-app`, `wails`, `golang`, `react`, `photo-management`, `video-trimming`, `deduplication`
- **Social preview**: Upload a screenshot or banner image (1280x640px recommended)

Under **Features**:
- [x] Issues
- [x] Discussions (optional — good for community Q&A)
- [x] Projects (optional)

## 6. Flip to Public

Go to **Settings > General > Danger Zone**:

- Click "Change visibility" → Public

## 7. Create "Good First Issue" Labels

Create issues tagged `good first issue` for easy entry points:
- "Add support for RAW formats (CR2, CR3, ARW, NEF, DNG)"
- "Improve test coverage for internal/app package"
- "Improve test coverage for internal/video package"
- "Add keyboard shortcut reference in Settings modal"

## 8. Homebrew Tap

You already have `docs/cullsnap.rb`. Create a public tap:

```bash
# Create the tap repository on GitHub
gh repo create Abhishekmitra-slg/homebrew-cullsnap --public --description "Homebrew tap for CullSnap"

# Clone and add the formula
gh repo clone Abhishekmitra-slg/homebrew-cullsnap
cp docs/cullsnap.rb homebrew-cullsnap/Casks/cullsnap.rb
cd homebrew-cullsnap
git add . && git commit -m "Add CullSnap cask"
git push
```

Users install via:
```bash
brew tap abhishekmitra-slg/cullsnap
brew install --cask cullsnap
```

## 9. Announce (Optional)

- **Reddit**: r/photography, r/golang, r/selfhosted
- **Hacker News**: "Show HN: CullSnap – Fast native photo culling tool (Go + React)"
- **Product Hunt**: Create a listing
- **X/Twitter**: Tag @AMAGILabs @AnthrCode (Wails community)
