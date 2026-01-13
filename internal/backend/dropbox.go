package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mindmorass/yippity-clippity/internal/clipboard"
	"github.com/mindmorass/yippity-clippity/internal/storage"
	"golang.org/x/oauth2"
)

const (
	// DropboxFilePath is the file path in Dropbox
	DropboxFilePath = "/Apps/YippityClippity/current.clip"

	// Dropbox API endpoints
	dropboxContentAPI  = "https://content.dropboxapi.com/2"
	dropboxAPI         = "https://api.dropboxapi.com/2"
	dropboxAuthURL     = "https://www.dropbox.com/oauth2/authorize"
	dropboxTokenURL    = "https://api.dropboxapi.com/oauth2/token"

	// KeychainService is the service name for storing tokens
	KeychainService = "com.yippityclippity.dropbox"
)

// DropboxBackend implements Backend for Dropbox storage
type DropboxBackend struct {
	appKey       string
	appSecret    string
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
	lastRev      string
	lastHash     string
	httpClient   *http.Client
	oauthConfig  *oauth2.Config
}

// NewDropboxBackend creates a new Dropbox backend
func NewDropboxBackend(appKey, appSecret string) *DropboxBackend {
	return &DropboxBackend{
		appKey:    appKey,
		appSecret: appSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Type returns the backend type
func (b *DropboxBackend) Type() BackendType {
	return BackendDropbox
}

// GetLocation returns "dropbox" as the location identifier
func (b *DropboxBackend) GetLocation() string {
	if b.accessToken == "" {
		return ""
	}
	return "dropbox:" + DropboxFilePath
}

// SetLocation is not used for Dropbox (path is fixed)
func (b *DropboxBackend) SetLocation(location string) error {
	// For Dropbox, we use a fixed path
	return nil
}

// Init initializes the Dropbox backend
func (b *DropboxBackend) Init(ctx context.Context) error {
	if b.appKey == "" {
		return fmt.Errorf("Dropbox app key not configured")
	}

	// Initialize OAuth config
	b.oauthConfig = &oauth2.Config{
		ClientID:     b.appKey,
		ClientSecret: b.appSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  dropboxAuthURL,
			TokenURL: dropboxTokenURL,
		},
	}

	// Try to load tokens from keychain
	if err := b.loadTokensFromKeychain(); err != nil {
		// Tokens not found - will need OAuth flow
		return fmt.Errorf("Dropbox not authenticated: %w", err)
	}

	// Refresh token if expired
	if time.Now().After(b.tokenExpiry) {
		if err := b.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	return nil
}

// Close releases resources
func (b *DropboxBackend) Close() error {
	return nil
}

// Write stores clipboard content to Dropbox
func (b *DropboxBackend) Write(ctx context.Context, content *clipboard.Content) error {
	if b.accessToken == "" {
		return ErrNotConfigured
	}

	// Encode content
	data, err := storage.Encode(content)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	// Prepare upload args
	args := map[string]interface{}{
		"path":       DropboxFilePath,
		"mode":       "overwrite",
		"autorename": false,
		"mute":       true,
	}

	// Use update mode with rev for optimistic locking if we have a rev
	if b.lastRev != "" {
		args["mode"] = map[string]string{
			".tag":   "update",
			"update": b.lastRev,
		}
	}

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		dropboxContentAPI+"/files/upload",
		bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+b.accessToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Dropbox-API-Arg", string(argsJSON))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		// Conflict - another client modified the file
		return ErrConflict
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get new rev
	var uploadResp struct {
		Rev         string `json:"rev"`
		ContentHash string `json:"content_hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	b.lastRev = uploadResp.Rev
	b.lastHash = uploadResp.ContentHash

	return nil
}

// Read retrieves clipboard content from Dropbox
func (b *DropboxBackend) Read(ctx context.Context) (*clipboard.Content, error) {
	if b.accessToken == "" {
		return nil, ErrNotConfigured
	}

	args := map[string]string{
		"path": DropboxFilePath,
	}
	argsJSON, _ := json.Marshal(args)

	req, err := http.NewRequestWithContext(ctx, "POST",
		dropboxContentAPI+"/files/download",
		nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+b.accessToken)
	req.Header.Set("Dropbox-API-Arg", string(argsJSON))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		// File not found
		return nil, nil
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Get metadata from response header
	apiResult := resp.Header.Get("Dropbox-API-Result")
	if apiResult != "" {
		var meta struct {
			Rev         string `json:"rev"`
			ContentHash string `json:"content_hash"`
		}
		if json.Unmarshal([]byte(apiResult), &meta) == nil {
			b.lastRev = meta.Rev
			b.lastHash = meta.ContentHash
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body failed: %w", err)
	}

	content, err := storage.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	return content, nil
}

// GetModTime returns the last modification time from Dropbox metadata
func (b *DropboxBackend) GetModTime(ctx context.Context) (time.Time, error) {
	meta, err := b.getMetadata(ctx)
	if err != nil {
		return time.Time{}, err
	}

	return meta.ServerModified, nil
}

// GetChecksum returns the content_hash which is efficient for change detection
func (b *DropboxBackend) GetChecksum(ctx context.Context) (string, error) {
	meta, err := b.getMetadata(ctx)
	if err != nil {
		return "", err
	}

	b.lastRev = meta.Rev
	b.lastHash = meta.ContentHash

	return meta.ContentHash, nil
}

// Exists returns true if the file exists in Dropbox
func (b *DropboxBackend) Exists(ctx context.Context) bool {
	_, err := b.getMetadata(ctx)
	return err == nil
}

// dropboxMetadata represents file metadata from Dropbox
type dropboxMetadata struct {
	Rev            string    `json:"rev"`
	ContentHash    string    `json:"content_hash"`
	ServerModified time.Time `json:"server_modified"`
	Size           int64     `json:"size"`
}

// getMetadata retrieves file metadata from Dropbox
func (b *DropboxBackend) getMetadata(ctx context.Context) (*dropboxMetadata, error) {
	if b.accessToken == "" {
		return nil, ErrNotConfigured
	}

	args := map[string]string{
		"path": DropboxFilePath,
	}
	argsJSON, _ := json.Marshal(args)

	req, err := http.NewRequestWithContext(ctx, "POST",
		dropboxAPI+"/files/get_metadata",
		bytes.NewReader(argsJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+b.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		return nil, ErrNotFound
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get_metadata failed with status %d: %s", resp.StatusCode, string(body))
	}

	var meta dropboxMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// GetAuthURL returns the OAuth authorization URL for user authentication
func (b *DropboxBackend) GetAuthURL(state string) string {
	if b.oauthConfig == nil {
		b.oauthConfig = &oauth2.Config{
			ClientID:     b.appKey,
			ClientSecret: b.appSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  dropboxAuthURL,
				TokenURL: dropboxTokenURL,
			},
		}
	}

	// Use PKCE for better security
	return b.oauthConfig.AuthCodeURL(state,
		oauth2.SetAuthURLParam("token_access_type", "offline"),
	)
}

// ExchangeCode exchanges an authorization code for tokens
func (b *DropboxBackend) ExchangeCode(ctx context.Context, code string) error {
	token, err := b.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to exchange code: %w", err)
	}

	b.accessToken = token.AccessToken
	b.refreshToken = token.RefreshToken
	b.tokenExpiry = token.Expiry

	// Save tokens to keychain
	return b.saveTokensToKeychain()
}

// SetTokens sets the OAuth tokens directly (for testing or migration)
func (b *DropboxBackend) SetTokens(accessToken, refreshToken string, expiry time.Time) {
	b.accessToken = accessToken
	b.refreshToken = refreshToken
	b.tokenExpiry = expiry
}

// IsAuthenticated returns true if the backend has valid tokens
func (b *DropboxBackend) IsAuthenticated() bool {
	return b.accessToken != ""
}

// refreshAccessToken refreshes the access token using the refresh token
func (b *DropboxBackend) refreshAccessToken(ctx context.Context) error {
	if b.refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	token := &oauth2.Token{
		RefreshToken: b.refreshToken,
	}

	tokenSource := b.oauthConfig.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return err
	}

	b.accessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		b.refreshToken = newToken.RefreshToken
	}
	b.tokenExpiry = newToken.Expiry

	return b.saveTokensToKeychain()
}

// loadTokensFromKeychain loads OAuth tokens from the macOS keychain
func (b *DropboxBackend) loadTokensFromKeychain() error {
	// Try to load from keychain
	item, err := loadFromKeychain(KeychainService, "tokens")
	if err != nil {
		return err
	}

	var tokens struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		Expiry       time.Time `json:"expiry"`
	}

	if err := json.Unmarshal(item, &tokens); err != nil {
		return err
	}

	b.accessToken = tokens.AccessToken
	b.refreshToken = tokens.RefreshToken
	b.tokenExpiry = tokens.Expiry

	return nil
}

// saveTokensToKeychain saves OAuth tokens to the macOS keychain
func (b *DropboxBackend) saveTokensToKeychain() error {
	tokens := struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		Expiry       time.Time `json:"expiry"`
	}{
		AccessToken:  b.accessToken,
		RefreshToken: b.refreshToken,
		Expiry:       b.tokenExpiry,
	}

	data, err := json.Marshal(tokens)
	if err != nil {
		return err
	}

	return saveToKeychain(KeychainService, "tokens", data)
}

// ClearTokens removes stored tokens (for logout)
func (b *DropboxBackend) ClearTokens() error {
	b.accessToken = ""
	b.refreshToken = ""
	b.tokenExpiry = time.Time{}
	return deleteFromKeychain(KeychainService, "tokens")
}

// Helper function to check for 409 conflict error path
func isDropboxNotFound(body []byte) bool {
	var errResp struct {
		Error struct {
			Tag  string `json:".tag"`
			Path struct {
				Tag string `json:".tag"`
			} `json:"path"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		return errResp.Error.Path.Tag == "not_found"
	}
	return strings.Contains(string(body), "not_found")
}
