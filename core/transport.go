package core

import (
	"io"
)

// RPCResponse captures both a response and a potential error.
type RPCResponse struct {
	Response interface{}
	Error    error
}

// RPC has a command, and provides a response mechanism.
type MsgWithSig struct {
	Command   interface{}
	Sig       []byte
	Reader    io.Reader // Set only for InstallSnapshot
	RespChan  chan<- RPCResponse
	Timestamp int64
}

// Respond is used to respond with a response, error or both
func (r *MsgWithSig) Respond(resp interface{}, err error) {
	r.RespChan <- RPCResponse{resp, err}
}
