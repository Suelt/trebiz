package core

import (
	"sync"
	"time"
)

type ReqHandler struct {
	BatchTimeOut   int
	BatchSize      int
	NodeName       string
	TimeOutControl *time.Timer
	ReqPool        *Pool
	Leader         string
}

type Pool struct {
	BatchStore     []*Request
	BatchStoreLock sync.Mutex
}

type Request struct {
	Cmd []byte
}

type RequestBatch struct {
	Batch []*Request
}
