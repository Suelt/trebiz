package core

import "sync"

// WorldState defines the in-memory state storage
type WorldState struct {
	// Escrow账户不会存储在底层数据库中
	StateMap map[string]Account
	DirtyMap map[string]struct{}
	TrieDB   *TrieDB
	MuLock   sync.RWMutex
}

// NewState creates a new state from a given state root
func NewState(file string, rootHash []byte) (*WorldState, error) {
	trieDB, err := NewTrieDB(file, rootHash)
	if err != nil {
		return nil, err
	}

	return &WorldState{
		StateMap: make(map[string]Account),
		DirtyMap: make(map[string]struct{}),
		TrieDB:   trieDB,
	}, nil
}

func (w *WorldState) InsertNormalAccount() {
	w.MuLock.Lock()
	defer w.MuLock.Unlock()
	account := NewNormalAccount()
	address := account.GetAddress()
	id := string(address[:16])
	w.StateMap[id] = account
}

func (w *WorldState) InsertEscrowAccounts(itxs []*InternalTX) {
	w.MuLock.Lock()
	defer w.MuLock.Unlock()
	for _, itx := range itxs {
		if len(itx.TradingID) == 0 {
			// 用初始交易编号作为trading ID
			itx.SetTradingID(string(itx.TxID))
		}
		account := NewEscrowAccount()
		w.StateMap[itx.TradingID] = account
	}
}

func (w *WorldState) UpdateTradingState(itx *InternalTX) (int, int) {
	w.MuLock.Lock()
	defer w.MuLock.Unlock()
	var before, after int
	acc := w.StateMap[itx.TradingID]
	if escrow, ok := acc.(*EscrowAccount); ok {
		before, after = escrow.ChangeTradingState()
	}
	return before, after
}

func (w *WorldState) GetAccount(key string) Account {
	w.MuLock.RLock()
	defer w.MuLock.RUnlock()
	if acc, ok := w.StateMap[key]; ok {
		return acc
	}
	return nil
}

func (w *WorldState) Clear(itxs []*InternalTX) {
	w.MuLock.Lock()
	defer w.MuLock.Unlock()
	for _, itx := range itxs {
		acc := w.StateMap[itx.TradingID]
		if escrow, ok := acc.(*EscrowAccount); ok {
			state := escrow.GetState()
			if state == Completed {
				delete(w.StateMap, itx.TradingID)
			}
		}
	}
}

// Commit commits all dirty (updated) account data into the underlying database
func (w *WorldState) Commit() error {
	w.MuLock.Lock()
	defer w.MuLock.Unlock()
	for addr := range w.DirtyMap {
		acc := w.StateMap[addr]
		if normalAcc, ok := acc.(*NormalAccount); ok {
			if err := w.TrieDB.StoreState(normalAcc); err != nil {
				return err
			}
			delete(w.StateMap, addr)
		}
	}
	w.TrieDB.Commit()
	w.DirtyMap = make(map[string]struct{})
	return nil
}
