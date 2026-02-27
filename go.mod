module carrier

go 1.23.8

require (
	carrier/baseagent v0.0.0 // indirect
	carrier/codeagent v0.0.0 // indirect
	carrier/daemon v0.0.0
	carrier/gateway v0.0.0
	carrier/profilesync v0.0.0 // indirect
	carrier/shared v0.0.0 // indirect
	carrier/webui v0.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
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
