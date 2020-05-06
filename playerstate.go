package main

import (
	"net"
)

type PlayerState struct {
	Id          uint64
	Conn        net.Conn
	Name        string
	Hand        []uint16
	CurrentGame *GameState
}

func NewPlayerState(id uint64, name string, currentGame *GameState) PlayerState {
	return PlayerState{
		id,
		nil,
		name,
		make([]uint16, 0),
		currentGame,
	}
}

func (ps *PlayerState) InGame() bool {
	return ps.CurrentGame != nil
}

func (ps *PlayerState) SendCommandBuffer(buffer []byte) error {
	return SendCommandBufferTo(ps.Conn, buffer)
}

func (ps *PlayerState) Draw(cardId uint16) {
	ps.Hand = append(ps.Hand, cardId)
}

func (ps *PlayerState) Discard(cardIndex int) {
	ps.Hand[cardIndex] = ps.Hand[len(ps.Hand)-1]
	ps.Hand = ps.Hand[:len(ps.Hand)-1]
}
