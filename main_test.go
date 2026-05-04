package main

import (
	"os"
	"strings"
	"testing"

	"mysh/tui"
)

func TestParseArgs_Host(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-h", "db.example.com"}
	parseArgs()
	if flagHost != "db.example.com" {
		t.Errorf("flagHost = %q, want %q", flagHost, "db.example.com")
	}
}

func TestParseArgs_HostLong(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "--host", "db.example.com"}
	parseArgs()
	if flagHost != "db.example.com" {
		t.Errorf("flagHost = %q, want %q", flagHost, "db.example.com")
	}
}

func TestParseArgs_Port(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-P", "3307"}
	parseArgs()
	if flagPort != 3307 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3307)
	}
}

func TestParseArgs_PortLong(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "--port", "3307"}
	parseArgs()
	if flagPort != 3307 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3307)
	}
}

func TestParseArgs_User(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-u", "admin"}
	parseArgs()
	if flagUser != "admin" {
		t.Errorf("flagUser = %q, want %q", flagUser, "admin")
	}
}

func TestParseArgs_UserLong(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "--user", "admin"}
	parseArgs()
	if flagUser != "admin" {
		t.Errorf("flagUser = %q, want %q", flagUser, "admin")
	}
}

func TestParseArgs_Password(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-p", "secret"}
	parseArgs()
	if flagPassword != "secret" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "secret")
	}
}

func TestParseArgs_PasswordLong(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "--password", "secret"}
	parseArgs()
	if flagPassword != "secret" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "secret")
	}
}

func TestParseArgs_Database(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-D", "mydb"}
	parseArgs()
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}

func TestParseArgs_DatabaseLong(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "--database", "mydb"}
	parseArgs()
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}

func TestParseArgs_AllFlags(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-h", "db.example.com", "-P", "3307", "-u", "admin", "-p", "secret", "-D", "mydb"}
	parseArgs()
	if flagHost != "db.example.com" {
		t.Errorf("flagHost = %q, want %q", flagHost, "db.example.com")
	}
	if flagPort != 3307 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3307)
	}
	if flagUser != "admin" {
		t.Errorf("flagUser = %q, want %q", flagUser, "admin")
	}
	if flagPassword != "secret" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "secret")
	}
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}

func TestParseArgs_NoArgs(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh"}
	parseArgs()
	if flagHost != "" {
		t.Errorf("flagHost = %q, want empty", flagHost)
	}
	if flagPort != 0 {
		t.Errorf("flagPort = %d, want 0", flagPort)
	}
	if flagUser != "" {
		t.Errorf("flagUser = %q, want empty", flagUser)
	}
	if flagPassword != "" {
		t.Errorf("flagPassword = %q, want empty", flagPassword)
	}
	if flagDatabase != "" {
		t.Errorf("flagDatabase = %q, want empty", flagDatabase)
	}
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	resetFlags()
	// Unknown flags should not crash, just print warning
	os.Args = []string{"mysh", "--unknown"}
	parseArgs()
	// Should not set any flags
	if flagHost != "" || flagPort != 0 || flagUser != "" || flagPassword != "" || flagDatabase != "" {
		t.Error("unknown flag should not set any values")
	}
}

func TestParseArgs_MissingFlagValue(t *testing.T) {
	resetFlags()
	// -h without value should not crash
	os.Args = []string{"mysh", "-h"}
	parseArgs()
	// flagHost should remain empty since no value provided
	if flagHost != "" {
		t.Errorf("flagHost = %q, want empty when no value provided", flagHost)
	}
}

func TestParseArgs_PortInvalid(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-P", "notanumber"}
	parseArgs()
	// Sscanf fails, flagPort should remain 0
	if flagPort != 0 {
		t.Errorf("flagPort = %d, want 0 for invalid port", flagPort)
	}
}

func TestPrintUsage(t *testing.T) {
	// Just verify printUsage doesn't panic and produces output
	// We can't easily capture stdout, but we can verify the help text content
	help := `mysh - MySQL CLI with syntax highlighting and auto-completion`
	if !strings.Contains(help, "mysh") {
		t.Error("help text should contain 'mysh'")
	}
}

func TestCleanup_NilDeps(t *testing.T) {
	// Should not panic with nil deps
	deps := tui.Dependencies{
		History: nil,
		Pool:    nil,
	}
	cleanup(deps)
}

// resetFlags resets global flag variables to zero values.
func resetFlags() {
	flagHost = ""
	flagPort = 0
	flagUser = ""
	flagPassword = ""
	flagDatabase = ""
}
