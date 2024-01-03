package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"go.dedis.ch/kyber/v3/share"
	"log"
	"time"
)

type PrePrepareMsg struct {
	SN        RequestSN
	BatchHash []byte
	ReqBatch  RequestBatch
	View      uint32
	ReplicaId uint32
	TimeStamp int64
}

type VoteForPrepare struct {
	SN        RequestSN
	BatchHash []byte
	View      uint32
}

type PrepareQc struct {
	SN           RequestSN
	BatchHash    []byte
	View         uint32
	ReplicaId    uint32
	ThresholdSig []byte
	TimeStamp    int64
}

type FastPrepareQc struct {
	SN           RequestSN
	BatchHash    []byte
	View         uint32
	ReplicaId    uint32
	ThresholdSig []byte
	TimeStamp    int64
}

type PrepareMsg struct {
	Vote           VoteForPrepare
	ReplicaId      uint32
	PartialSig     []byte //partialsig
	FastPartialSig []byte //fastPath partialsig
	TimeStamp      int64
}

type CommitQc struct {
	SN           RequestSN
	BatchHash    []byte
	View         uint32
	ReplicaId    uint32
	ThresholdSig []byte
	TimeStamp    int64
}

type CommitMsg struct {
	Vote       PrepareQc
	ReplicaId  uint32
	PartialSig []byte
	TimeStamp  int64
}

type CheckpointMsg struct {
	SeqN        RequestSN
	StateDigest string
	ReplicaId   uint32
}

// the index of every viewchangeMsg
type Vcidx struct {
	View      uint32
	ReplicaId uint32
}

type ViewChangeMsg struct {
	View         uint32
	LastStableCk uint32
	Cset         []CheckpointMsg //store checkpoint
	Pset         map[RequestSN]*PrePrepareMsg
	Qset         map[RequestSN]*PrepareQc
	ReplicaId    uint32
}

type NewViewMsg struct {
	View      uint32
	Vset      []ViewChangeMsg
	Oset      map[RequestSN]RequestBatch //store new pre_prepare_cmd
	ReplicaId uint32
}

type ViewChangeQuorumMsg struct{}

type ViewChangeTimerEvent struct{}

const (
	CreateEscrow = iota
	ProvideResource
	CompleteTrading
)

// InternalTX defines the intra-chain transaction struct
type InternalTX struct {
	TxID      []byte
	TradingID string
	From      Account
	To        Account
	Amount    int64
	Timestamp uint64
	Flag      int
}

func CreateNewITX(tradingID string, from, to Account, amount int64, flag int) *InternalTX {
	it := InternalTX{TradingID: tradingID, From: from, To: to, Amount: amount, Flag: flag}
	it.Timestamp = uint64(time.Now().Unix())
	it.TxID = it.Hash()
	return &it
}

func (it *InternalTX) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(*it)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

func (it *InternalTX) Hash() []byte {
	var hash [32]byte

	txCopy := *it
	txCopy.TxID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}

func (it *InternalTX) SetTradingID(id string) {
	it.TradingID = id
}

// CrossChainTX defines the cross-chain transaction struct
type CrossChainTX struct {
	CTxID        []byte
	TradingID    string
	ThresholdSig []byte
	TSPubKey     *share.PubPoly
	ProofID      []byte
	Timestamp    uint64
	Flag         int
}

func CreateNewCTX(tradingID string, thresholdSig, txID []byte, pubKey *share.PubPoly, flag int) *CrossChainTX {
	ct := CrossChainTX{TradingID: tradingID, ThresholdSig: thresholdSig, TSPubKey: pubKey, ProofID: txID, Flag: flag}
	ct.Timestamp = uint64(time.Now().Unix())
	ct.CTxID = ct.Hash()
	return &ct
}

func (ct *CrossChainTX) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(*ct)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

func (ct *CrossChainTX) Hash() []byte {
	var hash [32]byte

	txCopy := *ct
	txCopy.CTxID = []byte{}

	hash = sha256.Sum256(txCopy.Serialize())

	return hash[:]
}
