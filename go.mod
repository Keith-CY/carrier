module carrier

go 1.23.8

require (
	carrier/daemon v0.0.0
	carrier/webui v0.0.0
)

replace (
	carrier/daemon => ./daemon
	carrier/webui => ./webui
)
