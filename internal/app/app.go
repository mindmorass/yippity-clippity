package app

import (
	"log"

	"github.com/mindmorass/yippity-clippity/internal/sync"
	"github.com/mindmorass/yippity-clippity/internal/ui"
	"github.com/mindmorass/yippity-clippity/internal/update"
)

// App is the main application
type App struct {
	config        *Config
	syncEngine    *sync.Engine
	menubar       *ui.Menubar
	updateChecker *update.Checker
	version       string
	quitChan      chan struct{}
}

// New creates a new application instance
func New(version string) (*App, error) {
	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Printf("Warning: failed to load config: %v", err)
		config = DefaultConfig()
	}

	// Create sync engine
	engine := sync.NewEngine(config.SharedLocation)

	// Create update checker
	checker := update.NewChecker(version)

	app := &App{
		config:        config,
		syncEngine:    engine,
		updateChecker: checker,
		version:       version,
		quitChan:      make(chan struct{}),
	}

	// Create menubar
	app.menubar = ui.NewMenubar(app)

	return app, nil
}

// Run starts the application
func (a *App) Run() error {
	// Start sync engine
	if err := a.syncEngine.Start(); err != nil {
		log.Printf("Warning: failed to start sync engine: %v", err)
	}

	// Run menubar (blocking)
	a.menubar.Run()

	return nil
}

// GetSyncEngine returns the sync engine
func (a *App) GetSyncEngine() *sync.Engine {
	return a.syncEngine
}

// SetSharedLocation updates the shared location
func (a *App) SetSharedLocation(path string) error {
	if err := a.syncEngine.SetSharedLocation(path); err != nil {
		return err
	}

	// Update config
	a.config.SharedLocation = path
	if err := SaveConfig(a.config); err != nil {
		log.Printf("Warning: failed to save config: %v", err)
	}

	return nil
}

// GetSharedLocation returns the current shared location
func (a *App) GetSharedLocation() string {
	return a.syncEngine.GetSharedLocation()
}

// Quit stops the application
func (a *App) Quit() {
	a.syncEngine.Stop()
	a.menubar.Quit()
	close(a.quitChan)
}

// GetVersion returns the application version
func (a *App) GetVersion() string {
	return a.version
}

// GetUpdateChecker returns the update checker
func (a *App) GetUpdateChecker() *update.Checker {
	return a.updateChecker
}
