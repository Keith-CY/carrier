module carrier

go 1.23.8

require carrier/daemon v0.0.0

require carrier/webui v0.0.0 // indirect

replace (
	carrier/daemon => ./daemon
	carrier/webui => ./webui
)
