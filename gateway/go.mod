module carrier/gateway

go 1.23

toolchain go1.23.0

require (
	carrier/baseagent v0.0.0
	carrier/daemon v0.0.0
	carrier/shared v0.0.0
	github.com/google/uuid v1.6.0
)

replace (
	carrier/baseagent => ../baseagent
	carrier/daemon => ../daemon
	carrier/shared => ../shared
)
