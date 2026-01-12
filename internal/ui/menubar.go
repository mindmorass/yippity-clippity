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

// createClipboardIcon generates a clipboard icon for the menubar (template style)
func createClipboardIcon() []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Template icon: black shapes on transparent background
	// macOS will automatically adjust for light/dark mode
	black := color.RGBA{0, 0, 0, 255}

	// Helper to draw rounded rectangle
	drawRoundedRect := func(x1, y1, x2, y2, radius int) {
		for x := x1; x <= x2; x++ {
			for y := y1; y <= y2; y++ {
				// Skip corners for rounded effect
				inCorner := false
				if x < x1+radius && y < y1+radius {
					// Top-left corner
					dx, dy := x1+radius-x, y1+radius-y
					inCorner = dx*dx+dy*dy > radius*radius
				} else if x > x2-radius && y < y1+radius {
					// Top-right corner
					dx, dy := x-(x2-radius), y1+radius-y
					inCorner = dx*dx+dy*dy > radius*radius
				} else if x < x1+radius && y > y2-radius {
					// Bottom-left corner
					dx, dy := x1+radius-x, y-(y2-radius)
					inCorner = dx*dx+dy*dy > radius*radius
				} else if x > x2-radius && y > y2-radius {
					// Bottom-right corner
					dx, dy := x-(x2-radius), y-(y2-radius)
					inCorner = dx*dx+dy*dy > radius*radius
				}
				if !inCorner {
					img.Set(x, y, black)
				}
			}
		}
	}

	// Clipboard body with rounded corners
	drawRoundedRect(4, 5, 17, 20, 2)

	// Clipboard clip at top (centered tab)
	for x := 8; x <= 13; x++ {
		for y := 2; y <= 6; y++ {
			img.Set(x, y, black)
		}
	}

	// Clip hole (transparent circle in the clip)
	transparent := color.RGBA{0, 0, 0, 0}
	for x := 9; x <= 12; x++ {
		for y := 3; y <= 4; y++ {
			img.Set(x, y, transparent)
		}
	}

	// Content lines (lighter/thinner to show "paper")
	lineColor := color.RGBA{0, 0, 0, 180}
	for x := 6; x <= 15; x++ {
		img.Set(x, 10, lineColor)
		img.Set(x, 13, lineColor)
		img.Set(x, 16, lineColor)
	}

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
	systray.SetIcon(createClipboardIcon())
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
		return
	}

	m.updateInfo = info

	if info.Available {
		m.mUpdate.SetTitle(fmt.Sprintf("Update Available: %s", info.LatestVersion))
		m.mUpdate.Show()
		log.Printf("Update available: %s -> %s", info.CurrentVersion, info.LatestVersion)
	} else {
		m.mUpdate.Hide()
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
