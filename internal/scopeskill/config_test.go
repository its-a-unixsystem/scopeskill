package scopeskill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileEnvFileGrammar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	raw := strings.Join([]string{
		"# first",
		"UNKNOWN=value",
		"REST_REFRESH_TOKEN=old",
		"REST_REFRESH_TOKEN=winner ",
		"  # indented comment",
		"INLINE=value # not a comment",
		"",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	configFile, err := ReadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := configFile.Values()
	if values[ConfigKeyRestRefreshToken] != "winner" {
		t.Fatalf("REST_REFRESH_TOKEN = %q", values[ConfigKeyRestRefreshToken])
	}
	if values["INLINE"] != "value # not a comment" {
		t.Fatalf("INLINE = %q", values["INLINE"])
	}
	if err := configFile.Set(ConfigKeyRestRefreshToken, "new-token"); err != nil {
		t.Fatal(err)
	}
	if err := configFile.Set(ConfigKeyCustomer, "1234567"); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"# first",
		"UNKNOWN=value",
		"REST_REFRESH_TOKEN=new-token",
		"  # indented comment",
		"INLINE=value # not a comment",
		"",
		"CUSTOMER=1234567",
		"",
	}, "\n")
	if got := string(configFile.Bytes()); got != want {
		t.Fatalf("rewritten config:\n%s\nwant:\n%s", got, want)
	}
}

func TestConfigFileRejectsInvalidGrammar(t *testing.T) {
	for _, raw := range []string{
		"KEY =value\n",
		"1KEY=value\n",
		"not a comment\n",
		" key=value\n",
		"lower=value\n",
	} {
		t.Run(strings.TrimSpace(raw), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadConfigFile(path); err == nil {
				t.Fatal("expected invalid config error")
			}
		})
	}
}

func TestLoadClientConfigPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	raw := strings.Join([]string{
		"CUSTOMER=1234567",
		"REST_REFRESH_TOKEN=config-token",
		"BASE_URL=https://config.example",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRestRefreshToken, "env-token")
	t.Setenv(EnvBaseURL, "https://env.example")
	t.Setenv("SCOPESKILL_CUSTOMER", "9999999")

	config, err := LoadClientConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.RefreshToken != "env-token" {
		t.Fatalf("RefreshToken = %q", config.RefreshToken)
	}
	if config.BaseURL != "https://env.example" {
		t.Fatalf("BaseURL = %q", config.BaseURL)
	}
	if config.Customer != "1234567" {
		t.Fatalf("Customer = %q", config.Customer)
	}
}

func TestLoadClientConfigPathPrecedence(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env-config")
	explicitPath := filepath.Join(dir, "explicit-config")
	if err := os.WriteFile(envPath, []byte("CUSTOMER=1111111\nREST_REFRESH_TOKEN=env-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(explicitPath, []byte("CUSTOMER=2222222\nREST_REFRESH_TOKEN=explicit-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfig, envPath)
	t.Setenv(EnvRestRefreshToken, "")
	t.Setenv(EnvBaseURL, "")

	config, err := LoadClientConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if config.ConfigPath != envPath || config.Customer != "1111111" {
		t.Fatalf("env config = %#v", config)
	}

	config, err = LoadClientConfig(explicitPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConfigPath != explicitPath || config.Customer != "2222222" {
		t.Fatalf("explicit config = %#v", config)
	}
}

func TestDefaultConfigPathUsesUserConfigDir(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	want := filepath.Join(configDir, "scopeskill", "config")
	if got := DefaultConfigPath(); got != want {
		t.Fatalf("DefaultConfigPath = %q", got)
	}
}

func TestLoadClientConfigDefaultBaseURL(t *testing.T) {
	t.Setenv(EnvConfig, filepath.Join(t.TempDir(), "missing"))
	t.Setenv(EnvRestRefreshToken, "")
	t.Setenv(EnvBaseURL, "")

	config, err := LoadClientConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if config.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", config.BaseURL)
	}
}

func TestConfigFileWriteModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config")
	configFile := ConfigFile{Path: path}
	if err := configFile.Set(ConfigKeyCustomer, "1234567"); err != nil {
		t.Fatal(err)
	}
	if err := configFile.Set(ConfigKeyRestRefreshToken, "refresh-token"); err != nil {
		t.Fatal(err)
	}
	if err := configFile.Write(); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o700 {
		t.Fatalf("config parent mode = %o", mode)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config mode = %o", mode)
	}
}

func TestConfigFileAuthLoginBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	raw := strings.Join([]string{
		"# old comment",
		"CUSTOMER=old-customer",
		"BASE_URL=https://scopeskill.example",
		"REST_REFRESH_TOKEN=old-token",
		"UNKNOWN=preserved",
		AuthLoginConfigHeader,
		"REST_REFRESH_TOKEN=older-token",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	configFile, err := ReadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := configFile.SetAuthLogin("new-customer", "new-token"); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		AuthLoginConfigHeader,
		"CUSTOMER=new-customer",
		"REST_REFRESH_TOKEN=new-token",
		"# old comment",
		"BASE_URL=https://scopeskill.example",
		"UNKNOWN=preserved",
		"",
	}, "\n")
	if got := string(configFile.Bytes()); got != want {
		t.Fatalf("auth login config:\n%s\nwant:\n%s", got, want)
	}
}

func TestConfigFileDeletePreservesOtherLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	raw := strings.Join([]string{
		"# first",
		"CUSTOMER=1234567",
		"REST_REFRESH_TOKEN=old",
		"UNKNOWN=value",
		"REST_REFRESH_TOKEN=new",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	configFile, err := ReadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := configFile.Delete(ConfigKeyRestRefreshToken); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"# first",
		"CUSTOMER=1234567",
		"UNKNOWN=value",
		"",
	}, "\n")
	if got := string(configFile.Bytes()); got != want {
		t.Fatalf("deleted config:\n%s\nwant:\n%s", got, want)
	}
	if _, ok := configFile.Values()[ConfigKeyRestRefreshToken]; ok {
		t.Fatal("REST refresh token survived delete")
	}
}
