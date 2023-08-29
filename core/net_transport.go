package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-msgpack/codec"
)

type MsgType uint8

const (
	PrePrepareType MsgType = iota
	PrepareType
	CommitType
	PrepareQcType
	FastPrepareQcType
	CommitQcType
	CheckPointType
	ViewChangeType
	ViewChangeQuorumType
	NewViewType
)

var (
	// ErrTransportShutdown is returned when operations on a transport are
	// invoked after it's been terminated.
	ErrTransportShutdown = errors.New("transport shutdown")

	// ErrPipelineShutdown is returned when the pipeline is closed.
	ErrPipelineShutdown = errors.New("append pipeline closed")
)

/*

NetworkTransport provides a network based transport that can be
used to communicate with Raft on remote machines. It requires
an underlying stream layer to provide a stream abstraction, which can
be simple TCP, TLS, etc.

This transport is very simple and lightweight. Each RPC request is
framed by sending a byte that indicates the message type, followed
by the MsgPack encoded request.

The response is an error string followed by the response object,
both are encoded using MsgPack.

InstallSnapshot is special, in that after the RPC request we stream
the entire state. That socket is not re-used as the connection state
is not known if there is an error.

*/

type NetworkTransport struct {
	//connPool     map[string][]*NetConn
	//connPoolLock sync.Mutex
	conn    *NetConn
	maxPool int

	consumeCh chan MsgWithSig

	logger hclog.Logger

	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex

	stream StreamLayer

	// streamCtx is used to cancel existing connection handlers.
	streamCtx     context.Context
	streamCancel  context.CancelFunc
	streamCtxLock sync.RWMutex

	timeout time.Duration
}

// setupStreamContext is used to create a new stream context. This should be
// called with the stream lock held.
func (n *NetworkTransport) setupStreamContext() {
	ctx, cancel := context.WithCancel(context.Background())
	n.streamCtx = ctx
	n.streamCancel = cancel
}

// getStreamContext is used retrieve the current stream context.
func (n *NetworkTransport) getStreamContext() context.Context {
	n.streamCtxLock.RLock()
	defer n.streamCtxLock.RUnlock()
	return n.streamCtx
}

// listen is used to handling incoming connections.
func (n *NetworkTransport) listen() {
	const baseDelay = 5 * time.Millisecond
	const maxDelay = 1 * time.Second

	var loopDelay time.Duration
	for {
		// Accept incoming connections
		conn, err := n.stream.Accept()
		if err != nil {
			if loopDelay == 0 {
				loopDelay = baseDelay
			} else {
				loopDelay *= 2
			}

			if loopDelay > maxDelay {
				loopDelay = maxDelay
			}

			if !n.IsShutdown() {
				n.logger.Error("failed to accept connection", "error", err)
			}

			select {
			case <-n.shutdownCh:
				return
			case <-time.After(loopDelay):
				continue
			}
		}
		// No error, reset loop delay
		loopDelay = 0

		n.logger.Debug("accepted connection", "local-address", n.LocalAddr(), "remote-address", conn.RemoteAddr().String())

		// Handle the connection in dedicated routine
		go n.handleConn(n.getStreamContext(), conn)
	}
}

// handleConn is used to handle an inbound connection for its lifespan. The
// handler will exit when the passed context is cancelled or the connection is
// closed.
func (n *NetworkTransport) handleConn(connCtx context.Context, conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	dec := codec.NewDecoder(r, &codec.MsgpackHandle{})
	enc := codec.NewEncoder(w, &codec.MsgpackHandle{})

	for {
		select {
		case <-connCtx.Done():
			n.logger.Debug("stream layer is closed")
			return
		default:
		}

		if err := n.handleCommand(r, dec, enc); err != nil {
			if err != io.EOF {
				n.logger.Error("failed to decode incoming command", "error", err)
			}
			return
		}
		if err := w.Flush(); err != nil {
			n.logger.Error("failed to flush response", "error", err)
			return
		}
	}
}

// handleCommand is used to decode and dispatch a single command.
func (n *NetworkTransport) handleCommand(r *bufio.Reader, dec *codec.Decoder, enc *codec.Encoder) error {
	// Get the rpc type
	rpcType, err := r.ReadByte()
	if err != nil {
		return err
	}

	// Create the RPC object
	respCh := make(chan RPCResponse, 1)

	rpc := MsgWithSig{
		RespChan: respCh,
	}

	// Decode the command
	switch MsgType(rpcType) {
	case PrePrepareType:
		var req PrePrepareMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case PrepareType:
		var req PrepareMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case CommitType:
		var req CommitMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req
		//rpc.Reader = io.LimitReader(r, req.Size)
	case CheckPointType:
		var req CheckpointMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case ViewChangeType:
		var req ViewChangeMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case ViewChangeQuorumType:
		var req ViewChangeQuorumMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case NewViewType:
		var req NewViewMsg
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case CommitQcType:
		var req CommitQc
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case PrepareQcType:
		var req PrepareQc
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	case FastPrepareQcType:
		var req FastPrepareQc
		if err := dec.Decode(&req); err != nil {
			return err
		}
		rpc.Command = &req

	default:
		return fmt.Errorf("unknown rpc type %d", rpcType)
	}

	var sig []byte
	if err := dec.Decode(&sig); err != nil {
		return err
	}

	rpc.Sig = sig

	// Dispatch the RPC
	select {
	case n.consumeCh <- rpc:
	case <-n.shutdownCh:
		return ErrTransportShutdown
	}

	// Wait for response
	//select {
	//case resp := <-respCh:
	//	// Send the error first
	//	respErr := ""
	//	if resp.Error != nil {
	//		respErr = resp.Error.Error()
	//	}
	//	if err := enc.Encode(respErr); err != nil {
	//		return err
	//	}
	//
	//	// Send the response
	//	if err := enc.Encode(resp.Response); err != nil {
	//		return err
	//	}
	//case <-n.shutdownCh:
	//	return ErrTransportShutdown
	//}
	return nil
}

// LocalAddr implements the Transport interface.
func (n *NetworkTransport) LocalAddr() string {
	return n.stream.Addr().String()
}

// IsShutdown is used to check if the transport is shutdown.
func (n *NetworkTransport) IsShutdown() bool {
	select {
	case <-n.shutdownCh:
		return true
	default:
		return false
	}
}

// Consumer implements the Transport interface.
func (n *NetworkTransport) Consumer() <-chan MsgWithSig {
	return n.consumeCh
}

// Close is used to stop the network transport.
func (n *NetworkTransport) Close() error {
	n.shutdownLock.Lock()
	defer n.shutdownLock.Unlock()

	if !n.shutdown {
		close(n.shutdownCh)
		n.stream.Close()
		n.shutdown = true
	}
	return nil
}

type NetConn struct {
	target string
	conn   net.Conn
	w      *bufio.Writer
	dec    *codec.Decoder
	enc    *codec.Encoder
}

func (n *NetConn) Release() error {
	return n.conn.Close()
}

// NetworkTransportConfig encapsulates configuration for the network transport layer.
type NetworkTransportConfig struct {
	Logger  hclog.Logger
	MaxPool int
	// Dialer
	Stream StreamLayer

	// Timeout is used to apply I/O deadlines. For InstallSnapshot, we multiply
	// the timeout by (SnapshotSize / TimeoutScale).
	Timeout time.Duration
}

// NewNetworkTransportWithConfig creates a new network transport with the given config struct
func NewNetworkTransportWithConfig(
	config *NetworkTransportConfig,
) *NetworkTransport {
	if config.Logger == nil {
		config.Logger = hclog.New(&hclog.LoggerOptions{
			Name:   "pbft-net",
			Output: hclog.DefaultOutput,
			Level:  hclog.DefaultLevel,
		})
	}
	trans := &NetworkTransport{
		//	connPool:   make(map[string][]*NetConn),
		consumeCh:  make(chan MsgWithSig),
		logger:     config.Logger,
		shutdownCh: make(chan struct{}),
		stream:     config.Stream,
		timeout:    config.Timeout,
	}

	// Create the connection context and then start our listener.
	trans.setupStreamContext()
	go trans.listen()

	return trans
}

// NewNetworkTransport creates a new network transport with the given dialer
// and listener. The maxPool controls how many connections we will pool. The
// timeout is used to apply I/O deadlines. For InstallSnapshot, we multiply
// the timeout by (SnapshotSize / TimeoutScale).
func NewNetworkTransport(
	stream StreamLayer,
	timeout time.Duration,
	logOutput io.Writer,
	maxPool int,
) *NetworkTransport {
	if logOutput == nil {
		logOutput = os.Stderr
	}
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "pbft-net",
		Output: logOutput,
		Level:  hclog.DefaultLevel,
	})
	config := &NetworkTransportConfig{Stream: stream, Timeout: timeout, Logger: logger, MaxPool: maxPool}
	return NewNetworkTransportWithConfig(config)
}

// sendRPC is used to encode and send the RPC.
func SendRPC(conn *NetConn, rpcType MsgType, args interface{}, sig []byte) error {
	// Write the request type
	if err := conn.w.WriteByte(uint8(rpcType)); err != nil {
		conn.Release()
		return err
	}
	// Send the request
	if err := conn.enc.Encode(args); err != nil {
		conn.Release()
		return err
	}

	// Send the sig
	if err := conn.enc.Encode(sig); err != nil {
		conn.Release()
		return err
	}

	// Flush
	if err := conn.w.Flush(); err != nil {
		conn.Release()
		return err
	}
	return nil
}

// decodeResponse is used to decode an RPC response and reports whether
// the connection can be reused.
func decodeResponse(conn *NetConn, resp interface{}) (bool, error) {
	// Decode the error if any
	var rpcError string
	if err := conn.dec.Decode(&rpcError); err != nil {
		conn.Release()
		return false, err
	}

	// Decode the response
	if err := conn.dec.Decode(resp); err != nil {
		conn.Release()
		return false, err
	}

	// Format an error if any
	if rpcError != "" {
		return true, fmt.Errorf(rpcError)
	}
	return true, nil
}
