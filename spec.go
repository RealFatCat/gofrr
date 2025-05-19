package gofrr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

/* FRR Response

 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                       Plaintext Response                      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                 Marker (0x00)          |     Status Code      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

*/

// According to protocol, plaintext response is followed by 3 null marker bytes.
var terminationMarker = []byte{0, 0, 0}

// There is one byte of status code after termination marker, which is also marks the end of the response.
// So, in total, we have to manage 4 bytes of technical data.
const techDataTotalLen = 4

type socketResponse struct {
	plainText  []byte
	statusCode StatusCode
	err        error
}

// Connection represents connection to FRR socket.
type Connection struct {
	socketPath string

	conn net.Conn
}

var (
	ErrConnNotEstab            = errors.New("connection is not established for socket")
	ErrNotAcceptableStatusCode = errors.New("not acceptable status code")
)

// NecConnect creates new connection to FRR socket.
// It does not establish connection, it just creates new instance of Connection.
func NewConnection(socketPath string) *Connection {
	return &Connection{
		socketPath: socketPath,
	}
}

// Connect establishes connection to socket.
func (c *Connection) Connect(ctx context.Context) (err error) {
	var d net.Dialer

	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// Close closes connection to socket.
func (c *Connection) Close() error {
	if c.conn == nil {
		return nil
	}

	defer func() { c.conn = nil }()

	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("socket connection close %s: %w", c.socketPath, err)
	}
	return nil
}

// Execute runs one command via socket.
// Deadlines for both read and write can be set via context.WithDeadline.
// Default timeout is 1 second.
func (c *Connection) Execute(ctx context.Context, cmd string) (resp []byte, err error) {
	if c.conn == nil {
		return nil, fmt.Errorf("%w %s", ErrConnNotEstab, c.socketPath)
	}

	if err = c.setDeadline(ctx); err != nil {
		return nil, fmt.Errorf("set connection deadline %s: %w", c.socketPath, err)
	}

	if _, err = c.writeCommand(cmd); err != nil {
		return nil, fmt.Errorf("write %q to socket %q: %w", cmd, c.socketPath, err)
	}

	response := c.readResponse()
	if response.err != nil {
		return response.plainText, response.err
	}

	sc := response.statusCode

	// Got this check from frr vtysh_main.c, look for vtysh_execute_no_pager.
	if sc != Success && sc != Warning && sc != SuccessDaemon {
		return response.plainText, fmt.Errorf("%w, FRR command %q on socket %q: %s", ErrNotAcceptableStatusCode, cmd, c.socketPath, sc.String())
	}
	return response.plainText, nil
}

func (c *Connection) setDeadline(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(DefaultTimeout)
	}
	return c.conn.SetDeadline(deadline)
}

func (c *Connection) writeCommand(cmd string) (int, error) {
	return c.conn.Write([]byte(cmd + "\x00"))
}

func (c *Connection) readResponse() socketResponse {
	if c.conn == nil {
		return socketResponse{err: fmt.Errorf("%w %s", ErrConnNotEstab, c.socketPath)}
	}

	var response bytes.Buffer
	var statusCode StatusCode

	bufSize := 4096
	buf := make([]byte, bufSize)

	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return socketResponse{
				plainText: response.Bytes(),
				err:       err,
			}
		}

		response.Write(buf[:n])

		// Find termination marker position.
		endIdx := bytes.Index(response.Bytes(), terminationMarker)
		if endIdx < 0 {
			continue
		}

		// Find status code, stop reading if present
		if (response.Len() - endIdx) == techDataTotalLen {
			statusCode = StatusCode(response.Bytes()[response.Len()-1])
			break
		}
	}

	// get rid of technical data in response
	plainText := response.Bytes()[:response.Len()-techDataTotalLen]
	return socketResponse{plainText: plainText, statusCode: statusCode, err: nil}
}

// ApplyConfig is a helper function to pass specific daemon configuration via socket.
// It wraps passed commands in config argument with 'enable' and 'configure' commands.
// After applying config, ApplyConfig will try to exit from 'config' and 'enable' modes.
// It is better to close connection if any error returns, due to unknown mode of the current connection.
func (c *Connection) ApplyConfig(ctx context.Context, config []byte) error {
	commands := bytes.Split(config, []byte("\n"))

	// Preparations: enter modes
	for _, cmd := range []string{"enable", "configure"} {
		if _, err := c.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("could not run %s command: %w", cmd, err)
		}
	}

	// Run config commands
	for _, cmd := range commands {
		cmd = bytes.TrimSpace(cmd)
		if len(cmd) == 0 {
			continue
		}

		if _, err := c.Execute(ctx, string(cmd)); err != nil {
			return fmt.Errorf("exec command '%s' on frr socket %s while applying config: %w", cmd, c.socketPath, err)
		}
	}

	// Cleanups: exit modes
	for _, cmd := range []string{"exit", "disable"} {
		if _, err := c.Execute(ctx, cmd); err != nil {
			return fmt.Errorf("could not run %s command: %w", cmd, err)
		}
	}
	return nil
}

// ShowRunningConfig is a helper to get current config of the daemon we are connected to.
// It is better to close connection if any error returns, due to unknown mode of the current connection.
func (c *Connection) ShowRunningConfig(ctx context.Context) ([]byte, error) {
	if r, err := c.Execute(ctx, "enable"); err != nil {
		return nil, fmt.Errorf("show running config `enable` command, resp: %s, err: %w", r, err)
	}

	// analog of 'show running-config'
	response, err := c.Execute(ctx, "do write terminal")
	if err != nil {
		return nil, fmt.Errorf("show running config `do write terminal` command, resp: %s, err: %w", response, err)
	}

	// Cleanup
	if r, err := c.Execute(ctx, "disable"); err != nil {
		return nil, fmt.Errorf("show running config `disable` command, resp: %s, err: %w", r, err)
	}
	return response, nil
}
