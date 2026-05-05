package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	gossh "golang.org/x/crypto/ssh"

	"github.com/eyjian/mysh/config"
)

// Tunnel represents an SSH tunnel that forwards a local port to a remote MySQL server.
type Tunnel struct {
	client   *gossh.Client
	listener net.Listener
	local    string // local address (e.g., "127.0.0.1:12345")
	remote   string // remote address (e.g., "10.0.0.5:3306")
	closed   bool
	mu       sync.Mutex
}

// NewTunnel establishes an SSH tunnel and returns a Tunnel that forwards
// connections from a random local port to remoteHost:remotePort through the SSH server.
func NewTunnel(cfg *config.SSHConfig, remoteHost string, remotePort int) (*Tunnel, error) {
	if cfg == nil || !cfg.Enabled() {
		return nil, fmt.Errorf("SSH config is empty")
	}

	// Build SSH client config
	sshCfg, err := buildSSHClientConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH config error: %w", err)
	}

	// Connect to SSH server
	sshAddr := fmt.Sprintf("%s:%d", cfg.Host, sshPort(cfg.Port))
	client, err := gossh.Dial("tcp", sshAddr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s failed: %w", sshAddr, err)
	}

	// Create local listener on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("local listen failed: %w", err)
	}

	remote := fmt.Sprintf("%s:%d", remoteHost, remotePort)
	tunnel := &Tunnel{
		client:   client,
		listener: listener,
		local:    listener.Addr().String(),
		remote:   remote,
	}

	// Start accepting connections in the background
	go tunnel.acceptLoop()

	return tunnel, nil
}

// LocalAddr returns the local address that MySQL should connect to.
func (t *Tunnel) LocalAddr() string {
	return t.local
}

// LocalPort returns the local port number.
func (t *Tunnel) LocalPort() int {
	_, portStr, _ := net.SplitHostPort(t.local)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// Close shuts down the tunnel.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	t.listener.Close()
	return t.client.Close()
}

// IsClosed returns whether the tunnel has been closed.
func (t *Tunnel) IsClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// acceptLoop accepts incoming connections on the local listener and forwards
// them through the SSH tunnel to the remote MySQL server.
func (t *Tunnel) acceptLoop() {
	for {
		localConn, err := t.listener.Accept()
		if err != nil {
			// Listener closed or error
			return
		}

		go t.forward(localConn)
	}
}

// forward copies data between a local connection and the remote server via SSH.
func (t *Tunnel) forward(localConn net.Conn) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		localConn.Close()
		return
	}
	client := t.client
	remote := t.remote
	t.mu.Unlock()

	// Dial the remote server through the SSH tunnel
	remoteConn, err := client.Dial("tcp", remote)
	if err != nil {
		localConn.Close()
		return
	}

	// Bidirectional copy
	go func() {
		io.Copy(localConn, remoteConn)
		localConn.Close()
		remoteConn.Close()
	}()
	go func() {
		io.Copy(remoteConn, localConn)
		remoteConn.Close()
		localConn.Close()
	}()
}

// buildSSHClientConfig creates an ssh.ClientConfig from our SSHConfig.
func buildSSHClientConfig(cfg *config.SSHConfig) (*gossh.ClientConfig, error) {
	authMethods := []gossh.AuthMethod{}

	// Try key-based auth first
	if cfg.Key != "" {
		keyPath := expandHome(cfg.Key)
		signer, err := keySigner(keyPath)
		if err == nil {
			authMethods = append(authMethods, signer)
		}
	}

	// Also try default SSH keys if no explicit key was specified or as fallback
	if cfg.Key == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			defaultKeys := []string{
				filepath.Join(home, ".ssh", "id_ed25519"),
				filepath.Join(home, ".ssh", "id_rsa"),
				filepath.Join(home, ".ssh", "id_ecdsa"),
				filepath.Join(home, ".ssh", "id_xmss"),
			}
			for _, keyPath := range defaultKeys {
				if _, err := os.Stat(keyPath); err == nil {
					signer, err := keySigner(keyPath)
					if err == nil {
						authMethods = append(authMethods, signer)
					}
				}
			}
		}
	}

	// Password auth
	if cfg.Password != "" {
		authMethods = append(authMethods, gossh.Password(cfg.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth method available (configure key or password)")
	}

	return &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), // TODO: support known_hosts
		Timeout:         10 * 1000 * 1000 * 1000,       // 10 seconds
	}, nil
}

// keySigner loads a private key file and returns an AuthMethod.
func keySigner(path string) (gossh.AuthMethod, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", path, err)
	}

	signer, err := gossh.ParsePrivateKey(keyData)
	if err != nil {
		// Try with passphrase — but we don't have one, so just fail
		return nil, fmt.Errorf("parse key %s: %w", path, err)
	}

	return gossh.PublicKeys(signer), nil
}

// sshPort returns the SSH port, defaulting to 22 if 0.
func sshPort(port int) int {
	if port <= 0 {
		return 22
	}
	return port
}

// expandHome replaces ~ with the user's home directory.
func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
