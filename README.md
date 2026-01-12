# Yippity-Clippity

A macOS menubar application for transparent clipboard sharing between computers over shared drives (Dropbox, shared volumes).

## Features

- **Transparent Sync**: Clipboard changes are automatically synced to a shared location
- **Text & Images**: Supports plain text and images (PNG, JPEG)
- **Conflict Resolution**: Last-write-wins strategy for simultaneous changes
- **Menubar UI**: Native macOS menubar for easy access and configuration
- **Code Signed & Notarized**: Properly signed for macOS Gatekeeper

## Installation

### From Release

1. Download the latest `.dmg` from [Releases](https://github.com/mindmorass/yippity-clippity/releases)
2. Open the DMG and drag Yippity-Clippity to Applications
3. Launch from Applications folder

### From Source

```bash
# Clone the repository
git clone https://github.com/mindmorass/yippity-clippity.git
cd yippity-clippity

# Build
make build

# Or build and install to /Applications
make install
```

## Usage

1. Click the menubar icon
2. Select "Shared Location" → "Choose Folder..."
3. Select a folder on a shared drive (e.g., Dropbox, iCloud Drive, or SMB share)
4. Clipboard contents will now sync automatically

## Configuration

Configuration is stored in `~/.yippity-clippity/config.yaml`:

```yaml
shared_location: /Users/you/Dropbox/.yippity-clippity
launch_at_login: false
```

## How It Works

1. **Clipboard Monitoring**: Polls macOS NSPasteboard every 250ms for changes
2. **Sync Format**: Clipboard data is stored in a binary `.clip` format with JSON metadata
3. **Remote Watching**: Polls the shared location every 2 seconds for remote changes
4. **Conflict Resolution**: Uses timestamps to determine which change wins

## Requirements

- macOS 11.0 (Big Sur) or later
- A shared drive accessible from all machines (Dropbox, iCloud, SMB share, etc.)

## Development

### Prerequisites

- Go 1.23+
- Xcode Command Line Tools (for CGO)

### Building

```bash
# Build for current architecture
make build

# Build universal binary (arm64 + amd64)
make build-universal

# Create app bundle
make bundle

# Run tests
make test
```

### Project Structure

```
├── cmd/yippity-clippity/    # Application entry point
├── internal/
│   ├── app/                 # Application coordinator
│   ├── clipboard/           # macOS clipboard access (CGO)
│   ├── sync/                # Sync engine
│   ├── storage/             # File format and I/O
│   └── ui/                  # Menubar UI
├── assets/                  # App icons and Info.plist
├── entitlements/            # Hardened runtime entitlements
└── .github/workflows/       # CI/CD workflows
```

## GitHub Actions Secrets

For automated releases, configure these secrets:

| Secret | Description |
|--------|-------------|
| `MACOS_CERTIFICATE_BASE64` | Base64-encoded .p12 Developer ID certificate |
| `MACOS_CERTIFICATE_PASSWORD` | Password for .p12 file |
| `MACOS_DEVELOPER_ID` | Full cert name (e.g., "Developer ID Application: Name (TEAMID)") |
| `KEYCHAIN_PASSWORD` | Password for temporary keychain |
| `APPLE_ID` | Apple Developer account email |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `APPLE_APP_PASSWORD` | App-specific password for notarization |

## License

MIT
