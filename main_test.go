package main

import (
	"os"
	"strings"
	"testing"

	"github.com/eyjian/mysh/tui"
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
	os.Args = []string{"mysh", "--unknown"}
	err := parseArgs()
	if err == nil {
		t.Error("expected error for unknown flag")
	}
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
	flagExecute = ""
	flagAutoVerticalOutput = false
	flagFormat = ""
	flagCharset = ""
}

func TestParseDSN_Full(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "user:pass@tcp(127.0.0.1:3306)/testdb?charset=utf8mb4&parseTime=true"}
	parseArgs()
	if flagUser != "user" {
		t.Errorf("flagUser = %q, want %q", flagUser, "user")
	}
	if flagPassword != "pass" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "pass")
	}
	if flagHost != "127.0.0.1" {
		t.Errorf("flagHost = %q, want %q", flagHost, "127.0.0.1")
	}
	if flagPort != 3306 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3306)
	}
	if flagDatabase != "testdb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "testdb")
	}
	if flagCharset != "utf8mb4" {
		t.Errorf("flagCharset = %q, want %q", flagCharset, "utf8mb4")
	}
}

func TestParseDSN_NoTCP(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "admin:secret@db.example.com:3307/mydb"}
	parseArgs()
	if flagUser != "admin" {
		t.Errorf("flagUser = %q, want %q", flagUser, "admin")
	}
	if flagPassword != "secret" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "secret")
	}
	if flagHost != "db.example.com" {
		t.Errorf("flagHost = %q, want %q", flagHost, "db.example.com")
	}
	if flagPort != 3307 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3307)
	}
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}

func TestParseDSN_UserOnly(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "root@127.0.0.1/testdb"}
	parseArgs()
	if flagUser != "root" {
		t.Errorf("flagUser = %q, want %q", flagUser, "root")
	}
	if flagPassword != "" {
		t.Errorf("flagPassword = %q, want empty", flagPassword)
	}
	if flagHost != "127.0.0.1" {
		t.Errorf("flagHost = %q, want %q", flagHost, "127.0.0.1")
	}
	if flagDatabase != "testdb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "testdb")
	}
}

func TestParseDSN_HostPortDB(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "127.0.0.1:3307/mydb"}
	parseArgs()
	if flagHost != "127.0.0.1" {
		t.Errorf("flagHost = %q, want %q", flagHost, "127.0.0.1")
	}
	if flagPort != 3307 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3307)
	}
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}

func TestParseDSN_HostDB(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "db.example.com/mydb"}
	parseArgs()
	if flagHost != "db.example.com" {
		t.Errorf("flagHost = %q, want %q", flagHost, "db.example.com")
	}
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}

func TestParseArgs_PositionalDB(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "-h", "127.0.0.1", "-u", "root", "testdb"}
	parseArgs()
	if flagHost != "127.0.0.1" {
		t.Errorf("flagHost = %q, want %q", flagHost, "127.0.0.1")
	}
	if flagUser != "root" {
		t.Errorf("flagUser = %q, want %q", flagUser, "root")
	}
	if flagDatabase != "testdb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "testdb")
	}
}

func TestParseDSN_DoesNotOverrideFlags(t *testing.T) {
	resetFlags()
	// Flags should take priority over DSN values
	os.Args = []string{"mysh", "-u", "flaguser", "dsner:dsnpass@127.0.0.1/testdb"}
	parseArgs()
	if flagUser != "flaguser" {
		t.Errorf("flagUser = %q, want %q (flag should override DSN)", flagUser, "flaguser")
	}
	if flagPassword != "dsnpass" {
		t.Errorf("flagPassword = %q, want %q (DSN should fill unset flags)", flagPassword, "dsnpass")
	}
}

func TestParseDSN_PasswordWithAt(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "user:password@1234@tcp(127.0.0.1:3306)/testdb?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"}
	parseArgs()
	if flagUser != "user" {
		t.Errorf("flagUser = %q, want %q", flagUser, "user")
	}
	if flagPassword != "password@1234" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "password@1234")
	}
	if flagHost != "127.0.0.1" {
		t.Errorf("flagHost = %q, want %q", flagHost, "127.0.0.1")
	}
	if flagPort != 3306 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3306)
	}
	if flagDatabase != "testdb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "testdb")
	}
	if flagCharset != "utf8mb4" {
		t.Errorf("flagCharset = %q, want %q", flagCharset, "utf8mb4")
	}
}

func TestParseDSN_PasswordWithAtNoTCP(t *testing.T) {
	resetFlags()
	os.Args = []string{"mysh", "admin:p@ss@w0rd@db.example.com:3307/mydb"}
	parseArgs()
	if flagUser != "admin" {
		t.Errorf("flagUser = %q, want %q", flagUser, "admin")
	}
	if flagPassword != "p@ss@w0rd" {
		t.Errorf("flagPassword = %q, want %q", flagPassword, "p@ss@w0rd")
	}
	if flagHost != "db.example.com" {
		t.Errorf("flagHost = %q, want %q", flagHost, "db.example.com")
	}
	if flagPort != 3307 {
		t.Errorf("flagPort = %d, want %d", flagPort, 3307)
	}
	if flagDatabase != "mydb" {
		t.Errorf("flagDatabase = %q, want %q", flagDatabase, "mydb")
	}
}
