package manifest

import "runtime"

// PlatformCommandSpec extends CommandSpec with optional platform overrides.
type PlatformCommandSpec struct {
	Command   string            `json:"command"`
	Platforms map[string]string `json:"platforms,omitempty"`
}

// Resolve returns the platform-specific command if available, falling back
// to the default command. The goos parameter should be runtime.GOOS or a
// test-provided value.
func (p PlatformCommandSpec) Resolve(goos string) string {
	if p.Platforms != nil {
		if cmd, ok := p.Platforms[goos]; ok {
			return cmd
		}
	}
	return p.Command
}

// ResolveForCurrentPlatform returns the command for the current OS.
func (p PlatformCommandSpec) ResolveForCurrentPlatform() string {
	return p.Resolve(runtime.GOOS)
}
