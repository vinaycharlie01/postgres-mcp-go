package infrastructure

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the complete application configuration.
type Config struct {
	// Database connection configuration
	Database DatabaseConfig `yaml:"database"`

	// Always Encrypted configuration
	Encryption EncryptionConfig `yaml:"encryption"`

	// Query safety / limit configuration
	Query QueryConfig `yaml:"query"`

	// Connection pool configuration
	Pool PoolConfig `yaml:"pool"`

	// Logging configuration
	Logging LoggingConfig `yaml:"logging"`

	// MCP configuration
	MCP MCPConfig `yaml:"mcp"`
}

// DatabaseConfig holds Microsoft SQL Server connection settings.
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`

	// Encrypt controls TLS for the wire protocol: "true", "false" or "disable".
	Encrypt string `yaml:"encrypt"`

	// TrustServerCertificate skips server certificate validation when true.
	TrustServerCertificate bool `yaml:"trust_server_certificate"`

	// ConnectionTimeout is the login timeout in seconds.
	ConnectionTimeout int `yaml:"connection_timeout"`
}

// EncryptionConfig holds Always Encrypted settings.
type EncryptionConfig struct {
	// Enabled turns on column encryption (Always Encrypted) for the connection.
	Enabled bool `yaml:"enabled"`

	// KeyPath is the absolute path to the PFX certificate that holds the
	// column master key used to decrypt column encryption keys.
	KeyPath string `yaml:"key_path"`

	// KeyPassword is the password protecting the PFX certificate.
	KeyPassword string `yaml:"key_password"`
}

// QueryConfig holds read-only query safety limits.
type QueryConfig struct {
	// MaxLength is the maximum number of characters allowed in a query.
	MaxLength int `yaml:"max_length"`

	// MaxRows is the maximum number of rows returned to the caller.
	MaxRows int `yaml:"max_rows"`

	// Timeout is the maximum execution time for a single query.
	Timeout Duration `yaml:"timeout"`
}

// PoolConfig holds connection pool tuning.
type PoolConfig struct {
	MaxOpenConns    int      `yaml:"max_open_conns"`
	MaxIdleConns    int      `yaml:"max_idle_conns"`
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
}

// Duration is a time.Duration that unmarshals from either a Go duration string
// (e.g. "30s", "1h") or a plain number of seconds. This keeps config.yaml
// human-friendly while still mapping cleanly to time.Duration.
type Duration time.Duration

// AsDuration returns the value as a standard time.Duration.
func (d Duration) AsDuration() time.Duration { return time.Duration(d) }

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil && s != "" {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var secs int64
	if err := value.Decode(&secs); err != nil {
		return fmt.Errorf("duration must be a string (e.g. \"30s\") or a number of seconds: %w", err)
	}
	*d = Duration(time.Duration(secs) * time.Second)
	return nil
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`

	// File, when set, is a path to which logs are written (in addition to the
	// console stream). Useful with stdio transport, where stderr is otherwise
	// the only sink. The file is created if missing and appended to.
	File string `yaml:"file"`
}

// MCPConfig holds MCP-specific configuration.
type MCPConfig struct {
	Transport string `yaml:"transport"`
	Port      int    `yaml:"port"`
}

// Default values for the configuration.
const (
	defaultPort              = 1433
	defaultEncrypt           = "true"
	defaultConnectionTimeout = 30
	defaultMaxQueryLength    = 5000
	defaultMaxRows           = 1000
	defaultQueryTimeout      = Duration(30 * time.Second)
	defaultMaxOpenConns      = 10
	defaultMaxIdleConns      = 5
	defaultConnMaxLifetime   = Duration(time.Hour)
	defaultLogLevel          = "info"
	defaultLogFormat         = "json"
	defaultTransport         = "stdio"
	defaultMCPPort           = 9090
)

// LoadConfig loads configuration from a YAML file when present, otherwise it
// falls back to environment variables. Environment variables always override
// individual values that are set, so secrets can be injected at runtime.
func LoadConfig() (*Config, error) {
	configPath := getConfigPath()

	var config *Config
	if _, err := os.Stat(configPath); err == nil {
		loaded, err := loadFromYAML(configPath)
		if err != nil {
			return nil, err
		}
		config = loaded
	} else {
		config = &Config{}
	}

	// Overlay environment variables on top of the YAML (or empty) config.
	applyEnvOverrides(config)

	// Apply defaults for anything still unset.
	applyDefaults(config)

	return config, nil
}

// getConfigPath returns the path to the config file.
func getConfigPath() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(homeDir, ".mcp-mssql", "config.yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "config.yaml"
}

// loadFromYAML loads configuration from a YAML file.
func loadFromYAML(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return config, nil
}

// applyEnvOverrides overlays environment variables onto the config. Only set
// values override; empty environment variables are ignored.
func applyEnvOverrides(config *Config) {
	// Database
	if v := os.Getenv("SQLSERVER_HOST"); v != "" {
		config.Database.Host = v
	}
	if v, ok := lookupInt("SQLSERVER_PORT"); ok {
		config.Database.Port = v
	}
	if v := os.Getenv("SQLSERVER_USER"); v != "" {
		config.Database.User = v
	}
	if v := os.Getenv("SQLSERVER_PASSWORD"); v != "" {
		config.Database.Password = v
	}
	if v := os.Getenv("SQLSERVER_DB"); v != "" {
		config.Database.Database = v
	}
	if v := os.Getenv("SQLSERVER_ENCRYPT"); v != "" {
		config.Database.Encrypt = v
	}
	if v, ok := lookupBool("SQLSERVER_TRUST_SERVER_CERTIFICATE"); ok {
		config.Database.TrustServerCertificate = v
	}
	if v, ok := lookupInt("SQLSERVER_CONNECTION_TIMEOUT"); ok {
		config.Database.ConnectionTimeout = v
	}

	// Always Encrypted
	if v, ok := lookupBool("SQLSERVER_ALWAYS_ENCRYPTED_ENABLED"); ok {
		config.Encryption.Enabled = v
	}
	if v := os.Getenv("SQLSERVER_ALWAYS_ENCRYPTED_KEY_PATH"); v != "" {
		config.Encryption.KeyPath = v
	}
	if v := os.Getenv("SQLSERVER_ALWAYS_ENCRYPTED_KEY_PASSWORD"); v != "" {
		config.Encryption.KeyPassword = v
	}

	// Query limits
	if v, ok := lookupInt("QUERY_MAX_LENGTH"); ok {
		config.Query.MaxLength = v
	}
	if v, ok := lookupInt("QUERY_MAX_ROWS"); ok {
		config.Query.MaxRows = v
	}
	if v, ok := lookupInt("QUERY_TIMEOUT_SECONDS"); ok {
		config.Query.Timeout = Duration(time.Duration(v) * time.Second)
	}

	// Pool
	if v, ok := lookupInt("POOL_MAX_OPEN_CONNS"); ok {
		config.Pool.MaxOpenConns = v
	}
	if v, ok := lookupInt("POOL_MAX_IDLE_CONNS"); ok {
		config.Pool.MaxIdleConns = v
	}
	if v, ok := lookupInt("POOL_CONN_MAX_LIFETIME_SECONDS"); ok {
		config.Pool.ConnMaxLifetime = Duration(time.Duration(v) * time.Second)
	}

	// Logging
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Logging.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		config.Logging.Format = v
	}
	if v := os.Getenv("LOG_FILE"); v != "" {
		config.Logging.File = v
	}

	// MCP
	if v := os.Getenv("MCP_TRANSPORT"); v != "" {
		config.MCP.Transport = v
	}
	if v, ok := lookupInt("MCP_PORT"); ok {
		config.MCP.Port = v
	}
}

// applyDefaults applies default values to any unset configuration fields.
func applyDefaults(config *Config) {
	if config.Database.Port == 0 {
		config.Database.Port = defaultPort
	}
	if config.Database.Encrypt == "" {
		config.Database.Encrypt = defaultEncrypt
	}
	if config.Database.ConnectionTimeout == 0 {
		config.Database.ConnectionTimeout = defaultConnectionTimeout
	}

	if config.Query.MaxLength == 0 {
		config.Query.MaxLength = defaultMaxQueryLength
	}
	if config.Query.MaxRows == 0 {
		config.Query.MaxRows = defaultMaxRows
	}
	if config.Query.Timeout == 0 {
		config.Query.Timeout = defaultQueryTimeout
	}

	if config.Pool.MaxOpenConns == 0 {
		config.Pool.MaxOpenConns = defaultMaxOpenConns
	}
	if config.Pool.MaxIdleConns == 0 {
		config.Pool.MaxIdleConns = defaultMaxIdleConns
	}
	if config.Pool.ConnMaxLifetime == 0 {
		config.Pool.ConnMaxLifetime = defaultConnMaxLifetime
	}

	if config.Logging.Level == "" {
		config.Logging.Level = defaultLogLevel
	}
	if config.Logging.Format == "" {
		config.Logging.Format = defaultLogFormat
	}

	if config.MCP.Transport == "" {
		config.MCP.Transport = defaultTransport
	}
	if config.MCP.Port == 0 {
		config.MCP.Port = defaultMCPPort
	}
}

// Validate validates the configuration and returns an error describing any
// invalid or missing required values.
func (c *Config) Validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required (set SQLSERVER_HOST)")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database user is required (set SQLSERVER_USER)")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name is required (set SQLSERVER_DB)")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", c.Database.Port)
	}

	if c.Encryption.Enabled {
		if c.Encryption.KeyPath == "" {
			return fmt.Errorf("always encrypted is enabled but SQLSERVER_ALWAYS_ENCRYPTED_KEY_PATH is not set")
		}
		if _, err := os.Stat(c.Encryption.KeyPath); err != nil {
			return fmt.Errorf("always encrypted key path %q is not accessible: %w", c.Encryption.KeyPath, err)
		}
	}

	if c.Query.MaxLength < 1 {
		return fmt.Errorf("query.max_length must be positive")
	}
	if c.Query.MaxRows < 1 {
		return fmt.Errorf("query.max_rows must be positive")
	}
	if c.Query.Timeout <= 0 {
		return fmt.Errorf("query.timeout must be positive")
	}

	if c.Pool.MaxOpenConns < 1 {
		return fmt.Errorf("pool.max_open_conns must be positive")
	}
	if c.Pool.MaxIdleConns < 0 {
		return fmt.Errorf("pool.max_idle_conns must not be negative")
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validTransports := map[string]bool{"stdio": true, "http": true}
	if !validTransports[c.MCP.Transport] {
		return fmt.Errorf("invalid MCP transport: %s", c.MCP.Transport)
	}

	return nil
}

// lookupInt reads an integer environment variable, returning ok=false when the
// variable is unset or not a valid integer.
func lookupInt(key string) (int, bool) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(valueStr))
	if err != nil {
		return 0, false
	}
	return value, true
}

// lookupBool reads a boolean environment variable, returning ok=false when the
// variable is unset.
func lookupBool(key string) (bool, bool) {
	valueStr := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if valueStr == "" {
		return false, false
	}
	switch valueStr {
	case "true", "1", "yes", "on", "enabled":
		return true, true
	case "false", "0", "no", "off", "disabled":
		return false, true
	default:
		return false, false
	}
}
