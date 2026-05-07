package scopeskill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ConfigKeyBaseURL          = "BASE_URL"
	ConfigKeyCustomer         = "CUSTOMER"
	ConfigKeyRestRefreshToken = "REST_REFRESH_TOKEN"
	AuthLoginConfigHeader     = "# scopeskill config — managed by 'sv-cli auth login'"

	EnvConfig           = "SCOPESKILL_CONFIG"
	EnvBaseURL          = "SCOPESKILL_BASE_URL"
	EnvRestRefreshToken = "SCOPESKILL_REST_REFRESH_TOKEN"
	EnvAccessTokenCache = "SCOPESKILL_ACCESS_TOKEN_CACHE"
)

type ConfigFile struct {
	Path    string
	lines   []configLine
	values  map[string]string
	touched map[string]string
	deleted map[string]bool
	login   bool
}

type configLine struct {
	raw   string
	key   string
	value string
}

func LoadClientConfig(configPath string) (Config, error) {
	path := ResolveConfigPath(configPath)
	file, err := ReadConfigFile(path)
	if err != nil {
		return Config{}, err
	}

	values := file.Values()
	config := Config{
		ConfigPath:       path,
		BaseURL:          valueOrDefault(values[ConfigKeyBaseURL], DefaultBaseURL),
		Customer:         values[ConfigKeyCustomer],
		RefreshToken:     values[ConfigKeyRestRefreshToken],
		AccessTokenCache: os.Getenv(EnvAccessTokenCache),
	}
	if value := os.Getenv(EnvBaseURL); value != "" {
		config.BaseURL = value
	}
	if value := os.Getenv(EnvRestRefreshToken); value != "" {
		config.RefreshToken = value
	}
	return config, nil
}

func ResolveConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	if value := os.Getenv(EnvConfig); value != "" {
		return value
	}
	return DefaultConfigPath()
}

func DefaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err == nil {
		return filepath.Join(configDir, "scopeskill", "config")
	}
	homeDir, homeErr := os.UserHomeDir()
	if homeErr == nil {
		return filepath.Join(homeDir, ".config", "scopeskill", "config")
	}
	return filepath.Join(".config", "scopeskill", "config")
}

func ReadConfigFile(path string) (ConfigFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ConfigFile{Path: path, values: map[string]string{}}, nil
		}
		return ConfigFile{}, err
	}

	file := ConfigFile{
		Path:   path,
		values: map[string]string{},
	}
	for index, rawLine := range splitConfigLines(string(raw)) {
		line, err := parseConfigLine(rawLine, index+1)
		if err != nil {
			return ConfigFile{}, err
		}
		file.lines = append(file.lines, line)
		if line.key != "" {
			file.values[line.key] = line.value
		}
	}
	return file, nil
}

func (c ConfigFile) Values() map[string]string {
	values := map[string]string{}
	for key, value := range c.values {
		values[key] = value
	}
	for key := range c.deleted {
		delete(values, key)
	}
	for key, value := range c.touched {
		values[key] = value
	}
	return values
}

func (c *ConfigFile) Set(key string, value string) error {
	if !validConfigKey(key) {
		return fmt.Errorf("invalid config key: %s", key)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("config value for %s contains a newline", key)
	}
	if c.values == nil {
		c.values = map[string]string{}
	}
	if c.touched == nil {
		c.touched = map[string]string{}
	}
	c.values[key] = strings.TrimSpace(value)
	c.touched[key] = strings.TrimSpace(value)
	delete(c.deleted, key)
	return nil
}

func (c *ConfigFile) Delete(key string) error {
	if !validConfigKey(key) {
		return fmt.Errorf("invalid config key: %s", key)
	}
	if c.values == nil {
		c.values = map[string]string{}
	}
	if c.deleted == nil {
		c.deleted = map[string]bool{}
	}
	delete(c.values, key)
	delete(c.touched, key)
	c.deleted[key] = true
	return nil
}

func (c *ConfigFile) SetAuthLogin(customer string, restRefreshToken string) error {
	if err := c.Set(ConfigKeyCustomer, customer); err != nil {
		return err
	}
	if err := c.Set(ConfigKeyRestRefreshToken, restRefreshToken); err != nil {
		return err
	}
	c.login = true
	return nil
}

func (c ConfigFile) Write() error {
	if err := ensurePrivateConfigDir(filepath.Dir(c.Path)); err != nil {
		return err
	}
	raw := c.Bytes()
	file, err := os.OpenFile(c.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(c.Path, 0o600)
}

func (c ConfigFile) Bytes() []byte {
	if c.login {
		return c.authLoginBytes()
	}
	if len(c.touched) == 0 && len(c.deleted) == 0 {
		var builder strings.Builder
		for _, line := range c.lines {
			builder.WriteString(line.raw)
		}
		return []byte(builder.String())
	}

	written := map[string]bool{}
	var builder strings.Builder
	for _, line := range c.lines {
		if c.deleted[line.key] {
			continue
		}
		value, touched := c.touched[line.key]
		if !touched {
			builder.WriteString(line.raw)
			continue
		}
		if written[line.key] {
			continue
		}
		builder.WriteString(line.key)
		builder.WriteByte('=')
		builder.WriteString(value)
		builder.WriteString(lineEnding(line.raw))
		written[line.key] = true
	}

	for _, key := range []string{ConfigKeyBaseURL, ConfigKeyCustomer, ConfigKeyRestRefreshToken} {
		value, touched := c.touched[key]
		if !touched || written[key] {
			continue
		}
		if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(value)
		builder.WriteByte('\n')
		written[key] = true
	}

	return []byte(builder.String())
}

func (c ConfigFile) authLoginBytes() []byte {
	values := c.Values()
	written := map[string]bool{}
	var builder strings.Builder
	builder.WriteString(AuthLoginConfigHeader)
	builder.WriteByte('\n')
	for _, key := range []string{ConfigKeyCustomer, ConfigKeyRestRefreshToken} {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(values[key])
		builder.WriteByte('\n')
		written[key] = true
	}
	for _, line := range c.lines {
		if line.key != "" && (written[line.key] || c.deleted[line.key]) {
			continue
		}
		if strings.TrimRight(line.raw, "\r\n") == AuthLoginConfigHeader {
			continue
		}
		if line.key != "" {
			if value, touched := c.touched[line.key]; touched {
				builder.WriteString(line.key)
				builder.WriteByte('=')
				builder.WriteString(value)
				builder.WriteString(lineEnding(line.raw))
				written[line.key] = true
				continue
			}
		}
		builder.WriteString(line.raw)
	}
	keys := make([]string, 0, len(c.touched))
	for key := range c.touched {
		if written[key] || c.deleted[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(c.touched[key])
		builder.WriteByte('\n')
	}
	return []byte(builder.String())
}

func parseConfigLine(rawLine string, lineNumber int) (configLine, error) {
	content := strings.TrimSuffix(rawLine, "\n")
	content = strings.TrimSuffix(content, "\r")
	if strings.TrimSpace(content) == "" {
		return configLine{raw: rawLine}, nil
	}
	if strings.HasPrefix(strings.TrimLeft(content, " \t"), "#") {
		return configLine{raw: rawLine}, nil
	}
	key, value, ok := strings.Cut(content, "=")
	if !ok || !validConfigKey(key) {
		return configLine{}, fmt.Errorf("invalid scopeskill config line %d: expected KEY=VALUE", lineNumber)
	}
	return configLine{raw: rawLine, key: key, value: strings.TrimSpace(value)}, nil
}

func splitConfigLines(raw string) []string {
	if raw == "" {
		return nil
	}
	lines := []string{}
	for len(raw) > 0 {
		index := strings.IndexByte(raw, '\n')
		if index == -1 {
			return append(lines, raw)
		}
		lines = append(lines, raw[:index+1])
		raw = raw[index+1:]
	}
	return lines
}

func validConfigKey(key string) bool {
	if key == "" {
		return false
	}
	for index, char := range key {
		switch {
		case char == '_':
		case char >= 'A' && char <= 'Z':
		case index > 0 && char >= '0' && char <= '9':
		default:
			return false
		}
	}
	return true
}

func lineEnding(raw string) string {
	if strings.HasSuffix(raw, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(raw, "\n") {
		return "\n"
	}
	return "\n"
}

func valueOrDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func ensurePrivateConfigDir(dir string) error {
	cleanDir := filepath.Clean(dir)
	if cleanDir == "." || cleanDir == string(os.PathSeparator) {
		return nil
	}
	if err := os.MkdirAll(cleanDir, 0o700); err != nil {
		return err
	}
	return os.Chmod(cleanDir, 0o700)
}
