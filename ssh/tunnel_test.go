package ssh

import (
	"testing"

	"github.com/eyjian/mysh/config"
)

func TestSSHConfigEnabled(t *testing.T) {
	cfg := config.SSHConfig{}
	if cfg.Enabled() {
		t.Error("empty SSHConfig should not be enabled")
	}

	cfg.Host = "jump.example.com"
	if !cfg.Enabled() {
		t.Error("SSHConfig with Host should be enabled")
	}
}

func TestSSHPort(t *testing.T) {
	if got := sshPort(0); got != 22 {
		t.Errorf("sshPort(0) = %d, want 22", got)
	}
	if got := sshPort(2222); got != 2222 {
		t.Errorf("sshPort(2222) = %d, want 2222", got)
	}
	if got := sshPort(-1); got != 22 {
		t.Errorf("sshPort(-1) = %d, want 22", got)
	}
}

func TestExpandHome(t *testing.T) {
	// Non-home paths should be unchanged
	if got := expandHome("/etc/ssh/config"); got != "/etc/ssh/config" {
		t.Errorf("expandHome(/etc/ssh/config) = %s, want /etc/ssh/config", got)
	}

	// Home path should be expanded
	got := expandHome("~/test")
	if len(got) < 2 || got[len(got)-5:] != "/test" {
		t.Errorf("expandHome(~/test) = %s, want $HOME/test", got)
	}
}

func TestBuildSSHClientConfig_NoAuth(t *testing.T) {
	// This test verifies that a config with no explicit auth will either
	// find default keys (on systems that have them) or return an error.
	// Since we can't control whether the test environment has SSH keys,
	// we just verify the function doesn't panic.
	cfg := &config.SSHConfig{
		Host: "example.com",
		User: "test",
	}
	_, _ = buildSSHClientConfig(cfg)
	// Result depends on whether ~/.ssh/id_* keys exist
}

func TestBuildSSHClientConfig_PasswordAuth(t *testing.T) {
	cfg := &config.SSHConfig{
		Host:     "example.com",
		User:     "test",
		Password: "secret",
	}
	clientCfg, err := buildSSHClientConfig(cfg)
	if err != nil {
		t.Fatalf("buildSSHClientConfig failed: %v", err)
	}
	if clientCfg.User != "test" {
		t.Errorf("User = %s, want test", clientCfg.User)
	}
	if len(clientCfg.Auth) < 1 {
		t.Error("expected at least one auth method")
	}
}

func TestNewTunnel_EmptyConfig(t *testing.T) {
	cfg := &config.SSHConfig{}
	_, err := NewTunnel(cfg, "10.0.0.5", 3306)
	if err == nil {
		t.Error("expected error for empty SSH config")
	}
}
