module carrier

go 1.24.0

require (
	carrier/baseagent v0.0.0 // indirect
	carrier/codeagent v0.0.0 // indirect
	carrier/daemon v0.0.0
	carrier/gateway v0.0.0
	carrier/profilesync v0.0.0 // indirect
	carrier/shared v0.0.0
	carrier/webui v0.0.0
)

require golang.org/x/crypto v0.48.0

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/sys v0.41.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.66.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.38.2 // indirect
)

replace (
	carrier/baseagent => ./baseagent
	carrier/codeagent => ./codeagent
	carrier/daemon => ./daemon
	carrier/gateway => ./gateway
	carrier/profilesync => ./profilesync
	carrier/shared => ./shared
	carrier/webui => ./webui
)
