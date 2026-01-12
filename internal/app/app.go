package app

import (
	"log"

	"github.com/mindmorass/yippity-clippity/internal/sync"
	"github.com/mindmorass/yippity-clippity/internal/ui"
)

// App is the main application
type App struct {
	config     *Config
	syncEngine *sync.Engine
	menubar    *ui.Menubar
	quitChan   chan struct{}
}

// New creates a new application instance
func New() (*App, error) {
	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Printf("Warning: failed to load config: %v", err)
		config = DefaultConfig()
	}

	// Create sync engine
	engine := sync.NewEngine(config.SharedLocation)

	app := &App{
		config:     config,
		syncEngine: engine,
		quitChan:   make(chan struct{}),
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
