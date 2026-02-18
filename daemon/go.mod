module carrier/daemon

go 1.23

toolchain go1.23.0

require (
	carrier/webui v0.0.0
	github.com/google/uuid v1.6.0 // indirect
)

replace carrier/webui => ../webui
