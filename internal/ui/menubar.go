package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/systray"
	"github.com/mindmorass/yippity-clippity/internal/sync"
	"github.com/mindmorass/yippity-clippity/internal/update"
)

// App interface for the main application
type App interface {
	GetSyncEngine() *sync.Engine
	SetSharedLocation(path string) error
	GetSharedLocation() string
	GetVersion() string
	GetUpdateChecker() *update.Checker
	Quit()
}

// Menubar manages the system tray
type Menubar struct {
	app            App
	mStatus        *systray.MenuItem
	mLastSync      *systray.MenuItem
	mPause         *systray.MenuItem
	mResume        *systray.MenuItem
	mLocations     *systray.MenuItem
	mCurrentLoc    *systray.MenuItem
	mUpdate        *systray.MenuItem
	mCheckUpdate   *systray.MenuItem
	mVersion       *systray.MenuItem
	updateInfo     *update.UpdateInfo
	quitChan       chan struct{}
}

// createClipboardIcon generates a line-style clipboard icon for the menubar
// Uses outline style (not filled) for a clean, modern look
func createClipboardIcon() []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Template icon: black lines on transparent background
	// macOS will automatically invert for dark menu bar
	black := color.RGBA{0, 0, 0, 255}

	// Helper to draw a line
	drawLine := func(x1, y1, x2, y2 int) {
		if x1 == x2 {
			// Vertical line
			for y := y1; y <= y2; y++ {
				img.Set(x1, y, black)
			}
		} else {
			// Horizontal line
			for x := x1; x <= x2; x++ {
				img.Set(x, y1, black)
			}
		}
	}

	// Clipboard body outline (rectangle)
	// Left edge
	drawLine(4, 6, 4, 20)
	// Right edge
	drawLine(17, 6, 17, 20)
	// Bottom edge
	drawLine(4, 20, 17, 20)
	// Top edge (with gap for clip)
	drawLine(4, 6, 7, 6)
	drawLine(14, 6, 17, 6)

	// Clipboard clip at top
	// Clip body outline
	drawLine(7, 3, 14, 3)   // Top of clip
	drawLine(7, 3, 7, 6)    // Left side of clip
	drawLine(14, 3, 14, 6)  // Right side of clip
	// Inner clip detail (the grip hole)
	drawLine(9, 4, 12, 4)
	drawLine(9, 5, 12, 5)

	// Content lines (horizontal lines representing text)
	drawLine(6, 10, 15, 10)
	drawLine(6, 13, 15, 13)
	drawLine(6, 16, 12, 16) // Shorter line for variety

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// NewMenubar creates a new menubar
func NewMenubar(app App) *Menubar {
	return &Menubar{
		app:      app,
		quitChan: make(chan struct{}),
	}
}

// Run starts the menubar (blocking)
func (m *Menubar) Run() {
	systray.Run(m.onReady, m.onExit)
}

// Quit signals the menubar to exit
func (m *Menubar) Quit() {
	systray.Quit()
}

func (m *Menubar) onReady() {
	iconData := createClipboardIcon()
	systray.SetTemplateIcon(iconData, iconData)
	systray.SetTitle("")
	systray.SetTooltip("Yippity-Clippity")

	// Status items
	m.mStatus = systray.AddMenuItem("Status: Starting...", "")
	m.mStatus.Disable()

	m.mLastSync = systray.AddMenuItem("Last sync: Never", "")
	m.mLastSync.Disable()

	systray.AddSeparator()

	// Location submenu
	m.mLocations = systray.AddMenuItem("Shared Location", "Select sync folder")
	m.mCurrentLoc = m.mLocations.AddSubMenuItem("Not configured", "")
	m.mCurrentLoc.Disable()
	m.mLocations.AddSubMenuItem("", "")
	mChooseFolder := m.mLocations.AddSubMenuItem("Choose Folder...", "")

	systray.AddSeparator()

	// Sync controls
	m.mPause = systray.AddMenuItem("Pause Sync", "")
	m.mResume = systray.AddMenuItem("Resume Sync", "")
	m.mResume.Hide()

	systray.AddSeparator()

	// Update section
	m.mUpdate = systray.AddMenuItem("Update Available!", "A new version is available")
	m.mUpdate.Hide() // Hidden until update is found
	m.mCheckUpdate = systray.AddMenuItem("Check for Updates", "")
	m.mVersion = systray.AddMenuItem("Version: "+m.app.GetVersion(), "")
	m.mVersion.Disable()

	systray.AddSeparator()

	// About and Quit
	mAbout := systray.AddMenuItem("About Yippity-Clippity", "")
	mQuit := systray.AddMenuItem("Quit", "")

	// Update initial state
	m.updateLocation()
	m.updateStatus(sync.StatusSyncing)

	// Set up status change handler
	m.app.GetSyncEngine().OnStatusChange(func(status sync.Status) {
		m.updateStatus(status)
	})

	// Start last sync time updater
	go m.updateLastSyncLoop()

	// Check for updates on startup and periodically
	go m.checkForUpdates()
	go m.updateCheckLoop()

	// Handle menu events
	go func() {
		for {
			select {
			case <-mChooseFolder.ClickedCh:
				path := ShowFolderPicker()
				if path != "" {
					if err := m.app.SetSharedLocation(path); err != nil {
						// TODO: Show error notification
						continue
					}
					m.updateLocation()
				}

			case <-m.mPause.ClickedCh:
				m.app.GetSyncEngine().Pause()
				m.mPause.Hide()
				m.mResume.Show()

			case <-m.mResume.ClickedCh:
				m.app.GetSyncEngine().Resume()
				m.mResume.Hide()
				m.mPause.Show()

			case <-m.mCheckUpdate.ClickedCh:
				m.checkForUpdates()

			case <-m.mUpdate.ClickedCh:
				// Open release page in browser
				if m.updateInfo != nil && m.updateInfo.ReleaseURL != "" {
					openBrowser(m.updateInfo.ReleaseURL)
				}

			case <-mAbout.ClickedCh:
				// TODO: Show about dialog
				continue

			case <-mQuit.ClickedCh:
				m.app.Quit()
				return

			case <-m.quitChan:
				return
			}
		}
	}()
}

func (m *Menubar) onExit() {
	close(m.quitChan)
}

func (m *Menubar) updateStatus(status sync.Status) {
	switch status {
	case sync.StatusSyncing:
		m.mStatus.SetTitle("Status: Syncing ✓")
	case sync.StatusPaused:
		m.mStatus.SetTitle("Status: Paused ⏸")
	case sync.StatusError:
		m.mStatus.SetTitle("Status: Error ⚠")
	default:
		m.mStatus.SetTitle("Status: " + status.String())
	}
}

func (m *Menubar) updateLocation() {
	loc := m.app.GetSharedLocation()
	if loc == "" {
		m.mCurrentLoc.SetTitle("Not configured")
	} else {
		m.mCurrentLoc.SetTitle("✓ " + loc)
	}
}

func (m *Menubar) updateLastSyncLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lastSync := m.app.GetSyncEngine().GetLastSyncTime()
			if lastSync.IsZero() {
				m.mLastSync.SetTitle("Last sync: Never")
			} else {
				ago := time.Since(lastSync)
				m.mLastSync.SetTitle(fmt.Sprintf("Last sync: %s ago", formatDuration(ago)))
			}
		case <-m.quitChan:
			return
		}
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	} else {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
}

func (m *Menubar) checkForUpdates() {
	checker := m.app.GetUpdateChecker()
	if checker == nil {
		return
	}

	info, err := checker.Check()
	if err != nil {
		log.Printf("Update check failed: %v", err)
		m.mUpdate.SetTitle("Update check failed")
		m.mUpdate.Show()
		// Hide the error after a few seconds
		go func() {
			time.Sleep(3 * time.Second)
			if m.updateInfo == nil || !m.updateInfo.Available {
				m.mUpdate.Hide()
			}
		}()
		return
	}

	m.updateInfo = info

	if info.Available {
		m.mUpdate.SetTitle(fmt.Sprintf("Update Available: %s", info.LatestVersion))
		m.mUpdate.Show()
		log.Printf("Update available: %s -> %s", info.CurrentVersion, info.LatestVersion)
	} else {
		// Show "up to date" briefly so user knows the check completed
		m.mUpdate.SetTitle("Up to date ✓")
		m.mUpdate.Show()
		go func() {
			time.Sleep(3 * time.Second)
			if m.updateInfo != nil && !m.updateInfo.Available {
				m.mUpdate.Hide()
			}
		}()
	}
}

func (m *Menubar) updateCheckLoop() {
	// Initial delay before first check
	time.Sleep(5 * time.Second)
	m.checkForUpdates()

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkForUpdates()
		case <-m.quitChan:
			return
		}
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		log.Printf("Unsupported platform for opening browser")
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}
