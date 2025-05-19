package gofrr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultTimeout is a default socket timeout.
	DefaultTimeout = 1 * time.Second

	// DefaultFRRConfigPerm is a default permission for FRR configuration files.
	DefaultFRRConfigPerm = fs.FileMode(0640)
)

const (
	mgmtSocketName  = "mgmtd.vty"
	bfdSocketName   = "bfdd.vty"
	bgpSocketName   = "bgpd.vty"
	zebraSocketName = "zebra.vty"
)

// Sockets is a container for multiple FRR connections.
type Sockets struct {
	bgpConn   *Connection
	bfdConn   *Connection
	mgmtConn  *Connection
	zebraConn *Connection

	allConnections []*Connection

	frrConfigPath string
}

// NewSockets creates a container with connections to multiple FRR sockets.
// Currently there is support of connecting to BFD, BGP, Mgmt and Zebra sockets.
func NewSockets(frrConfigPath string, frrRunDir string) *Sockets {
	bgpConn := NewConnection(filepath.Join(frrRunDir, bgpSocketName))
	bfdConn := NewConnection(filepath.Join(frrRunDir, bfdSocketName))
	mgmtConn := NewConnection(filepath.Join(frrRunDir, mgmtSocketName))
	zebraConn := NewConnection(filepath.Join(frrRunDir, zebraSocketName))

	// this is a helper to iterate over sockets for Connect(), Close() and DumpRunningConfig() methods.
	allConnections := []*Connection{
		bgpConn,
		bfdConn,
		mgmtConn,
		zebraConn,
	}

	return &Sockets{
		bgpConn:   bgpConn,
		bfdConn:   bfdConn,
		mgmtConn:  mgmtConn,
		zebraConn: zebraConn,

		allConnections: allConnections,

		frrConfigPath: frrConfigPath,
	}
}

func (s *Sockets) execute(ctx context.Context, conn *Connection, cmd string) ([]byte, error) {
	return conn.Execute(ctx, cmd)
}

// ExecuteBFD executes a command on the BFD socket.
func (s *Sockets) ExecuteBFD(ctx context.Context, cmd string) ([]byte, error) {
	return s.execute(ctx, s.mgmtConn, cmd)
}

// ExecuteBGP executes a command on the BGP socket.
func (s *Sockets) ExecuteBGP(ctx context.Context, cmd string) ([]byte, error) {
	return s.execute(ctx, s.bgpConn, cmd)
}

// ExecuteMgmt executes a command on the Mgmt socket.
func (s *Sockets) ExecuteMgmt(ctx context.Context, cmd string) ([]byte, error) {
	return s.execute(ctx, s.mgmtConn, cmd)
}

// ExecuteZebra executes a command on the Zebra socket.
func (s *Sockets) ExecuteZebra(ctx context.Context, cmd string) ([]byte, error) {
	return s.execute(ctx, s.zebraConn, cmd)
}

// Connect establishes connections with all FRR sockets.
func (s *Sockets) Connect(ctx context.Context) (err error) {
	for _, c := range s.allConnections {
		err = errors.Join(err, c.Connect(ctx))
	}
	if err != nil {
		err = errors.Join(err, s.Close())
		return err
	}
	return nil
}

// Close closes connections to all FRR sockets.
func (s *Sockets) Close() (err error) {
	for _, c := range s.allConnections {
		err = errors.Join(err, c.Close())
	}
	return err
}

// ApplyBGPConfig passes configuration to BGP daemon. Configuration is a bunch of BGP daemon commadns split by "\n".
func (s *Sockets) ApplyBGPConfig(ctx context.Context, config []byte) error {
	return s.bgpConn.ApplyConfig(ctx, config)
}

// ApplyMgmtConfig passes configuration to Mgmt daemon. Configuration is a bunch of Mgmt daemon commadns split by "\n".
func (s *Sockets) ApplyMgmtConfig(ctx context.Context, config []byte) error {
	return s.mgmtConn.ApplyConfig(ctx, config)
}

// ShowRunningConfigBGP is a helper to get current configuration of BGP daemon.
func (s *Sockets) ShowRunningConfigBGP(ctx context.Context) ([]byte, error) {
	return s.bgpConn.ShowRunningConfig(ctx)
}

// ShowRunningConfigMgmt is a helper to get current configuration of Mgmt daemon.
func (s *Sockets) ShowRunningConfigMgmt(ctx context.Context) ([]byte, error) {
	return s.mgmtConn.ShowRunningConfig(ctx)
}

// DumpRunningConfig runs 'do write terminal' (analog of 'show running-config' command) to all connected sockets.
// After getting all seprate configurations, it merges them in one, and atomicly writes the result to `dstFile` with specified `mode` permissions.
func (s *Sockets) DumpRunningConfig(ctx context.Context, dstFile string, mode fs.FileMode) (err error) {
	var resultConfig bytes.Buffer
	for _, conn := range s.allConnections {
		var resp []byte
		resp, err = conn.ShowRunningConfig(ctx)
		if err != nil {
			return fmt.Errorf("show running config, resp: %s, err: %w", resp, err)
		}

		// Add information about socket to config comment.
		comment := fmt.Appendf([]byte{}, "! %s\n", conn.socketPath)
		if _, err = resultConfig.Write(comment); err != nil {
			return fmt.Errorf("write comment to buffer: %w", err)
		}

		if _, err = resultConfig.Write(resp); err != nil {
			return fmt.Errorf("write response to buffer: %w", err)
		}
	}

	if err := atomicWrite(dstFile, resultConfig.Bytes(), mode); err != nil {
		return fmt.Errorf("atomic write for %s: %w", s.frrConfigPath, err)
	}

	return nil
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return fmt.Errorf("writing temporary file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temporary file %q to destination file %q: %w", tmpPath, path, err)
	}
	return nil
}
