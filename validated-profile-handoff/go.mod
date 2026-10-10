module github.com/runtimeconditions/rc-demos/validated-profile-handoff

go 1.25.0

require (
	github.com/runtimeconditions/go-rc-profiler v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/runtimeconditions/go-rc-profiler => ../../go-rc-profiler
