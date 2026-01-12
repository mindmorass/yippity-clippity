package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"time"

	"fyne.io/systray"
	"github.com/mindmorass/yippity-clippity/internal/sync"
)

// App interface for the main application
type App interface {
	GetSyncEngine() *sync.Engine
	SetSharedLocation(path string) error
	GetSharedLocation() string
	Quit()
}

// Menubar manages the system tray
type Menubar struct {
	app         App
	mStatus     *systray.MenuItem
	mLastSync   *systray.MenuItem
	mPause      *systray.MenuItem
	mResume     *systray.MenuItem
	mLocations  *systray.MenuItem
	mCurrentLoc *systray.MenuItem
	quitChan    chan struct{}
}

// createClipboardIcon generates a simple clipboard icon for the menubar
func createClipboardIcon() []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw clipboard shape (black on transparent for template icon)
	black := color.RGBA{0, 0, 0, 255}

	// Clipboard body (rectangle)
	for x := 4; x < 18; x++ {
		for y := 5; y < 20; y++ {
			// Border
			if x == 4 || x == 17 || y == 5 || y == 19 {
				img.Set(x, y, black)
			}
		}
	}

	// Clipboard clip at top
	for x := 8; x < 14; x++ {
		img.Set(x, 3, black)
		img.Set(x, 4, black)
		img.Set(x, 5, black)
	}
	// Clip sides
	img.Set(7, 4, black)
	img.Set(7, 5, black)
	img.Set(14, 4, black)
	img.Set(14, 5, black)

	// Lines on clipboard (content)
	for x := 6; x < 16; x++ {
		img.Set(x, 9, black)
		img.Set(x, 12, black)
		img.Set(x, 15, black)
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
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
