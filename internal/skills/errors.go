package skills

import "errors"

// Skill system errors.
var (
	// ErrSkillNotFound is returned when a skill cannot be found by ID.
	ErrSkillNotFound = errors.New("skill not found")

	// ErrSkillAlreadyActive is returned when trying to activate an already active skill.
	ErrSkillAlreadyActive = errors.New("skill already active")

	// ErrSkillNotActive is returned when trying to deactivate an inactive skill.
	ErrSkillNotActive = errors.New("skill not active")

	// ErrManifestInvalid is returned when a manifest.json file is invalid.
	ErrManifestInvalid = errors.New("invalid manifest")

	// ErrDependencyMissing is returned when a required dependency is not active.
	ErrDependencyMissing = errors.New("dependency not active")

	// ErrConfigInvalid is returned when skill configuration is invalid.
	ErrConfigInvalid = errors.New("invalid configuration")

	// ErrToolHandlerInvalid is returned when a tool handler format is invalid.
	ErrToolHandlerInvalid = errors.New("invalid tool handler format")

	// ErrPromptInvalid is returned when a prompt definition is invalid.
	ErrPromptInvalid = errors.New("invalid prompt definition")

	// ErrSkillIDInvalid is returned when a skill ID doesn't match the required pattern.
	ErrSkillIDInvalid = errors.New("invalid skill ID format")

	// ErrVersionInvalid is returned when a version string is invalid.
	ErrVersionInvalid = errors.New("invalid version format")
)
