package core

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/treble-h/trebiz/sign"
	"math"
	"strconv"
	"time"
)

var fastQcTime int64

func (n *Node) createPrePrepareMsg(reqBatch *RequestBatch) *PrePrepareMsg {

	batchHash, err := dataHashByte(*reqBatch)
	if err != nil {
		fmt.Println("hash reqbatch err!")
	}

	var PPM = PrePrepareMsg{
		BatchHash: batchHash,
		ReqBatch:  *reqBatch,
		View:      n.currenView,
		ReplicaId: n.replicaId,
	}

	n.reqSn.num.Lock()
	PPM.SN = n.reqSn.nextSN
	n.reqSn.nextSN++
	n.reqSn.num.Unlock()
	PPM.TimeStamp = time.Now().UnixNano()
	fmt.Printf(time.Now().Format("2006-01-02 15:04:05")+"PrePrepareMsg with sn :%d is created.\n", PPM.SN)
	return &PPM
}

func (n *Node) broadcastPrePrepareMsg(reqBatch *RequestBatch, err chan error) {

	if !n.inWatermarks(n.reqSn.nextSN) || uint32(n.reqSn.nextSN) > n.h+n.K/2 {
		// We don't have the necessary stable certificates to advance our watermarks
		fmt.Printf("Primary %d not sending pre-prepare for current batch - out of sequence numbers\n", n.replicaId)
		return
	}

	ppm := n.createPrePrepareMsg(reqBatch)
	Id := MsgId{ppm.SN, ppm.View}

	BatchString := hex.EncodeToString(ppm.BatchHash)
	n.prepareTimer[Id] = &fastTimer{
		false,
		time.NewTimer(time.Millisecond * time.Duration(n.fastTimeout)),
	}
	go func() {
		select {
		case <-n.prepareTimer[Id].timeControl.C:
			n.prepareTimer[Id].stop = true
			n.prepareTimer[Id].timeControl.Stop()
			//if prepared, send prepareQc
			n.checkIfPrepareQc(Id, BatchString)
		}
	}()

	fmt.Printf("node %d broadcast pre-prepare, sn:%d, v:%d\n",
		n.replicaId, ppm.SN, ppm.View)

	if errBC := n.broadcast(PrePrepareType, ppm, nil, ppm.SN); errBC != nil {
		err <- errBC
	}

}

// handle the prepreparemsg
func (n *Node) handlePrePrepareMsg(ppm *PrePrepareMsg, err chan error) {

	fmt.Printf("node %d receive pre-prepare of sn:%d from node %d\n", n.replicaId, ppm.SN, ppm.ReplicaId)

	msg := ppm //prepreparemsg
	// check if the prepreparemsg is from the leader
	if msg.ReplicaId != n.primary(n.currenView) {
		err <- errors.New("prepreparemsg from a non-leader node")
		return
	}

	if !n.activeView {
		err <- errors.New("node is in viewchange and ignores the preprepare msg")
		return
	}

	/*
		if !n.inWatermarks(uint32(msg.SN)){
			fmt.Printf("The sequence %d of preprepare is out of the watermark\n",msg.SN)
			return
		}
	*/

	Id := MsgId{msg.SN, msg.View}

	cert := n.getCert(Id)

	// check if a prepreparemsg with the same sn has been received before
	if len(cert.reqBatchDigest) != 0 {
		if !bytes.Equal(cert.reqBatchDigest, msg.BatchHash) {
			fmt.Printf("Pre-prepare found for same view/seqNo but different digest: received %s, stored %s", msg.BatchHash, cert.reqBatchDigest)
			//	n.sendViewChange(err)
			return
		} else {
			fmt.Printf("An old prepreparemsg with sn: %d is received again at node%d\n", msg.SN, msg.ReplicaId)
		}
	}

	// store the prepreparemsg locally

	n.reqBatch[Id] = msg.ReqBatch
	cert.reqBatchStage = received
	cert.reqBatchDigest = msg.BatchHash
	cert.prePrepareStore = msg

	//receive prepareQc/commitQc before prePrepare
	n.checkIfPrepared(Id, err)

	// the leader avoid sending the preparemsg to itself
	if n.replicaId != n.primary(n.currenView) {
		n.sendPrepareMsg(msg.SN, err)
	} else {
		//create a partial signature, and then handle it's prepareMsg
		pm := n.createPrepareMsg(msg.SN)
		n.handlePrepareMsg(pm, err)
	}
}

func (n *Node) createPrepareMsg(sn RequestSN) *PrepareMsg {

	cert := n.getCert(MsgId{sn, n.currenView})

	prepareVote := VoteForPrepare{
		SN:        sn,
		View:      n.currenView,
		BatchHash: cert.reqBatchDigest,
	}

	var PM = PrepareMsg{
		Vote:           prepareVote,
		ReplicaId:      n.replicaId,
		PartialSig:     nil,
		FastPartialSig: nil,
	}
	encodedBytes, err := encode(prepareVote)
	if err != nil {
		panic(err)
		return nil
	}
	PM.PartialSig = sign.SignTSPartial(n.tsPriKey, encodedBytes)
	PM.FastPartialSig = sign.SignTSPartial(n.fastPriKey, encodedBytes)
	PM.TimeStamp = time.Now().UnixNano()
	return &PM
}

//send prepareMsg to leader
func (n *Node) sendPrepareMsg(sn RequestSN, err chan error) {
	pm := n.createPrepareMsg(sn)
	leaderId := n.primary(n.currenView)
	leadAddr := n.clusterAddr[leaderId] + ":" + strconv.Itoa(n.clusterPort[leaderId])

	fmt.Printf("Node %d send prepare to primary %d, sn:%d, v:%d\n",
		n.replicaId, leaderId, pm.Vote.SN, pm.Vote.View)

	if errBC := n.sendToLeader(PrepareType, pm, leadAddr); errBC != nil {
		err <- errBC
	}

}

func (n *Node) sendCommitMsg(pqc *PrepareQc, err chan error) {
	cm := n.createCommitMsg(pqc)
	leaderId := n.primary(n.currenView)
	leadAddr := n.clusterAddr[leaderId] + ":" + strconv.Itoa(n.clusterPort[leaderId])

	fmt.Printf("Node %d send commit to primary %d, sn:%d, v:%d\n",
		n.replicaId, leaderId, cm.Vote.SN, cm.Vote.View)

	if errBC := n.sendToLeader(CommitType, cm, leadAddr); errBC != nil {
		err <- errBC
	}

}

// check if a corresponding prepreparemsg has been received before
func (n *Node) checkIfPrePrepared(Id MsgId) bool {
	cert := n.getCert(Id)
	if cert.prePrepareStore == nil {
		fmt.Printf("Node %d hasn't receive preprepare msg of sn %d .\n", n.replicaId, Id.Sn)
		return false
	}
	return true
}

// handle the preparemsg
func (n *Node) handlePrepareMsg(pm *PrepareMsg, err chan error) {

	fmt.Printf("node %d receive prepare of sn:%d from node %d\n", n.replicaId, pm.Vote.SN, pm.ReplicaId)

	/*
		if !n.inWatermarks(uint32(pm.Vote.SN)){
			fmt.Printf("The sequence %d of PrepareMsg is out of the watermark\n",pm.Vote.SN)
			return
		}
	*/

	msg := pm //prepareMsg
	// check if a corresponding prepreparemsg has been received before
	n.checkIfPrePrepared(MsgId{msg.Vote.SN, n.currenView})

	// check if a preparemsg with the same sn has been received from the same sender before
	cert := n.getCert(MsgId{msg.Vote.SN, n.currenView})
	if _, ok := cert.prePareStore[msg.ReplicaId]; ok {
		// has received, warn...
		fmt.Printf("a prepare with the same sn %d  has been received from node: %d before\n", msg.Vote.SN, msg.ReplicaId)
		return
	}

	cert.prePareStore[msg.ReplicaId] = msg
	n.handlePrepareVote(msg)
}

func (n *Node) handlePrepareVote(pm *PrepareMsg) {

	Id := MsgId{
		pm.Vote.SN,
		pm.Vote.View,
	}
	cert := n.getCert(Id)
	_, ok := n.partialSigInPrepare[Id]
	if !ok {
		n.partialSigInPrepare[Id] = make(map[string]map[uint32][]byte)
		n.fastPartialSigInPrepare[Id] = make(map[string]map[uint32][]byte)
	}

	BatchString := hex.EncodeToString(pm.Vote.BatchHash)

	_, ok = n.partialSigInPrepare[Id][BatchString]
	if !ok {
		n.partialSigInPrepare[Id][BatchString] = make(map[uint32][]byte)
		n.fastPartialSigInPrepare[Id][BatchString] = make(map[uint32][]byte)
	}

	n.partialSigInPrepare[Id][BatchString][pm.ReplicaId] = pm.PartialSig
	n.fastPartialSigInPrepare[Id][BatchString][pm.ReplicaId] = pm.FastPartialSig

	if n.prepareTimer[Id].stop == false {
		if len(n.fastPartialSigInPrepare[Id][BatchString]) >= n.fastQcQuorum && !cert.prepareQcStage {
			fmt.Printf("node %d send fastprepareQc of sn:%d \n", n.replicaId, Id.Sn)
			fastQcTime = time.Now().UnixNano()
			cert.prepareQcStage = true
			fastPrepareQcMsg := n.createFastPrepareQc(Id, BatchString)
			n.broadcast(FastPrepareQcType, fastPrepareQcMsg, nil, 0)
		}
	} else {
		n.checkIfPrepareQc(Id, BatchString)
	}

}
func (n *Node) checkIfPrepareQc(Id MsgId, BatchString string) {

	cert := n.getCert(Id)

	if len(n.partialSigInPrepare[Id][BatchString]) >= 2*n.f+1 && !cert.prepareQcStage {
		fmt.Printf("node %d send prepareQc of sn:%d \n", n.replicaId, Id.Sn)
		cert.prepareQcStage = true
		prepareQcMsg := n.createPrepareQc(Id, BatchString)
		n.broadcast(PrepareQcType, prepareQcMsg, nil, prepareQcMsg.SN)
	}

}
func (n *Node) createPrepareQc(Id MsgId, BatchString string) *PrepareQc {

	batchHash, _ := hex.DecodeString(BatchString)
	voteMsg := VoteForPrepare{
		SN:        Id.Sn,
		View:      Id.View,
		BatchHash: batchHash,
	}

	var partialSigs [][]byte
	for _, sig := range n.partialSigInPrepare[Id][BatchString] {
		partialSigs = append(partialSigs, sig)
	}

	prepareQc := PrepareQc{
		ReplicaId:    n.replicaId,
		SN:           Id.Sn,
		View:         Id.View,
		BatchHash:    batchHash,
		ThresholdSig: nil,
	}

	Sig, err := n.createThresholdSig(voteMsg, partialSigs, n.tsPubKey, 2*n.f+1, n.replicaCount)
	if err != nil {
		return nil
	}
	prepareQc.ThresholdSig = Sig
	prepareQc.TimeStamp = time.Now().UnixNano()
	return &prepareQc
}

func (n *Node) createFastPrepareQc(Id MsgId, BatchString string) *FastPrepareQc {

	batchHash, _ := hex.DecodeString(BatchString)
	voteMsg := VoteForPrepare{
		SN:        Id.Sn,
		View:      Id.View,
		BatchHash: batchHash,
	}

	var partialSigs [][]byte
	for _, sig := range n.fastPartialSigInPrepare[Id][BatchString] {
		partialSigs = append(partialSigs, sig)
	}

	fastPrepareQc := FastPrepareQc{
		ReplicaId:    n.replicaId,
		SN:           Id.Sn,
		View:         Id.View,
		BatchHash:    batchHash,
		ThresholdSig: nil,
	}

	Sig, err := n.createThresholdSig(voteMsg, partialSigs, n.fastPubKey, n.fastQcQuorum, n.replicaCount)
	if err != nil {
		return nil
	}
	fastPrepareQc.ThresholdSig = Sig
	fastPrepareQc.TimeStamp = time.Now().UnixNano()
	return &fastPrepareQc
}

func (n *Node) handlePrepareQc(pqc *PrepareQc, err chan error) {
	/*
		if !n.inWatermarks(uint32(pqc.SN)){
			fmt.Printf("The sequence %d of prepareQc is out of the watermark\n",pqc.SN)
			return
		}
	*/
	fmt.Printf("node %d receive prepareQc of sn:%d from node %d\n", n.replicaId, pqc.SN, pqc.ReplicaId)

	if pqc.ReplicaId != n.primary(n.currenView) {
		fmt.Println("prepareQc from a non-leader node")
		//send view change
		return
	}

	Id := MsgId{
		pqc.SN,
		pqc.View,
	}
	cert := n.getCert(Id)

	if cert.prepareQc != nil {
		fmt.Printf("Already has a prepareQc of the same sn %d for this Id!", pqc.SN)
		return
	}

	voteMsg := VoteForPrepare{
		SN:        pqc.SN,
		View:      pqc.View,
		BatchHash: pqc.BatchHash,
	}

	ok, _ := n.verifyThresholdSig(voteMsg, n.tsPubKey, pqc.ThresholdSig)
	if !ok {
		fmt.Printf("node %d receive invalid prepareQc of sn:%d from node %d\n", n.replicaId, pqc.SN, pqc.ReplicaId)
		return
	}

	cert.prepareQc = pqc

	n.checkIfPrepared(Id, err)

	n.sendCommitMsg(pqc, err)
}

func (n *Node) checkIfPrepared(Id MsgId, err chan error) {
	cert := n.getCert(Id)
	// check if the preprepared msg and prepareQc has been received
	if cert.reqBatchStage == received && cert.fastPrepareQc != nil {
		cert.reqBatchStage = committed
		fmt.Printf("sn:%d can be commit in fast path\n", Id.Sn)
		go n.executeRequest(Id, err)
		return
	}

	if cert.reqBatchStage == received && cert.prepareQc != nil {
		cert.reqBatchStage = prepared
		n.checkIfCommitted(Id, err)
	}

}

func (n *Node) createCommitMsg(pqc *PrepareQc) *CommitMsg {

	var CM = CommitMsg{
		Vote:      *pqc,
		ReplicaId: n.replicaId,
	}

	encodedBytes, err := encode(CM.Vote)
	if err != nil {
		panic(err)
		return nil
	}

	CM.PartialSig = sign.SignTSPartial(n.tsPriKey, encodedBytes)
	CM.TimeStamp = time.Now().UnixNano()
	return &CM
}

// handle the commitmsg
func (n *Node) handleCommitMsg(cmsg *CommitMsg, err chan error) {

	/*
		if !n.inWatermarks(uint32(cmsg.Vote.SN)){
			fmt.Printf("The sequence %d of CommitMsg is out of the watermark\n",cmsg.Vote.SN)
			return
		}
	*/
	fmt.Printf("node %d receive commit of sn:%d  from node %d\n", n.replicaId, cmsg.Vote.SN, cmsg.ReplicaId)

	if !n.activeView {
		err <- errors.New("node is in viewchange and ignores the commit msg")
		return
	}

	msg := cmsg
	Id := MsgId{
		msg.Vote.SN,
		msg.Vote.View,
	}
	// check if a corresponding prepreparemsg has been received before
	n.checkIfPrePrepared(Id)
	cert := n.getCert(Id)
	// check if a commitmsg with the same sn has been received before
	if _, ok := cert.commitStore[msg.ReplicaId]; ok {
		// has received, warn...
		fmt.Printf("node %d receive a same Commit of sn:%d  from node %d\n", n.replicaId, cmsg.Vote.SN, cmsg.ReplicaId)
		return
		//Todo send viewchange
	} else {
		cert.commitStore[msg.ReplicaId] = msg
		n.handleCommitVote(msg)
	}

	// check if 2/3 commit msgs received
	// !!! there may be a problem: only if a new commitmsg is received, will the 2/3 be triggered to check.
	// !!! will there be a situation: 2/3 has reached while no new commitmsg is received to trigger the check
	// n.checkIfCommitted(msg.SN, err)
}
func (n *Node) handleCommitVote(cm *CommitMsg) {
	Id := MsgId{
		cm.Vote.SN,
		cm.Vote.View,
	}
	cert := n.getCert(Id)

	_, ok := n.partialSigInCommit[Id]
	if !ok {
		n.partialSigInCommit[Id] = make(map[string]map[uint32][]byte)
	}

	VoteBytes, _ := dataHashByte(cm.Vote)

	VoteString := hex.EncodeToString(VoteBytes)

	_, ok = n.partialSigInCommit[Id][VoteString]

	if !ok {
		n.partialSigInCommit[Id][VoteString] = make(map[uint32][]byte)
	}

	n.partialSigInCommit[Id][VoteString][cm.ReplicaId] = cm.PartialSig
	if len(n.partialSigInCommit[Id][VoteString]) >= 2*n.f+1 && !cert.commitQcStage {

		fmt.Printf("node %d send commitQc of sn:%d\n", n.replicaId, Id.Sn)
		commitQcMsg := n.createCommitQc(Id, cm.Vote)
		cert.commitQcStage = true
		n.broadcast(CommitQcType, commitQcMsg, nil, commitQcMsg.SN)
	}
}

func (n *Node) createCommitQc(Id MsgId, vote PrepareQc) *CommitQc {

	VoteBytes, _ := dataHashByte(vote)
	VoteString := hex.EncodeToString(VoteBytes)

	voteMsg := vote

	var partialSigs [][]byte
	for _, sig := range n.partialSigInCommit[Id][VoteString] {
		partialSigs = append(partialSigs, sig)
	}

	commitQc := CommitQc{
		ReplicaId:    n.replicaId,
		SN:           Id.Sn,
		View:         Id.View,
		BatchHash:    voteMsg.BatchHash,
		ThresholdSig: nil,
	}

	Sig, err := n.createThresholdSig(voteMsg, partialSigs, n.tsPubKey, 2*n.f+1, n.replicaCount)
	if err != nil {
		panic(err)
		return nil
	}

	commitQc.ThresholdSig = Sig
	commitQc.TimeStamp = time.Now().UnixNano()
	return &commitQc
}

//notice:check if received prepareQc and Prepare
func (n *Node) handleFastPrepareQc(fpqc *FastPrepareQc, err chan error) {

	if fpqc.ReplicaId != n.primary(n.currenView) {
		fmt.Println("fastPrepareQc from a non-leader node")
		//send view change
		return
	}

	Id := MsgId{
		fpqc.SN,
		fpqc.View,
	}
	cert := n.getCert(Id)

	if cert.fastPrepareQc != nil {
		fmt.Printf("Already has a fastPrepareQc of the same sn %d for this Id!", fpqc.SN)
		return
	}

	voteMsg := VoteForPrepare{
		SN:        fpqc.SN,
		View:      fpqc.View,
		BatchHash: fpqc.BatchHash,
	}

	ok, _ := n.verifyThresholdSig(voteMsg, n.fastPubKey, fpqc.ThresholdSig)
	if !ok {
		fmt.Printf("node %d receive invalid fastPrepareQc of sn:%d from node %d\n", n.replicaId, fpqc.SN, fpqc.ReplicaId)
		return
	}

	cert.fastPrepareQc = fpqc
	//check if prepared
	go n.checkIfPrepared(Id, err)

}
func (n *Node) handleCommitQc(cqc *CommitQc, err chan error) {

	/*
		if !n.inWatermarks(uint32(cqc.SN)){
			fmt.Printf("The sequence %d of commitQc is out of the watermark\n",cqc.SN)
			return
		}
	*/
	fmt.Printf("node %d receive commitQc of sn:%d from node %d\n", n.replicaId, cqc.SN, cqc.ReplicaId)

	if cqc.ReplicaId != n.primary(n.currenView) {
		fmt.Println("commitQc from a non-leader node")
		//send view change
		return
	}

	Id := MsgId{
		cqc.SN,
		cqc.View,
	}
	cert := n.getCert(Id)

	if cert.commitQc != nil {
		fmt.Printf("Already has a commitQc of the same sn %d for this Id!", cqc.SN)
		return
	}

	cert.commitQc = cqc

	voteMsg := cert.prepareQc

	//先收到commitQc，后收到PrepareQc,在收到PrepareQc处做判断再执行
	if voteMsg == nil {
		fmt.Println("Node hasn't receive prepareQc!")
		return
	}

	ok, _ := n.verifyThresholdSig(voteMsg, n.tsPubKey, cqc.ThresholdSig)

	if !ok {
		fmt.Println("Invalid commitQc!!!!")
		//sen the view change
		return
	}
	//prepared and receive commitQc,then execute
	//else wait for the preprepare or prepareQc
	n.checkIfCommitted(Id, err)
}

// check if commitQc received
func (n *Node) checkIfCommitted(Id MsgId, err chan error) {
	// check if the preprepared msg has been received
	cert := n.getCert(Id)

	if cert.reqBatchStage == prepared && cert.commitQc != nil {

		voteMsg := cert.prepareQc
		cqc := cert.commitQc

		ok, _ := n.verifyThresholdSig(voteMsg, n.tsPubKey, cqc.ThresholdSig)

		if !ok {
			fmt.Println("Invalid commitQc!!!!")
			//Todo viewchange
			return
		}
		cert.reqBatchStage = committed
		fmt.Printf("sn:%d can be commit in slow path\n", Id.Sn)
		// execute the request and reply to the client
		go n.executeRequest(Id, err)
	}
}

// execute the request and reply to the client
// just simply print a log
// ======= need to implement it

func (n *Node) executeRequest(Id MsgId, err chan error) {
	//buffer the command
	n.execReqIdBuffer[Id.Sn] = Id
	n.execReqBuffer[Id.Sn] = n.getCert(Id).reqBatchDigest
	if n.exeSn.lastExec < Id.Sn {
		n.exeSn.num.Lock()
		for i := n.exeSn.lastExec + 1; n.execReqBuffer[i] != nil; i++ {
			cert := n.getCert(n.execReqIdBuffer[i])
			timePast := calTimeDuration(cert.prePrepareStore.TimeStamp)
			fmt.Printf(time.Now().Format("2006-01-02 15:04:05")+"The request with sn: %d has been executed at node %d!\n", n.exeSn.lastExec+1, n.replicaId)
			fmt.Printf("TimePast:sn:%d,millisecend:%d\n", n.exeSn.lastExec+1, timePast/1000000)
			n.exeSn.lastExec++
			n.checkIfCreateChkpt(n.exeSn.lastExec, err)
			/*
				if n.exeSn.lastExec==RequestSN(n.viewChangeSeqNumber) {
					n.sendViewChange(err)
				}
			*/
		}
		n.exeSn.num.Unlock()
	}
}

func (n *Node) checkIfCreateChkpt(sn RequestSN, err chan error) {

	if uint32(sn)%n.T == 0 {

		go n.checkpoint(sn, nil, err)

	}
}

func (n *Node) checkpoint(seqNo RequestSN, digest []byte, err chan error) {

	if (uint32(seqNo) % n.T) != 0 {
		fmt.Printf("Attempted to checkpoint a sequence number (%d) which is not a multiple of the checkpoint interval (%d)", seqNo, n.K)
		return
	}

	chkpt := &CheckpointMsg{
		SeqN:        seqNo,
		StateDigest: string(digest),
		ReplicaId:   n.replicaId,
	}

	//store the state of the checkpoint

	n.chkpts[uint32(seqNo)] = string(digest)

	if errBC := n.broadcast(CheckPointType, chkpt, nil, 0); errBC != nil {
		err <- errBC
	}
}

func (n *Node) recvCheckpoint(chkpt *CheckpointMsg, err chan error) {

	fmt.Printf("node %d receive ckpoint with sn: %d from node %d!\n", n.replicaId, chkpt.SeqN, chkpt.ReplicaId)

	if !n.inWatermarks(RequestSN(uint32(chkpt.SeqN))) {
		fmt.Printf("Checkpoint sequence number outside watermarks:: seqNo %d, low-mark %d\n", chkpt.SeqN, n.h)
		return
	}

	if n.checkpointStore[chkpt.SeqN] == nil {

		n.checkpointStore[chkpt.SeqN] = make(map[CheckpointMsg]bool)

	}
	n.checkpointStore[chkpt.SeqN][*chkpt] = true

	diffValues := make(map[string]struct{})
	diffValues[chkpt.StateDigest] = struct{}{}

	matching := 0

	for testChkpt := range n.checkpointStore[chkpt.SeqN] {

		if testChkpt.StateDigest == chkpt.StateDigest {
			matching++
		} else {
			if _, ok := diffValues[testChkpt.StateDigest]; !ok {
				diffValues[testChkpt.StateDigest] = struct{}{}
			}
		}
	}

	if count := len(diffValues); count > n.f+1 {
		fmt.Printf("Network unable to find stable certificate for seqNo %d (%d different values observed already)\n",
			chkpt.SeqN, count)
		return
	}

	if matching < int(math.Ceil(float64(2*(len(n.clusterAddr))/3.0)))+1 {
		fmt.Printf("mattch%d\n", matching)
		return
	}

	//2f+1 checkpoints with the same sn and digest
	//a stable checkpoint
	n.moveWatermarks(chkpt.SeqN)
}

func (n *Node) moveWatermarks(seqNo RequestSN) {
	//move the watermark
	n.h = uint32(seqNo)
	fmt.Printf("Node %d moves low-mark to %d ,high-mark is %d\n", n.replicaId, n.h, n.h+n.K)
}

func (n *Node) inWatermarks(seqNo RequestSN) bool {
	return (uint32(seqNo))-n.h > 0 && (uint32(seqNo))-n.h <= n.K
}

func (n *Node) primary(view uint32) uint32 {
	return view % uint32(n.replicaCount)
}

func (n *Node) updateViewChangeSeqNo() {
	if n.viewChangePeriod == 0 {
		return
	}
	// Ensure the view change always occurs at middle of checkpoint boundary
	n.viewChangeSeqNumber = uint32(n.reqSn.nextSN-1) + n.viewChangePeriod*n.K + n.T/2
	fmt.Printf("Node %d updating view change sequence number to %d\n", n.replicaId, n.viewChangeSeqNumber)
}
