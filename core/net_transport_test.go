package core

import (
	"reflect"
	"testing"
	"time"
)

// TestSimpleComm tests if node1 (addr1, client) can connect to node2 (addr2, server) correctly
// And if node1 can send message of type 'MsgType1' to node2
// And if node2 can respond with the message of type 'MsgType1Resp' to node1
func TestSimpleComm(t *testing.T) {
	addr1 := "127.0.0.1:8888"
	tran1, _ := NewTCPTransport(addr1, 2*time.Second, nil, 1)
	defer tran1.Close()
	rpcCh := tran1.Consumer()

	args := PrePrepareMsg{BatchHash: []byte("A msg")}
	//resp := PrepareMsg{ Vote.Cmd: []byte("A response"),}

	// Listen for a request
	go func() {
		select {
		case rpc := <-rpcCh:
			// Verify the command
			req := rpc.Command.(*PrePrepareMsg)
			if !reflect.DeepEqual(req, &args) {
				t.Errorf("command mismatch: %#v %#v", req, args)
				return
			}

		//	rpc.Respond(&resp, nil)

		case <-time.After(2000 * time.Millisecond):
			t.Errorf("timeout")
		}
	}()

	addr2 := "127.0.0.1:9999"
	tran2, _ := NewTCPTransport(addr2, 2*time.Second, nil, 1)
	defer tran2.Close()

	conn, err := tran2.GetConn(addr1, 0, 0)
	if err != nil {
		t.Errorf(err.Error())
	}

	//var out PrepareMsg
	sig := []byte("1111")

	if err := SendRPC(conn, PrePrepareType, &args, sig); err != nil {
		t.Errorf(err.Error())
	}

	//// Decode the response
	//if _, err = decodeResponse(conn, &out); err != nil {
	//	t.Error(err.Error())
	//}

	//if !reflect.DeepEqual(out, resp) {
	//	t.Errorf("response mismatch: %#v %#v", out, resp)
	//	return
	//}

}

// TestReusageOfConn tests if node1 can reuses an old connection with node2
func TestReusageOfConn(t *testing.T) {
	addr1 := "127.0.0.1:8888"
	tran1, _ := NewTCPTransport(addr1, 2*time.Second, nil, 1)
	defer tran1.Close()
	rpcCh := tran1.Consumer()

	args := PrePrepareMsg{BatchHash: []byte("A msg")}
	//	resp := PrepareMsg{ Cmd: []byte("A response"),}

	// Listen for a request
	go func() {
		select {
		case rpc := <-rpcCh:
			// Verify the command
			req := rpc.Command.(*PrePrepareMsg)
			if !reflect.DeepEqual(req, &args) {
				t.Errorf("command mismatch: %#v %#v", req, args)
				return
			}

		//	rpc.Respond(&resp, nil)

		case <-time.After(20000 * time.Millisecond):
			t.Errorf("timeout")
		}
	}()

	addr2 := "127.0.0.1:9999"
	tran2, _ := NewTCPTransport(addr2, 2*time.Second, nil, 1)
	defer tran2.Close()

	conn, err := tran2.GetConn(addr1, 0, 0)
	if err != nil {
		t.Errorf(err.Error())
	}

	tran2.ReturnConn(conn)

	// From now, try to reuse the connection
	// Sleep for a while to check if the existing connection is robust
	time.Sleep(10 * time.Second)

	conn2, err := tran2.GetConn(addr1, 0, 0)
	if err != nil {
		t.Errorf(err.Error())
	}
	sig := []byte("1111")
	//var out PrepareMsg
	if err := SendRPC(conn2, PrePrepareType, &args, sig); err != nil {
		t.Errorf(err.Error())
	}

	//// Decode the response
	//if _, err = decodeResponse(conn2, &out); err != nil {
	//	t.Error(err.Error())
	//}
	//
	//if !reflect.DeepEqual(out, resp) {
	//	t.Errorf("response mismatch: %#v %#v", out, resp)
	//	return
	//}

}
