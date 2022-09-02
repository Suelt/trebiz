package core

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/treble-h/trebiz/config"
	"github.com/treble-h/trebiz/sign"
	"testing"
	"time"
)

func judgeNodeType(i int, b []int) bool {
	for _, v := range b {
		if i == v {
			return true
		}
	}
	return false
}

func makeNodes(nodeNumber int, bg, abm, pbm []int) ([]*Node, error) {

	clusterAddr := make(map[uint32]string)
	p2pClusterPort := make(map[uint32]int)

	privKeys := make([]ed25519.PrivateKey, nodeNumber)
	pubKeys := make(map[uint32]ed25519.PublicKey)

	// create the ED25519 keys
	for i := 0; i < nodeNumber; i++ {
		clusterAddr[uint32(i)] = "127.0.0.1"
		p2pClusterPort[uint32(i)] = 8000 + i

		privKeys[i], pubKeys[uint32(i)], _ = sign.GenKeys()
	}

	// create the threshold keys
	numT := nodeNumber - nodeNumber/3
	shares, pubPoly := sign.GenTSKeys(numT, nodeNumber)

	// create the threshold keys for fast path

	fastQuorum := nodeNumber - len(abm) - len(pbm)/2
	viewChangeQuorum := 2*(len(bg)+len(abm)+len(pbm)) + 1 + len(abm) + len(pbm)
	prePrepareSubsetCount := len(bg) + len(abm) + len(pbm) + 1 + (len(pbm)+1)/2

	fastShares, fastPubPoly := sign.GenTSKeys(fastQuorum, nodeNumber)

	if len(shares) != nodeNumber {
		return []*Node{}, errors.New("number of generated private keys is incorrect")
	}

	confs := make([]*config.Config, nodeNumber)
	nodes := make([]*Node, nodeNumber)

	for i := 0; i < nodeNumber; i++ {

		confs[i] = config.New(clusterAddr[uint32(i)], clusterAddr, p2pClusterPort, uint32(i),
			privKeys[i], pubKeys, shares[i], pubPoly, fastShares[i], fastPubPoly, p2pClusterPort[uint32(i)],
			9000+i, 10, 1, 2, 4, 3, 8, 5, fastQuorum, viewChangeQuorum, prePrepareSubsetCount, 1, 200, 200, 0)

		if judgeNodeType(i, bg) {
			confs[i].NodeType = 1
		} else if judgeNodeType(i, abm) {
			confs[i].NodeType = 2
		} else if judgeNodeType(i, pbm) {
			confs[i].NodeType = 3
		} else {
			confs[i].NodeType = 0
		}
		fmt.Printf("Node %d type %d\n", i, confs[i].NodeType)
		nodes[i] = NewNode(confs[i])

		if err := nodes[i].StartListen(); err != nil {
			panic(err)
		}

	}

	for i := 0; i < nodeNumber; i++ {

		if err := nodes[i].StartConnect(); err != nil {
			panic(err)
		}

	}

	//Wait the all the connections to be established
	time.Sleep(time.Second * 2)

	for i := 0; i < nodeNumber; i++ {
		//errs[i] = make(chan error, 1)
		go nodes[i].StartPBFT(nil)
	}

	return nodes, nil
}

func TestNormalCase4Nodes(t *testing.T) {

	bg := []int{}
	abm := []int{1}
	pbm := []int{2}
	Nodes, _ := makeNodes(4, bg, abm, pbm)
	var reply string
	go Nodes[0].HandleReqBatchLoop()

	Nodes[0].rHandler.ReceiveNewRequest([]byte("a"), &reply)

	time.Sleep(time.Second * 2)

}

func Test4NodesConstantMsg(t *testing.T) {

	bg := []int{}
	abm := []int{1}
	pbm := []int{2}
	Nodes, _ := makeNodes(4, bg, abm, pbm)
	var reply string
	go Nodes[0].HandleReqBatchLoop()

	Nodes[0].rHandler.ReceiveNewRequest([]byte("a"), &reply)

	Nodes[0].rHandler.ReceiveNewRequest([]byte("a"), &reply)

	time.Sleep(time.Second * 5)
}

func TestCheckpoint(t *testing.T) {
	bg := []int{}
	abm := []int{}
	pbm := []int{1}
	Nodes, _ := makeNodes(4, bg, abm, pbm)
	var reply string
	go Nodes[0].HandleReqBatchLoop()

	Nodes[0].rHandler.ReceiveNewRequest([]byte("LLL"), &reply)

	Nodes[0].rHandler.ReceiveNewRequest([]byte("SSS"), &reply)

	time.Sleep(time.Second * 2)

	targetLowWaterMark := uint32(2)

	for i := 0; i < 4; i++ {
		if Nodes[i].h != targetLowWaterMark {
			t.Fatalf("Node %d dosen't update low watermark,it's h is %d\n", i, Nodes[i].h)
		} else {
			fmt.Printf("Node %d updates low watermark to %d\n", i, Nodes[i].h)
		}
	}
}

func TestViewChange(t *testing.T) {

	bg := []int{}
	abm := []int{}
	pbm := []int{}

	Nodes, _ := makeNodes(4, bg, abm, pbm)
	for i := 0; i < 4; i++ {
		Nodes[i].autoViewChange = 1
	}
	var reply string
	go Nodes[0].HandleReqBatchLoop()

	for i := 0; i < 3; i++ {
		time.Sleep(time.Millisecond * 200)
		Nodes[0].rHandler.ReceiveNewRequest([]byte("LLL"), &reply)
	}

	time.Sleep(time.Second * 2)

	targetView := uint32(1)
	targetLowWaterMark := uint32(6)

	for i := 0; i < 4; i++ {
		if Nodes[i].currenView != targetView {
			t.Fatalf("Node %d dosen't update view,it's view is %d\n", i, Nodes[i].currenView)
		} else {
			fmt.Printf("Node %d updates to view %d\n", i, Nodes[i].currenView)
		}

		if Nodes[i].h != targetLowWaterMark {
			t.Fatalf("Node %d dosen't update low watermark,it's h is %d\n", i, Nodes[i].h)
		} else {
			fmt.Printf("Node %d updates low watermark to %d\n", i, Nodes[i].h)
		}

	}
	initialS, ok := Nodes[1].selectInitialSequence(Nodes[1].getViewChangeMsgs())
	if !ok || initialS != 2 {
		t.Fatalf("Wrong new initial sn of checkpoint: %d",
			initialS)
	}
}

func TestViewChangeCheckpointSelection(t *testing.T) {
	bg := []int{}
	abm := []int{}
	pbm := []int{}
	Nodes, _ := makeNodes(1, bg, abm, pbm)

	vset := make([]ViewChangeMsg, 3)

	// Replica 0 sent checkpoints for 5
	vset[0] = ViewChangeMsg{
		LastStableCk: 5,
		Cset: []CheckpointMsg{
			{
				SeqN:        10,
				StateDigest: "ten",
			},
		},
	}

	// Replica 1 sent checkpoints for 5
	vset[1] = ViewChangeMsg{
		LastStableCk: 5,
		Cset: []CheckpointMsg{
			{
				SeqN:        5,
				StateDigest: "five",
			},
		},
	}

	// Replica 2 sent checkpoints for 10
	vset[2] = ViewChangeMsg{
		LastStableCk: 10,
		Cset: []CheckpointMsg{
			{
				SeqN:        15,
				StateDigest: "fifteen",
			},
		},
	}

	checkpointSn, ok := Nodes[0].selectInitialSequence(vset)

	if !ok {
		t.Fatalf("Failed to pick correct a checkpoint for view change")
	}

	expected := uint32(10)
	if checkpointSn != expected {
		t.Fatalf("Expected to pick checkpoint %d, but picked %d", expected, checkpointSn)
	}
}

func TestDiffSubet(t *testing.T) {

	bg := []int{}
	abm := []int{}
	pbm := []int{}
	Nodes, _ := makeNodes(1, bg, abm, pbm)
	Nodes[0].prePrepareSubsetCount = 2

	vset := make([]ViewChangeMsg, 4)
	// Replica 0 sent checkpoints for 5
	vset[0] = ViewChangeMsg{
		LastStableCk: 5,
		Pset: map[RequestSN]*PrePrepareMsg{
			6: {
				SN:        6,
				BatchHash: []byte("1"),
			},
		},
	}

	// Replica 1 sent checkpoints for 5
	vset[1] = ViewChangeMsg{
		LastStableCk: 5,
		Pset: map[RequestSN]*PrePrepareMsg{
			6: {
				SN:        6,
				BatchHash: []byte("1"),
			},
		},
	}

	// Replica 2 sent checkpoints for 10
	vset[2] = ViewChangeMsg{
		LastStableCk: 5,
		Pset: map[RequestSN]*PrePrepareMsg{
			6: {
				SN:        6,
				BatchHash: []byte("2"),
			},
		},
	}

	vset[3] = ViewChangeMsg{
		LastStableCk: 5,
		Pset: map[RequestSN]*PrePrepareMsg{
			6: {
				SN:        6,
				BatchHash: []byte("2"),
			},
		},
	}

	_, ok := Nodes[0].assignSequenceNumbers(vset, 5)

	if ok {
		t.Fatalf("Failed to handle viewChange properly")
	}

}
