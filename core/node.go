package core

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/rpc"
	"strconv"
	"sync"
	"time"

	"github.com/treble-h/trebiz/config"
	"github.com/treble-h/trebiz/sign"
	"go.dedis.ch/kyber/v3/share"
)

type MsgStage uint8

type RequestSN uint32

type MsgId struct {
	Sn   RequestSN
	View uint32
}

type reqNum struct {
	nextSN RequestSN
	num    sync.Mutex
}

type execNum struct {
	lastExec RequestSN
	num      sync.Mutex
}

type fastTimer struct {
	stop        bool
	timeControl *time.Timer
}

type MsgCert struct {
	reqBatchStage   MsgStage // record the stage of each request
	reqBatchDigest  []byte   // record the hash of each batch
	prepareQc       *PrepareQc
	fastPrepareQc   *FastPrepareQc
	prepareQcStage  bool //record if had send prepareQc
	commitQc        *CommitQc
	commitQcStage   bool
	prePrepareStore *PrePrepareMsg
	prePareStore    map[uint32]*PrepareMsg
	commitStore     map[uint32]*CommitMsg

	msglock sync.Mutex
}

const (
	received MsgStage = iota
	prepared
	committed
)

const (
	cn, bg, abm, pbm = 0, 1, 2, 3
)

// Define a node
type Node struct {
	reqSn    reqNum
	exeSn    execNum
	rHandler *ReqHandler

	addrStr     string
	replicaId   uint32
	clusterAddr map[uint32]string // map replicaId to address
	clusterPort map[uint32]int    // map replicaId to port
	trans       *NetworkTransport
	maxPool     int

	stageLock sync.Mutex

	viewChangeSeqNumber uint32
	viewChangePeriod    uint32
	autoViewChange      int
	currenView          uint32
	activeView          bool
	viewChangeStore     map[Vcidx]*ViewChangeMsg //store all viewchangeMsg
	viewChangeStage     map[uint32]bool          //record if have sent the ViewChangeMsg
	newViewStore        map[uint32]*NewViewMsg   //key is the view number
	newViewStage        map[uint32]bool          //record if have sent the NewViewMsg

	newViewTimer       Timer
	viewChangeTimeout  time.Duration
	LastNewVewTimeout  time.Duration
	newViewTimerReason string // what triggered the timer
	timerActive        bool   // is the timer running?

	myPrivateKey ed25519.PrivateKey
	publicKey    map[uint32]ed25519.PublicKey //publicKey of all the nodes in the clusterAddr,key replicaId
	tsPriKey     *share.PriShare
	tsPubKey     *share.PubPoly
	fastPriKey   *share.PriShare
	fastPubKey   *share.PubPoly

	reqPool  *Pool
	reqBatch map[MsgId]RequestBatch // record the request of each batch

	partialSigInPrepare     map[MsgId]map[string]map[uint32][]byte //key voteString, replicaId
	partialSigPrepareLock   sync.Mutex
	fastPartialSigInPrepare map[MsgId]map[string]map[uint32][]byte //key voteString, replicaId
	partialSigInCommit      map[MsgId]map[string]map[uint32][]byte //key voteString, replicaId
	partialSigCommitLock    sync.Mutex
	reqCertStore            map[MsgId]*MsgCert

	f               int                                  // byzantine nodes
	tprep           int                                  // Quorum threshold
	tcommit         int                                  // Quorum threshold
	h               uint32                               // low watermark
	T               uint32                               // checkpoint period
	K               uint32                               //h+k is the high watermark
	replicaCount    int                                  // number of replicas; PBFT `|R|`
	chkpts          map[uint32]string                    // state checkpoints，key is the sequence, value is state
	checkpointStore map[RequestSN]map[CheckpointMsg]bool //track checkpoints as set

	rpcListenPort   int
	p2pListenPort   int
	shutdownCh      chan struct{}
	execReqBuffer   map[RequestSN][]byte
	execReqIdBuffer map[RequestSN]MsgId

	fastTimeout           int
	fastQcQuorum          int
	viewChangeQuorum      int
	prePrepareSubsetCount int
	prepareTimer          map[MsgId]*fastTimer

	nodeType      int
	evilPR        int
	SameIpTimeout int

	HardcodedBatch   []*Request
	HardcodedPayload []byte

	Simlatency float64

	reqBatchLock sync.Mutex

	certMapLock sync.Mutex

	prepTimerLock sync.Mutex

	checkPointLock sync.Mutex

	executeLock sync.Mutex
}

func NewNode(conf *config.Config) *Node {

	var n Node
	n.reqSn.nextSN = 1
	n.exeSn.lastExec = 0
	n.addrStr = conf.AddrStr
	n.replicaId = conf.ReplicaId
	n.clusterAddr = conf.ClusterAddr
	n.clusterPort = conf.ClusterPort
	n.currenView = 0
	n.activeView = true

	n.partialSigInPrepare = make(map[MsgId]map[string]map[uint32][]byte)
	n.fastPartialSigInPrepare = make(map[MsgId]map[string]map[uint32][]byte)
	n.partialSigInCommit = make(map[MsgId]map[string]map[uint32][]byte)
	n.reqCertStore = make(map[MsgId]*MsgCert)
	n.reqBatch = make(map[MsgId]RequestBatch)

	n.chkpts = make(map[uint32]string)
	n.checkpointStore = make(map[RequestSN]map[CheckpointMsg]bool)
	n.viewChangeStore = make(map[Vcidx]*ViewChangeMsg)
	n.viewChangeStage = make(map[uint32]bool)
	n.newViewStore = make(map[uint32]*NewViewMsg)
	n.newViewStage = make(map[uint32]bool)

	n.myPrivateKey = conf.MyPrivateKey
	n.publicKey = conf.PublicKeyMap
	n.tsPriKey = conf.TsPriKey
	n.tsPubKey = conf.TsPubKey
	n.tprep = conf.PrepQuorum
	n.tcommit = conf.CommitQuorum

	fmt.Println(n.tprep, n.tcommit)

	n.Simlatency = conf.Simlatency
	// n.fastPriKey = conf.FastPriKey
	// n.fastPubKey = conf.FastPubKey

	n.replicaCount = len(n.clusterAddr)
	n.f = int(math.Ceil(float64(n.replicaCount)) / 3.0)

	n.h = 0
	n.T = conf.CheckPointT
	n.K = conf.LogK
	n.rpcListenPort = conf.RPCListenPort
	n.p2pListenPort = conf.P2PListenPort

	n.execReqBuffer = make(map[RequestSN][]byte)
	n.execReqIdBuffer = make(map[RequestSN]MsgId)
	n.prepareTimer = make(map[MsgId]*fastTimer)

	n.reqPool = &Pool{}
	n.rHandler = &ReqHandler{
		BatchTimeOut: conf.BatchTimeout,
		BatchSize:    conf.BatchSize,
		NodeName:     "Node" + strconv.Itoa(int(n.replicaId)),
		ReqPool:      n.reqPool,
		Leader:       n.clusterAddr[0],
	}
	n.maxPool = conf.MaxPool
	n.fastTimeout = conf.FastTimeout
	n.fastQcQuorum = conf.FastQcQuorum
	n.viewChangeQuorum = conf.ViewChangeQuorum
	n.prePrepareSubsetCount = conf.PrePrepareSubsetCount
	n.nodeType = conf.NodeType
	n.evilPR = conf.EvilPR

	n.SameIpTimeout = conf.SameIpTimeout
	n.viewChangeSeqNumber = n.T + n.T/2
	n.viewChangePeriod = 1
	n.timerActive = false
	n.viewChangeTimeout = time.Duration(conf.ViewChangeTimeout) * time.Millisecond
	n.LastNewVewTimeout = time.Duration(conf.ViewChangeTimeout) * time.Millisecond
	n.autoViewChange = conf.AutoViewChange

	hardcodedBytes := make([]byte, 1000)
	for i := 0; i < 512; i++ {
		hardcodedBytes[i] = byte(i)
	}
	for i := 0; i < n.rHandler.BatchSize-1; i++ {
		//fmt.Println(1)
		n.HardcodedBatch = append(n.HardcodedBatch, &Request{Cmd: hardcodedBytes})
	}
	n.HardcodedPayload = hardcodedBytes

	return &n
}

// Establish connections with other nodes
func (n *Node) establishConns() error {
	if n.trans == nil {
		return errors.New("networktransport has not been created")
	}

	for i, addr := range n.clusterAddr {
		// Avoid establishing connection with itself
		if i == n.replicaId {
			continue
		}
		addr = addr + ":" + strconv.Itoa(n.clusterPort[i])

		conn, err := n.trans.GetConn(addr, 0, 0)
		if err != nil {
			fmt.Println(err)
			continue
			//return err
		}

		n.trans.ReturnConn(conn)
		fmt.Printf("Connection from %s to %s has been established\n", n.addrStr+":"+strconv.Itoa(n.p2pListenPort), addr)
	}

	return nil
}

// start the node
// establish the connection with all the other nodes
// accept the request from the client if it is leader
func (n *Node) StartListen() error {

	addr := ":" + strconv.Itoa(n.p2pListenPort)

	var err error
	n.trans, err = NewTCPTransport(addr, 10*time.Second, nil, n.maxPool)
	if err != nil {
		return err
	}
	n.newViewTimer = n.CreateTimer(n.trans)
	return nil

}

func (n *Node) StartConnect() error {
	var err error
	if err = n.establishConns(); err != nil {
		//return err
	}

	return nil
}

func (n *Node) StartPBFT(ch chan error) {
	rpcCh := n.trans.Consumer()
	for {
		select {
		case rpc := <-rpcCh:
			switch msg := rpc.Command.(type) {
			case *PrePrepareMsg:
				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid PrePrepareMsg from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {

					go n.handlePrePrepareMsg(msg, ch)
				}

			case *PrepareMsg:

				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid PrepareMsg from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {

					go n.handlePrepareMsg(msg, ch)
				}

			case *CommitMsg:

				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid CommitMsg from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {

					go n.handleCommitMsg(msg, ch)
				}
			case *PrepareQc:

				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid prepareQc from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {

					go n.handlePrepareQc(msg, ch)
				}
			// case *FastPrepareQc:
			// 	ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
			// 	if !ok {
			// 		fmt.Printf("node %d receive invalid fastPrepareQc from node %d\n", n.replicaId, msg.ReplicaId)
			// 		panic(err)
			// 	} else {
			// 		n.handleFastPrepareQc(msg, ch)
			// 	}
			case *CommitQc:
				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid commitQc from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {

					go n.handleCommitQc(msg, ch)
				}
			case *CheckpointMsg:

				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid CheckpointMsg from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {

					go n.recvCheckpoint(msg, ch)
				}

			case *ViewChangeMsg:

				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid ViewChangeMsg from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {
					n.recvViewChange(msg, ch)
				}

			case *ViewChangeQuorumMsg:
				//new leader send new view
				if n.replicaId == n.primary(n.currenView) {
					n.sendNewView(ch)
				}

			case *NewViewMsg:

				ok, err := n.verifyRsaSignature(msg, msg.ReplicaId, rpc.Sig)
				if !ok {
					fmt.Printf("node %d receive invalid NewViewMsg from node %d\n", n.replicaId, msg.ReplicaId)
					panic(err)
				} else {
					n.recvNewView(msg, ch)
				}

			case *ViewChangeTimerEvent:
				fmt.Printf("node %d send viewChange because of timeout\n", n.replicaId)
				n.sendViewChange(ch)

			default:
			}
		}
	}
}

// Given a digest/view/seq, is there an entry in the certLog?
// If so, return it. If not, create it.
func (n *Node) getCert(idx MsgId) (cert *MsgCert) {
	n.certMapLock.Lock()
	cert, ok := n.reqCertStore[idx]
	if ok {
		n.certMapLock.Unlock()
		return
	}
	cert = &MsgCert{}
	cert.prePareStore = make(map[uint32]*PrepareMsg)
	cert.commitStore = make(map[uint32]*CommitMsg)
	n.reqCertStore[idx] = cert
	n.certMapLock.Unlock()
	return
}

// broadcast the msg to each node, excluding the addrs in excAddrs
func (n *Node) broadcast(ty MsgType, msg interface{}, excAddrs map[uint32]bool, sn RequestSN) error {

	if n.nodeType != cn {
		// randomNum := getRandomNum()
		// fmt.Printf("randomNum %d\n", randomNum)
		// if randomNum >= n.evilPR {
		// 	fmt.Printf("it‘s randomNum is %d, avoid broadcasting\n", randomNum)
		// 	return nil
		// }
		return nil
	}
	//
	encodedBytes, err := encode(msg)
	if err != nil {
		return err
	}

	sig := sign.SignWithPrikey(encodedBytes, n.myPrivateKey)

	for i, addr := range n.clusterAddr {
		if excAddrs != nil {
			if _, ok := excAddrs[i]; ok {
				continue
			}
		}

		go func(index uint32, addr string) {
			var conn *NetConn
			if conn, err = n.trans.GetConn(addr+":"+strconv.Itoa(n.clusterPort[index]), sn, index); err != nil {
				fmt.Println("cannot get a connection:", err)
				//return err
			}
			//  defer n.trans.ReturnConn(conn)
			if addr == n.addrStr {
				time.Sleep(time.Duration(n.SameIpTimeout) * time.Millisecond)
			}

			if err = SendRPC(conn, ty, msg, sig); err != nil {
				fmt.Println("cannot send rpc:", err)
				//return err
			}

			if err = n.trans.ReturnConn(conn); err != nil {
				fmt.Println("cannot release conn:", err)
			}
		}(i, addr)

	}

	return nil
}

func (n *Node) sendToLeader(ty MsgType, msg interface{}, leadAddr string) error {

	if n.nodeType != cn {
		// randomNum := getRandomNum()
		// fmt.Printf("Node %d,randomNum %d \n", n.replicaId, randomNum)
		// if randomNum >= n.evilPR {
		// 	fmt.Printf("Node %d is not a correct node,it‘s randomNum is %d, avoid broadcasting\n", n.replicaId, randomNum)
		// 	return nil
		// }
		return nil
	}
	//
	encodedBytes, err := encode(msg)
	if err != nil {
		return err
	}

	sig := sign.SignWithPrikey(encodedBytes, n.myPrivateKey)
	var conn *NetConn

	if conn, err = n.trans.GetConn(leadAddr, 0, 0); err != nil {
		fmt.Println("cannot get a connection")
		return err
	}

	if leadAddr == n.addrStr {

		time.Sleep(10 * time.Millisecond)

	}

	if err = SendRPC(conn, ty, msg, sig); err != nil {
		return err
	}

	if err = n.trans.ReturnConn(conn); err != nil {
		return err
	}

	return nil
}

func (n *Node) StartRPCListen() error {
	err := rpc.Register(n.rHandler)
	if err != nil {
		return err
	}

	rpc.HandleHTTP()

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(n.rpcListenPort))
	if err != nil {
		log.Fatal("Request listen error: ", err)
	}

	go http.Serve(listener, nil)

	go func(l net.Listener) {
		<-n.shutdownCh
		l.Close()
	}(listener)

	return nil
}

func (n *Node) verifyRsaSignature(msg interface{}, sendId uint32, sig []byte) (bool, error) {

	encodedBytes, err := encode(msg)
	if err != nil {
		return false, err
	}

	pub, ok := n.publicKey[sendId]
	if !ok {
		fmt.Println("Msg from an unknown sender!")
		return false, nil
	}
	ok, errr := sign.VerifySignEd(encodedBytes, pub, sig)
	return ok, errr

}

func (n *Node) createThresholdSig(msg interface{}, partialSigs [][]byte, pubPloy *share.PubPoly, quorumNum, nodeNum int) ([]byte, error) {

	encodedBytes, err := encode(msg)
	if err != nil {
		return nil, err
	}

	thresholdSig := sign.AssembleIntactTSPartial(partialSigs, pubPloy, encodedBytes, quorumNum, nodeNum)

	return thresholdSig, nil

}
func (n *Node) verifyThresholdSig(msg interface{}, pubPoly *share.PubPoly, thSig []byte) (bool, error) {

	encodedBytes, err := encode(msg)
	if err != nil {
		return false, err
	}

	ok, errr := sign.VerifyTS(pubPoly, encodedBytes, thSig)
	return ok, errr
}

func (n *Node) sendBatch() {

	endNum := n.rHandler.BatchSize

	n.reqPool.BatchStoreLock.Lock()
	if len(n.reqPool.BatchStore) < endNum {
		endNum = len(n.reqPool.BatchStore)
	}
	RequestBatch := &RequestBatch{
		Batch: n.reqPool.BatchStore[0:endNum],
	}

	fmt.Printf("batch len:%d\n", len(RequestBatch.Batch))
	//fmt.Println(RequestBatch.Batch[15].Cmd)
	n.reqPool.BatchStore = n.reqPool.BatchStore[endNum:]
	n.reqPool.BatchStoreLock.Unlock()

	if n.activeView && n.primary(n.currenView) == n.replicaId {
		go n.broadcastPrePrepareMsg(RequestBatch, nil)
	}
}

func (n *Node) ReceiveNewRequestForTest(requestNum int) {

	for i := 0; i < requestNum; i++ {
		n.rHandler.ReceiveNewRequest([]byte(""))
	}
}

func (n *Node) HandleReqBatchLoop() {

	n.reqPool.BatchStoreLock.Lock()
	n.reqPool.BatchStore = append(n.reqPool.BatchStore, n.HardcodedBatch...)
	n.reqPool.BatchStoreLock.Unlock()
	n.rHandler.TimeOutControl = time.NewTimer(time.Millisecond * time.Duration(n.rHandler.BatchTimeOut))

	for {
		n.rHandler.Leader = n.clusterAddr[n.primary(n.currenView)]
		if len(n.reqPool.BatchStore) >= n.rHandler.BatchSize {
			n.sendBatch()
			fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "send a batch because of batchSize")

			n.reqPool.BatchStoreLock.Lock()
			n.reqPool.BatchStore = append(n.reqPool.BatchStore, n.HardcodedBatch...)

			n.reqPool.BatchStoreLock.Unlock()
			n.rHandler.TimeOutControl.Reset(time.Millisecond * time.Duration(n.rHandler.BatchTimeOut))

		}
		select {
		case <-n.rHandler.TimeOutControl.C:
			fmt.Printf(time.Now().Format("2006-01-02 15:04:05") + " ")
			if n.activeView && n.primary(n.currenView) == n.replicaId {

				fmt.Printf("an empty batch because of timeout,the primary node packs a batch of size %d\n", n.rHandler.BatchSize)
				n.rHandler.ReceiveNewRequest(n.HardcodedPayload)
				//n.rHandler.TimeOutControl.Reset(time.Millisecond * time.Duration(n.rHandler.BatchTimeOut))
			}

		default:
		}
	}
}

func (n *Node) HandleReqBatchLoopForTest() {

	n.rHandler.TimeOutControl = time.NewTimer(time.Millisecond * time.Duration(n.rHandler.BatchTimeOut))

	for {
		n.rHandler.Leader = n.clusterAddr[n.primary(n.currenView)]
		if len(n.reqPool.BatchStore) >= n.rHandler.BatchSize {
			n.sendBatch()
			fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "send a batch because of batchSize")
			n.rHandler.TimeOutControl.Reset(time.Millisecond * time.Duration(n.rHandler.BatchTimeOut))
		}
		select {
		case <-n.rHandler.TimeOutControl.C:
			if len(n.reqPool.BatchStore) == 0 {
				fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "an empty batch because of timeout,ignore")
			} else {
				n.sendBatch()
				fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "send a batch because of timeout")
			}
			n.rHandler.TimeOutControl.Reset(time.Millisecond * time.Duration(n.rHandler.BatchTimeOut))
		default:
		}
	}
}

// receive a new request
func (rh *ReqHandler) ReceiveNewRequest(Cmd []byte) error {

	req := &Request{
		append([]byte("cmd"), Cmd...),
	}

	rh.ReqPool.BatchStoreLock.Lock()
	rh.ReqPool.BatchStore = append(rh.ReqPool.BatchStore, req)
	rh.ReqPool.BatchStoreLock.Unlock()
	//*reply = "response from " + rh.NodeName + " and current leaderIp is " + rh.Leader
	return nil
}
