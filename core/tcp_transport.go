package core

import (
	"bufio"
	"fmt"
	"github.com/hashicorp/go-msgpack/codec"
	"io"
	"net"
	"time"
)

// StreamLayer is used with the NetworkTransport to provide
// the low level stream abstraction.
type StreamLayer interface {
	net.Listener

	// Dial is used to create a new outgoing connection
	Dial(address string, timeout time.Duration) (net.Conn, error)
}

// TCPStreamLayer implements StreamLayer interface for plain TCP.
type TCPStreamLayer struct {
	listener *net.TCPListener
}

// Dial implements the StreamLayer interface.
func (t *TCPStreamLayer) Dial(address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", string(address), timeout)
}

// Accept implements the net.Listener interface.
func (t *TCPStreamLayer) Accept() (c net.Conn, err error) {
	return t.listener.Accept()
}

// Close implements the net.Listener interface.
func (t *TCPStreamLayer) Close() (err error) {
	return t.listener.Close()
}

// Addr implements the net.Listener interface.
func (t *TCPStreamLayer) Addr() net.Addr {
	return t.listener.Addr()
}

func (n *NetworkTransport) dialConn(target string, temp1 RequestSN, temp2 uint32) (*NetConn, error) {
	// Dial a new connection

	conn, err := n.stream.Dial(target, n.timeout)

	if err != nil {
		return nil, err
	}

	// Wrap the conn
	netC := &NetConn{
		target: target,
		conn:   conn,
		dec:    codec.NewDecoder(bufio.NewReader(conn), &codec.MsgpackHandle{}),
		w:      bufio.NewWriter(conn),
	}

	netC.enc = codec.NewEncoder(netC.w, &codec.MsgpackHandle{})

	//Done
	return netC, nil
}

func (n *NetworkTransport) GetConn(target string, temp1 RequestSN, temp2 uint32) (*NetConn, error) {
	/*n.connPoolLock.Lock()
	defer n.connPoolLock.Unlock()
	// Check for an exiting conn
	netConns, ok := n.connPool[target]
	if ok && len(netConns) > 0 {
		var netC *NetConn
		num := len(netConns)
		netC, netConns[num-1] = netConns[num-1], nil
		n.connPool[target] = netConns[:num-1]
		return netC, nil
	}
	*/
	return n.dialConn(target, temp1, temp2)
}

// To avoid establishing connections repeatedly, try to maintain the net connection for later reusage
func (n *NetworkTransport) ReturnConn(netC *NetConn) error {
	/*n.connPoolLock.Lock()
	defer n.connPoolLock.Unlock()

	key := netC.target
	netConns, _ := n.connPool[key]

	if !n.IsShutdown() && len(netConns) < n.maxPool {
		n.connPool[key] = append(netConns, netC)
		return nil
	} else
	{
		NetConnNumberLock.Lock()
		NetConnNumber[key]--
		NetConnNumberLock.Unlock()
		return netC.Release()
	}*/
	if err := netC.Release(); err != nil {
		fmt.Println("cannot release conn:", err)
		return err
	}
	return nil
	//return netC.Release()
}

func newTCPTransport(bindAddr string,
	transportCreator func(stream StreamLayer) *NetworkTransport) (*NetworkTransport, error) {
	// Try to bind
	list, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	// Create stream
	stream := &TCPStreamLayer{
		listener: list.(*net.TCPListener),
	}

	// Create the network transport
	trans := transportCreator(stream)
	return trans, nil
}

// NewTCPTransport returns a NetworkTransport that is built on top of
// a TCP streaming transport layer.
func NewTCPTransport(
	bindAddr string,
	timeout time.Duration,
	logOutput io.Writer,
	maxPool int,
) (*NetworkTransport, error) {
	return newTCPTransport(bindAddr, func(stream StreamLayer) *NetworkTransport {
		return NewNetworkTransport(stream, timeout, logOutput, maxPool)
	})
}
