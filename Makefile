.PHONY: all build clean run test bundle sign install uninstall

APP_NAME := yippity-clippity
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR := dist
BUNDLE_DIR := $(BUILD_DIR)/Yippity-Clippity.app

LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION)"

all: build

build:
	@echo "Building for arm64..."
	CGO_ENABLED=1 GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

clean:
	rm -rf $(BUILD_DIR)

run: build
	./$(BUILD_DIR)/$(APP_NAME)

test:
	go test -v -race ./...

deps:
	go mod download
	go mod tidy

bundle: build
	@echo "Creating app bundle..."
	mkdir -p $(BUNDLE_DIR)/Contents/{MacOS,Resources}
	cp $(BUILD_DIR)/$(APP_NAME) $(BUNDLE_DIR)/Contents/MacOS/$(APP_NAME)
	cp assets/Info.plist $(BUNDLE_DIR)/Contents/
	@if [ -f assets/icons/icon.icns ]; then \
		cp assets/icons/icon.icns $(BUNDLE_DIR)/Contents/Resources/; \
	fi
	@# Update version in Info.plist
	@/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $(VERSION)" $(BUNDLE_DIR)/Contents/Info.plist 2>/dev/null || true
	@/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $(VERSION)" $(BUNDLE_DIR)/Contents/Info.plist 2>/dev/null || true
	@echo "Bundle created at $(BUNDLE_DIR)"

sign: bundle
	@echo "Signing app bundle..."
	@if [ -z "$(DEVELOPER_ID)" ]; then \
		echo "Error: DEVELOPER_ID not set"; \
		exit 1; \
	fi
	codesign --force --deep --sign "$(DEVELOPER_ID)" \
		--options runtime \
		--entitlements entitlements/entitlements.plist \
		$(BUNDLE_DIR)
	codesign --verify --verbose=4 $(BUNDLE_DIR)

install: bundle
	@echo "Installing to /Applications..."
	rm -rf /Applications/Yippity-Clippity.app
	cp -R $(BUNDLE_DIR) /Applications/
	@echo "Installed successfully"

uninstall:
	rm -rf /Applications/Yippity-Clippity.app
	rm -rf ~/Library/Application\ Support/yippity-clippity
	rm -rf ~/.yippity-clippity
	@echo "Uninstalled successfully"
