package core

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

//
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

//the index of every viewchangeMsg
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
