// Package config provides configuration structures for SDP components
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ControllerConfig represents Controller configuration
type ControllerConfig struct {
	Server ControllerServerConfig `yaml:"server"`
	Security ControllerSecurityConfig `yaml:"security"`
	TLS TLSCertConfig `yaml:"tls"`
	Storage StorageConfig `yaml:"storage"`
}

// ControllerServerConfig server settings
type ControllerServerConfig struct {
	SPAListen string `yaml:"spa_listen"` // UDP port for SPA
	APIListen string `yaml:"api_listen"` // HTTP port for API
}

// ControllerSecurityConfig security settings
type ControllerSecurityConfig struct {
	JWTSecret      string `yaml:"jwt_secret"`
	TokenExpiryMin int    `yaml:"token_expiry_min"`
}

// TLSCertConfig TLS certificate settings
type TLSCertConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// StorageConfig storage settings
type StorageConfig struct {
	DeviceRegistry string `yaml:"device_registry"`
}

// AppServerConfig represents Application Server configuration
type AppServerConfig struct {
	Server AppServerNetworkConfig `yaml:"server"`
	Security AppServerSecurityConfig `yaml:"security"`
	TLS TLSCertConfig `yaml:"tls"`
}

// AppServerNetworkConfig network settings
type AppServerNetworkConfig struct {
	ListenAddr string `yaml:"listen_addr"` // TCP port for mTLS
	SPAPort    int    `yaml:"spa_port"`    // UDP port for SPA
}

// AppServerSecurityConfig security settings
type AppServerSecurityConfig struct {
	RequireMTLS      bool   `yaml:"require_mtls"`
	ControllerAddr   string `yaml:"controller_addr"`
	AllowedDeviceIDs []string `yaml:"allowed_device_ids"`
}

// HostAgentConfig represents Host Agent configuration
type HostAgentConfig struct {
	Client HostAgentClientConfig `yaml:"client"`
	TLS    TLSCertConfig `yaml:"tls"`
}

// HostAgentClientConfig client settings
type HostAgentClientConfig struct {
	DeviceID       string `yaml:"device_id"`
	ControllerAddr string `yaml:"controller_addr"`
	ServerAddr     string `yaml:"server_addr"`
}

// LoadControllerConfig loads Controller configuration from file
func LoadControllerConfig(path string) (*ControllerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg ControllerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadAppServerConfig loads Application Server configuration from file
func LoadAppServerConfig(path string) (*AppServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg AppServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadHostAgentConfig loads Host Agent configuration from file
func LoadHostAgentConfig(path string) (*HostAgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg HostAgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DefaultControllerConfig returns default Controller configuration
func DefaultControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		Server: ControllerServerConfig{
			SPAListen: ":8442",
			APIListen: ":8080",
		},
		Security: ControllerSecurityConfig{
			JWTSecret:      "sdp-demo-secret-key-change-in-production",
			TokenExpiryMin: 10,
		},
		TLS: TLSCertConfig{
			CertFile: "./certs/server.pem",
			KeyFile:  "./certs/server.key",
			CAFile:   "./certs/ca.pem",
		},
		Storage: StorageConfig{
			DeviceRegistry: "./data/devices.json",
		},
	}
}

// DefaultAppServerConfig returns default Application Server configuration
func DefaultAppServerConfig() *AppServerConfig {
	return &AppServerConfig{
		Server: AppServerNetworkConfig{
			ListenAddr: ":8443",
			SPAPort:    8442,
		},
		Security: AppServerSecurityConfig{
			RequireMTLS:    true,
			ControllerAddr: "localhost:8080",
		},
		TLS: TLSCertConfig{
			CertFile: "./certs/server.pem",
			KeyFile:  "./certs/server.key",
			CAFile:   "./certs/ca.pem",
		},
	}
}

// DefaultHostAgentConfig returns default Host Agent configuration
func DefaultHostAgentConfig() *HostAgentConfig {
	return &HostAgentConfig{
		Client: HostAgentClientConfig{
			DeviceID:       "",
			ControllerAddr: "localhost:8080",
			ServerAddr:     "localhost:8443",
		},
		TLS: TLSCertConfig{
			CertFile: "./certs/client.pem",
			KeyFile:  "./certs/client.key",
			CAFile:   "./certs/ca.pem",
		},
	}
}
