package core

import (
	"crypto/sha256"
	"github.com/treble-h/trebiz/sign"
)

const (
	Escrowed = iota
	Provided
	Completed
)

type Account interface {
	GetBalance() int64
	GetAddress() []byte
	SetBalance(balance int64)
	UpdateBalance(b int64)
}

// EscrowAccount defines an escrow account that temporarily stores transfer funds
type EscrowAccount struct {
	Address      []byte
	Balance      int64
	TradingState int
}

func NewEscrowAccount() *EscrowAccount {
	_, pubKey, _ := sign.GenKeys()
	msgHash := sha256.New()
	msgHash.Write(pubKey)
	address := msgHash.Sum(nil)
	return &EscrowAccount{
		Address:      address,
		Balance:      0,
		TradingState: Escrowed,
	}
}

func (e *EscrowAccount) GetAddress() []byte {
	return e.Address
}

func (e *EscrowAccount) GetBalance() int64 {
	return e.Balance
}

func (e *EscrowAccount) SetBalance(balance int64) {
	e.Balance = balance
}

func (e *EscrowAccount) UpdateBalance(b int64) {
	e.Balance += b
}

func (e *EscrowAccount) GetState() int {
	return e.TradingState
}

func (e *EscrowAccount) escrowed() {
	e.TradingState = Escrowed
}

func (e *EscrowAccount) provided() {
	e.TradingState = Provided
}

func (e *EscrowAccount) completed() {
	e.TradingState = Completed
}

func (e *EscrowAccount) ChangeTradingState() (int, int) {
	var before, after int
	switch e.TradingState {
	case Escrowed:
		before = Escrowed
		e.provided()
		after = Provided
	case Provided:
		before = Provided
		e.completed()
		after = Completed
	case Completed:
		before, after = Completed, Completed
	}
	return before, after
}

// NormalAccount defines a normal sender and receiver account
type NormalAccount struct {
	Address []byte
	Balance int64
}

func NewNormalAccount() *NormalAccount {
	_, pubKey, _ := sign.GenKeys()
	msgHash := sha256.New()
	msgHash.Write(pubKey)
	address := msgHash.Sum(nil)
	return &NormalAccount{
		Address: address,
		Balance: 100000000,
	}
}

func (n *NormalAccount) GetAddress() []byte {
	return n.Address
}

func (n *NormalAccount) GetBalance() int64 {
	return n.Balance
}

func (n *NormalAccount) SetBalance(balance int64) {
	n.Balance = balance
}

func (n *NormalAccount) UpdateBalance(b int64) {
	n.Balance += b
}
