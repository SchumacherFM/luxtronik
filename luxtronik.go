package luxtronik

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	DefaultPort           = "8889"
	ParametersWrite       = 3002
	ParametersRead        = 3003
	CalculationsRead      = 3004
	VisibilitiesRead      = 3005
	SocketReadSizePeek    = 16
	SocketReadSizeInteger = 4
	SocketReadSizeChar    = 1
)

// Locking is being used to ensure that only a single socket operation is
// performed at any point in time. This helps to avoid issues with the
// Luxtronik controller, which seems unstable otherwise.
var globalLock = &sync.Mutex{}

type Client struct {
	opts Options
	host string
	port string
	conn net.Conn
}

type Options struct {
	ConnCB      func(net.Conn) // gets called during connect to set conn specific params
	SafeMode    bool
	DialTimeout time.Duration
	Logger      *zap.Logger
}

func MustNewClient(hostPort string, opts Options) *Client {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		panic(err)
	}
	if opts.DialTimeout < 1 {
		opts.DialTimeout = time.Minute
	}

	return &Client{
		opts: opts,
		host: host,
		port: port,
	}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Connect() (err error) {
	if c.conn == nil {
		c.conn, err = net.DialTimeout("tcp", c.host+":"+c.port, c.opts.DialTimeout)
		if err != nil {
			return err
		}
		if c.opts.ConnCB != nil {
			c.opts.ConnCB(c.conn)
		}
	}

	return err
}

func (c *Client) readParameters(pm DataTypeMap) error {
	return c.readFromHeatPump(pm, ParametersRead, 0)
}

func (c *Client) readCalculations(pm DataTypeMap) error {
	return c.readFromHeatPump(pm, CalculationsRead, 0)
}

func (c *Client) readVisibilities(pm DataTypeMap) error {
	return c.readFromHeatPump(pm, VisibilitiesRead, 0)
}

func (c *Client) readFromHeatPump(pm DataTypeMap, data ...int32) error {
	if len(data) < 2 {
		return fmt.Errorf("")
	}
	_, err := c.netWrite(data...)
	if err != nil {
		return fmt.Errorf("readFromHeatPump.netWrite to send %d failed: %w", data[0], err)
	}

	cmd, err := c.readUint32()
	if err != nil {
		return fmt.Errorf("readFromHeatPump.readUint32.cmd failed: %w", err)
	}

	if data[0] == CalculationsRead {
		var stat uint32
		stat, err = c.readUint32()
		if err != nil {
			return fmt.Errorf("readFromHeatPump.readUint32.cmd failed: %w", err)
		}
		_ = stat
	}

	if cmd != uint32(data[0]) {
		return fmt.Errorf("readFromHeatPump. received invalid command: %d want: %d", cmd, data[0])
	}

	length, err := c.readUint32()
	if err != nil {
		return fmt.Errorf("readFromHeatPump.readUint32.length failed: %w", err)
	}

	rawValues := make([]uint32, length)
	for i := uint32(0); i < length; i++ {
		if data[0] == VisibilitiesRead {
			char, err := c.readChar()
			if err != nil {
				return fmt.Errorf("readFromHeatPump.readUint32.paramID at index %d failed: %w", i, err)
			}
			rawValues[i] = uint32(char) // 0 or 1
		} else {
			paramID, err := c.readUint32()
			if err != nil {
				return fmt.Errorf("readFromHeatPump.readUint32.paramID at index %d failed: %w", i, err)
			}

			rawValues[i] = paramID
		}
	}

	return pm.SetRawValues(rawValues)
}

func (c *Client) readUint32() (uint32, error) {
	var buf [SocketReadSizeInteger]byte
	n, err := c.conn.Read(buf[:])
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(buf[:n]), nil
}

func (c *Client) readChar() (byte, error) {
	var buf [SocketReadSizeChar]byte
	n, err := c.conn.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != SocketReadSizeChar {
		return 0, fmt.Errorf("read length %d is not equal to char size of %d", n, SocketReadSizeChar)
	}

	// res := binary.BigEndian.Uint32()
	return buf[0], nil
}

func (c *Client) netRead(b []byte) (int, error) {
	var (
		n, cur, end int
		err         error
	)
	end = len(b)
	for {
		if n, err = c.conn.Read(b[cur:end]); err != nil {
			cur += n
			return cur, fmt.Errorf("netRead failed to read with error: %w", err)
		}
		cur += n
		if cur == end {
			break
		}
	}
	return end, nil
}

// readAndWrite
// Read and/or write value from and/or to heatpump.
// This method is essentially a wrapper for the _read() and _write()
// methods.
// Locking is being used to ensure that only a single socket operation is
// performed at any point in time. This helps to avoid issues with the
// Luxtronik controller, which seems unstable otherwise.
// If write is true, all parameters will be written to the heat pump
// prior to reading back in all data from the heat pump. If write is
// false, no data will be written, but all available data will be read
// from the heat pump.
// :param Parameters() parameters  Parameter dictionary to be written
//
//	to the heatpump before reading all available data
//	from the heatpump. At 'None' it is read only.
func (c *Client) netWrite(data ...int32) (int, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	var buf bytes.Buffer // refactor later
	if err := binary.Write(&buf, binary.BigEndian, data); err != nil {
		return 0, fmt.Errorf("netWrite failed to encode: %#v with error: %w", data, err)
	}

	return c.conn.Write(buf.Bytes())
}
