package core

import (
	"encoding/hex"
	"fmt"
	"time"
)

func (n *Node) sendViewChange(err chan error) {

	n.stopTimer()

	n.currenView++
	n.activeView = false

	vc := &ViewChangeMsg{
		View:         n.currenView,
		LastStableCk: n.h,
		ReplicaId:    n.replicaId,
	}

	//preprepareMsg and prepareQc
	vc.Pset, vc.Qset = n.calPQset()

	//2f+1 checkpoint to verify the LastStableCk
	count := 0

	for ChkptfromNodes := range n.checkpointStore[RequestSN(n.h)] {
		//verify if the stateDigests are consistent
		if ChkptfromNodes.StateDigest == n.chkpts[n.h] {
			vc.Cset = append(vc.Cset, ChkptfromNodes)
			count++
			if count == 2*n.f+1 {
				break
			}
		}
	}

	n.viewChangeStage[n.currenView] = true
	fmt.Printf("Node %d sending view-change, v:%d, h:%d, |C|:%d, |P|:%d, |Q|:%d\n",
		n.replicaId, vc.View, vc.LastStableCk, len(vc.Cset), len(vc.Pset), len(vc.Qset))

	if errBC := n.broadcast(ViewChangeType, vc, nil, 0); errBC != nil {
		err <- errBC
	}
	n.startTimer(n.LastNewVewTimeout, "new view change")
	n.LastNewVewTimeout = n.LastNewVewTimeout * 2
}

//store preprepare
func (n *Node) calPQset() (map[RequestSN]*PrePrepareMsg, map[RequestSN]*PrepareQc) {

	pset := make(map[RequestSN]*PrePrepareMsg)

	qset := make(map[RequestSN]*PrepareQc)

	for idx, cert := range n.reqCertStore {

		if idx.Sn <= RequestSN(n.h) {
			continue
		}

		if cert.prepareQc != nil {
			pset[idx.Sn] = cert.prePrepareStore
			//qset for Qc
			qset[idx.Sn] = cert.prepareQc
		}
	}
	return pset, qset
}

//prepared at with a sequence number higher than . Each
//set contains a valid pre-Prepare message (withoutthe
//corresponding client message) and 2f
//+1 matching, valid
//Prepare messages signed by different backups with the
//same view, sequence number, and the digest of .
func (n *Node) correctViewChange(vc *ViewChangeMsg) bool {

	//verify preprepare
	for _, p := range vc.Pset {
		if !(p.View < vc.View && p.SN > RequestSN(vc.LastStableCk) && p.SN < RequestSN(vc.LastStableCk+n.K)) {
			fmt.Printf("Node %d receive invalid p entry in view-change: vc(v:%d h:%d) p(v:%d n:%d)\n",
				n.replicaId, vc.View, vc.LastStableCk, p.View, p.SN)
			return false
		}
	}

	//todo: verify sig

	for _, pqc := range vc.Qset {

		voteMsg := VoteForPrepare{
			SN:        pqc.SN,
			View:      pqc.View,
			BatchHash: pqc.BatchHash,
		}

		ok, _ := n.verifyThresholdSig(voteMsg, n.tsPubKey, pqc.ThresholdSig)
		if !ok {
			fmt.Println("invalid prepareQc")
			return false
		}
	}

	// PBFT: the paper says c.n > =vc.h???

	for _, c := range vc.Cset {

		if !(c.SeqN == RequestSN(vc.LastStableCk)) {
			fmt.Printf("Node %d :invalid c entry in view-change: vc(v:%d h:%d) c(n:%d)\n",
				n.replicaId, vc.View, vc.LastStableCk, c.SeqN)
			return false
		}
	}

	return true
}

func (n *Node) recvViewChange(vc *ViewChangeMsg, err chan error) {

	fmt.Printf("Node %d received view-change from node %d, v:%d, h:%d, |C|:%d, |P|:%d, |Q|:%d\n",
		n.replicaId, vc.ReplicaId, vc.View, vc.LastStableCk, len(vc.Cset), len(vc.Pset), len(vc.Qset))

	if vc.View < n.currenView {
		fmt.Printf("Node %d found view-change message for old view\n", n.replicaId)
		return
	}

	if !n.correctViewChange(vc) {
		fmt.Printf("Node %d found view-change message incorrect\n", n.replicaId)
		return
	}

	if _, ok := n.viewChangeStore[Vcidx{vc.View, vc.ReplicaId}]; ok {
		fmt.Printf("Node %d already has a view change message for view %d from node %d\n", n.replicaId, vc.View, vc.ReplicaId)
		return
	}

	n.viewChangeStore[Vcidx{View: vc.View, ReplicaId: vc.ReplicaId}] = vc

	//"if a replica receives a set of
	// f+1 valid VIEW-CHANGE messages from other replicas for
	// views greater than its current view, it sends a VIEW-CHANGE
	// message for the smallest view in the set, even if its timer
	// has not expired"
	replicas := make(map[uint32]bool)
	minView := uint32(0)
	for idx := range n.viewChangeStore {
		if idx.View <= n.currenView {
			continue
		}
		replicas[idx.ReplicaId] = true
		if minView == 0 || idx.View < minView {
			minView = idx.View
		}
	}

	// We only enter this if there are enough view change messages _greater_ than our current view
	if len(replicas) >= n.f+1 {
		fmt.Printf("Node %d received f+1 view-change messages whose view is greater than current view, triggering view-change to view %d\n",
			n.replicaId, minView)
		// subtract one, because sendViewChange() increments
		n.currenView = minView - 1
		n.sendViewChange(err)
	}

	quorum := 0
	for idx := range n.viewChangeStore {
		if idx.View == n.currenView {
			quorum++
		}
	}

	if !n.activeView && vc.View == n.currenView && quorum >= n.viewChangeQuorum && n.primary(vc.View) == n.replicaId {

		fmt.Printf(
			"Node %d now has %d view change requests for view %d\n",
			n.replicaId,
			quorum,
			n.currenView,
		)
		//leader send new view
		if n.replicaId == n.primary(n.currenView) {
			n.sendNewView(err)
		}
	}
}

func (n *Node) sendNewView(err chan error) {

	if _, ok := n.newViewStore[n.currenView]; ok {
		fmt.Printf("Node %d already has new view in store for view %d, skipping\n", n.replicaId, n.currenView)
		return
	}

	vset := n.getViewChangeMsgs()

	//min-s  =  the sequence number of the latest stable checkpoint in ƴ
	initialS, ok := n.selectInitialSequence(vset)

	if initialS == 0 || !ok {
		fmt.Printf("Node %d could not find consistent checkpoint in vset: %+v\n", n.replicaId, vset)
		return
	}

	msgList, ok := n.assignSequenceNumbers(vset, initialS)

	if msgList == nil || ok == false {
		fmt.Printf("Node %d could not assign sequence numbers for new view\n", n.replicaId)
		return
	}

	nv := &NewViewMsg{
		View:      n.currenView,
		Vset:      vset,
		Oset:      msgList,
		ReplicaId: n.replicaId,
	}

	fmt.Printf("Node %d send new view for view %d\n", n.replicaId, nv.View)

	excAddrs := make(map[uint32]bool)
	excAddrs[n.replicaId] = true

	if n.newViewStage[n.currenView] {
		fmt.Printf("Node %d has send newView for view %d\n", n.replicaId, n.currenView)
		return
	}

	if errBC := n.broadcast(NewViewType, nv, excAddrs, 0); errBC != nil {
		err <- errBC
	}

	n.newViewStore[n.currenView] = nv
	n.newViewStage[n.currenView] = true

	n.processNewView(err)

}

func (n *Node) recvNewView(nv *NewViewMsg, err chan error) {

	fmt.Printf("Node %d received new-view from node %d\n", n.replicaId, nv.View)

	if !(nv.View > 0 && nv.View >= n.currenView && n.primary(nv.View) == nv.ReplicaId && n.newViewStore[nv.View] == nil) {
		fmt.Printf("Node %d rejecting invalid new-view from node %d, v:%d\n",
			n.replicaId, nv.ReplicaId, nv.View)
		return
	}

	//todo verify viewchange in new view

	n.newViewStore[nv.View] = nv
	n.processNewView(err)
}

func (n *Node) processNewView(err chan error) {

	nv, ok := n.newViewStore[n.currenView]
	if !ok {
		fmt.Printf("Node %d ignoring processNewView as it could not find view %d in its newViewStore", n.replicaId, n.currenView)
		return
	}

	if n.activeView {
		fmt.Printf("Node %d ignoring processNewView as it is in active view.", n.replicaId)
		return
	}

	//Todo verify new view

	n.stopTimer()

	n.activeView = true
	//update the nextSN
	n.reqSn.nextSN = RequestSN(n.h + n.K + 1)
	fmt.Printf("Node %d update nextSn to %d\n", n.replicaId, n.reqSn.nextSN)
	n.updateViewChangeSeqNo()
	keySort := GetMapkey(nv.Oset)

	for _, sn := range keySort {

		if sn <= RequestSN(n.h) {
			continue
		}
		reqBatch := nv.Oset[sn]

		batchHash, herr := dataHashByte(reqBatch)
		if herr != nil {
			fmt.Println("hash reqbatch err!")
		}

		//receive preprepare
		prePrepare := &PrePrepareMsg{
			SN:        sn,
			BatchHash: batchHash,
			ReqBatch:  reqBatch,
			View:      n.currenView,
			ReplicaId: nv.ReplicaId, //new leader Id
		}

		if n.replicaId == n.primary(n.currenView) {
			Id := MsgId{sn, n.currenView}
			BatchString := hex.EncodeToString(prePrepare.BatchHash)
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
		}

		prePrepare.TimeStamp = time.Now().UnixNano()
		n.handlePrePrepareMsg(prePrepare, nil)

	}

}

func (n *Node) getViewChangeMsgs() (vset []ViewChangeMsg) {

	for _, vc := range n.viewChangeStore {
		if vc.View == n.currenView {
			vset = append(vset, *vc)
		}
	}

	return
}

func (n *Node) selectInitialSequence(vset []ViewChangeMsg) (sn uint32, ok bool) {

	//find the latest stable checkpoint sequence
	sn = 0

	for _, vc := range vset {
		if sn <= vc.LastStableCk {
			sn = vc.LastStableCk
			ok = true
		}
	}

	return
}

func (n *Node) assignSequenceNumbers(vset []ViewChangeMsg, sn uint32) (msgList map[RequestSN]RequestBatch, err bool) {

	msgList = make(map[RequestSN]RequestBatch)
	var ok = true
	min_s := sn + 1
	max_s := sn + n.K

	for s := min_s; s <= max_s; s++ {
		diffValues := make(map[string]int)
		diffValuesReqBatch := make(map[string]RequestBatch)
		for _, vc := range vset {
			//if has pre prepare
			if _, ok := vc.Pset[RequestSN(s)]; ok {
				BatchHashString := string(vc.Pset[RequestSN(s)].BatchHash)
				diffValues[BatchHashString]++
				diffValuesReqBatch[BatchHashString] = vc.Pset[RequestSN(s)].ReqBatch
				//if find a valid prepareQC
				if _, ok := vc.Qset[RequestSN(s)]; ok {
					cmd := vc.Pset[RequestSN(s)].ReqBatch
					msgList[RequestSN(s)] = cmd
					break
				}
			}
		}

		DiffSubet := 0
		for _, value := range diffValues {
			if value >= n.prePrepareSubsetCount {
				DiffSubet++
			}
		}

		if DiffSubet == 1 {
			for key, value := range diffValues {
				if value >= n.prePrepareSubsetCount {
					msgList[RequestSN(s)] = diffValuesReqBatch[key]
				}
			}
		}

		if DiffSubet == 2 {
			ok = false
		}

		//null request
		if DiffSubet == 0 {
			msgList[RequestSN(s)] = RequestBatch{}
		}

	}
	return msgList, ok
}
