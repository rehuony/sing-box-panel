module github.com/rehuony/sing-box-panel

go 1.26

tool (
	github.com/rehuony/sing-box-panel/internal/cmd/sign-release
	github.com/rehuony/sing-box-panel/internal/cmd/singbox-support
	github.com/rehuony/sing-box-panel/internal/cmd/third-party-notices
	github.com/rehuony/sing-box-panel/internal/cmd/verify-openapi
)

require (
	github.com/spf13/cobra v1.10.2
	github.com/tailscale/hujson v0.0.0-20260727124030-b80ff77dac4f
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/mod v0.38.0
	golang.org/x/sys v0.47.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	modernc.org/libc v1.75.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
