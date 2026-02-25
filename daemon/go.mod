module carrier/daemon

go 1.23

toolchain go1.23.0

require (
	carrier/baseagent v0.0.0
	carrier/shared v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	carrier/baseagent => ../baseagent
	carrier/shared => ../shared
)
