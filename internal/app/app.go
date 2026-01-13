package app

import (
	"context"
	"log"

	"github.com/mindmorass/yippity-clippity/internal/backend"
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

	// Create backend based on configuration
	backendCfg := &backend.Config{
		Type:             backend.BackendType(config.BackendType),
		Location:         config.SharedLocation,
		S3Bucket:         config.S3Bucket,
		S3Prefix:         config.S3Prefix,
		S3Region:         config.S3Region,
		DropboxAppKey:    config.DropboxAppKey,
		DropboxAppSecret: config.DropboxAppSecret,
	}

	// Default to local backend if not specified
	if backendCfg.Type == "" {
		backendCfg.Type = backend.BackendLocal
	}

	b, err := backend.New(backendCfg)
	if err != nil {
		log.Printf("Warning: failed to create backend: %v, falling back to local", err)
		b = backend.NewDefault()
	}

	// Initialize backend
	ctx := context.Background()
	if err := b.Init(ctx); err != nil {
		log.Printf("Warning: failed to initialize backend: %v", err)
		// For local backend, this might just mean the directory doesn't exist yet
	}

	// Create sync engine with the backend
	engine := sync.NewEngineWithBackend(b)

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

// GetBackendType returns the current backend type
func (a *App) GetBackendType() string {
	return a.config.BackendType
}

// SetBackendType updates the backend type (requires restart to take effect)
func (a *App) SetBackendType(backendType string) error {
	a.config.BackendType = backendType
	if err := SaveConfig(a.config); err != nil {
		log.Printf("Warning: failed to save config: %v", err)
		return err
	}
	return nil
}
