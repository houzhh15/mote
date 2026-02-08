// Package skills provides the skill system for Mote Agent Runtime.
package skills

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Gating checks whether a skill should be included based on environment requirements.
type Gating struct{}

// NewGating creates a new Gating instance.
func NewGating() *Gating {
	return &Gating{}
}

// ShouldInclude returns true if the skill entry passes all gating checks.
func (g *Gating) ShouldInclude(entry *SkillEntry) bool {
	if entry == nil {
		return false
	}

	// Manifest.json skills are always included (they don't have gating metadata)
	if entry.Source == SourceManifest {
		return true
	}

	// SKILL.md skills need metadata checks
	if entry.Source == SourceSkillMD && entry.Metadata != nil {
		return g.checkMetadata(entry.Metadata)
	}

	// Default: include if no metadata
	return true
}

// checkMetadata validates the skill requirements from SKILL.md metadata.
func (g *Gating) checkMetadata(meta *SkillMDMetadata) bool {
	// Check OS requirements
	if len(meta.OS) > 0 {
		if !g.checkOS(meta.OS) {
			return false
		}
	}

	// Check requirements if present
	if meta.Requires != nil {
		if !g.checkRequirements(meta.Requires) {
			return false
		}
	}

	return true
}

// checkOS verifies if the current OS matches any of the required OS values.
func (g *Gating) checkOS(requiredOS []string) bool {
	currentOS := runtime.GOOS
	for _, os := range requiredOS {
		osLower := strings.ToLower(os)
		// Handle aliases
		switch osLower {
		case "mac", "macos":
			if currentOS == "darwin" {
				return true
			}
		case "win", "windows":
			if currentOS == "windows" {
				return true
			}
		default:
			if strings.EqualFold(osLower, currentOS) {
				return true
			}
		}
	}
	return false
}

// checkRequirements validates all skill requirements.
func (g *Gating) checkRequirements(req *SkillRequirements) bool {
	// Check required binaries (all must exist)
	for _, bin := range req.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}

	// Check anyBins (at least one must exist)
	if len(req.AnyBins) > 0 {
		found := false
		for _, bin := range req.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check required environment variables
	for _, env := range req.Env {
		if os.Getenv(env) == "" {
			return false
		}
	}

	// Check required paths (all must exist)
	if !g.checkPaths(req.Paths) {
		return false
	}

	// Config checks would require access to Mote's config system
	// For now, we skip config checks (they could be added later)

	return true
}

// checkPaths verifies if all required paths exist.
// Paths are expanded for environment variables before checking.
func (g *Gating) checkPaths(paths []string) bool {
	for _, p := range paths {
		expanded := os.ExpandEnv(p)
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// CheckBinaryExists returns true if the given binary is available in PATH.
func CheckBinaryExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// CheckEnvExists returns true if the environment variable is set and non-empty.
func CheckEnvExists(name string) bool {
	return os.Getenv(name) != ""
}
