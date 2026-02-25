package config

// SecretStore abstracts encrypted credential storage.
// Phase 3: Currently all secrets are stored in plaintext config files.
// Future implementations will use platform-specific secure storage.
type SecretStore interface {
	// Get retrieves a secret by key.
	Get(key string) (string, error)
	// Set stores a secret for the given key.
	Set(key string, value string) error
	// Delete removes a secret by key.
	Delete(key string) error
	// Available returns whether this store backend is usable.
	Available() bool
}

// PlaintextStore implements SecretStore using the config file directly.
// This is the current default behavior (no encryption).
type PlaintextStore struct {
	configPath string
}

// NewPlaintextStore creates a plaintext secret store using the config file.
func NewPlaintextStore(configPath string) *PlaintextStore {
	return &PlaintextStore{configPath: configPath}
}

// Get returns an empty string — plaintext store reads from config directly.
func (p *PlaintextStore) Get(_ string) (string, error) {
	// Phase 3: Implement actual key lookup from config
	return "", nil
}

// Set is a no-op — plaintext store writes happen via config.Save().
func (p *PlaintextStore) Set(_ string, _ string) error {
	// Phase 3: Implement actual key storage
	return nil
}

// Delete is a no-op — plaintext store doesn't manage individual keys.
func (p *PlaintextStore) Delete(_ string) error {
	return nil
}

// Available returns true — plaintext store is always available.
func (p *PlaintextStore) Available() bool {
	return true
}

// KeychainStore uses macOS Keychain for secure storage.
// Phase 3 implementation placeholder.
// type KeychainStore struct{}

// LibsecretStore uses Linux D-Bus Secret Service for secure storage.
// Phase 3 implementation placeholder.
// type LibsecretStore struct{}
