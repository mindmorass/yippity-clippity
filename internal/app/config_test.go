//go:build darwin

package app

import (
	"strings"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "local backend no fields required",
			config:  Config{BackendType: "local"},
			wantErr: false,
		},
		{
			name:    "empty backend_type treated as local",
			config:  Config{BackendType: ""},
			wantErr: false,
		},
		{
			name:    "s3 backend with bucket ok",
			config:  Config{BackendType: "s3", S3Bucket: "my-bucket"},
			wantErr: false,
		},
		{
			name:    "s3 backend missing bucket",
			config:  Config{BackendType: "s3", S3Bucket: ""},
			wantErr: true,
			errMsg:  "s3_bucket",
		},
		{
			name:    "dropbox backend with credentials ok",
			config:  Config{BackendType: "dropbox", DropboxAppKey: "key", DropboxAppSecret: "secret"},
			wantErr: false,
		},
		{
			name:    "dropbox backend missing app key",
			config:  Config{BackendType: "dropbox", DropboxAppKey: "", DropboxAppSecret: "secret"},
			wantErr: true,
			errMsg:  "dropbox_app_key",
		},
		{
			name:    "dropbox backend missing app secret",
			config:  Config{BackendType: "dropbox", DropboxAppKey: "key", DropboxAppSecret: ""},
			wantErr: true,
			errMsg:  "dropbox_app_secret",
		},
		{
			name:    "unknown backend type",
			config:  Config{BackendType: "ftp"},
			wantErr: true,
			errMsg:  "unknown backend_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || len(err.Error()) == 0 {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if got := err.Error(); !contains(got, tt.errMsg) {
					t.Errorf("error message %q does not contain expected %q", got, tt.errMsg)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
