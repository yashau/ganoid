.PHONY: ui build build-windows build-linux build-darwin icons clean

VERSION := 0.1.0

# Build the SvelteKit UI into cmd/ganoidd/ui/dist (where ganoidd embeds it from)
ui:
	cd ui && pnpm install --frozen-lockfile && pnpm run build

# Generate Windows resource files (.syso) that embed the icon + version metadata.
# Requires: go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
# Also requires internal/tray/icon.ico to exist.
icons:
	cd cmd/ganoidd && goversioninfo -64 -o resource.syso versioninfo.json
	cd cmd/ganoid  && goversioninfo -64 -o resource.syso versioninfo.json

# Build both binaries for the host platform
build: ui
	go build -ldflags="-s -w" -o ganoidd ./cmd/ganoidd
	go build -ldflags="-s -w" -o ganoid  ./cmd/ganoid

# Windows build with embedded icon + version info.
# Run `make icons` first (requires goversioninfo and internal/tray/icon.ico).
build-windows: ui icons
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o ganoidd.exe ./cmd/ganoidd
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o ganoid.exe  ./cmd/ganoid

build-linux: ui
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ganoidd-linux ./cmd/ganoidd
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ganoid-linux  ./cmd/ganoid

build-darwin: ui
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o ganoidd-darwin ./cmd/ganoidd
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o ganoid-darwin  ./cmd/ganoid

# Quick Go-only build using the placeholder dist (no pnpm required)
build-noui:
	go build -ldflags="-s -w" -o ganoidd ./cmd/ganoidd
	go build -ldflags="-s -w" -o ganoid  ./cmd/ganoid

clean:
	rm -f ganoid ganoid.exe ganoid-linux ganoid-darwin
	rm -f ganoidd ganoidd.exe ganoidd-linux ganoidd-darwin
	rm -f cmd/ganoidd/resource.syso cmd/ganoid/resource.syso
	rm -rf cmd/ganoidd/ui/dist
	mkdir -p cmd/ganoidd/ui/dist
	echo '<html><body>Run make ui first.</body></html>' > cmd/ganoidd/ui/dist/index.html
