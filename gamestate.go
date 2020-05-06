package main

import (
	"math/rand"
	"sync"
	"time"
)

type GameState struct {
	spec    *GameSpecification
	Deck    []uint16
	Players []*PlayerState
	mutex   *sync.Mutex
	Id      uint64
	rng     *rand.Rand
}

func CreateGameFromSpec(spec *GameSpecification) GameState {
	result := GameState{
		spec,
		make([]uint16, len(spec.Deck)),
		make([]*PlayerState, 0),
		&sync.Mutex{},
		uint64(0),
		rand.New(rand.NewSource(time.Now().UTC().UnixNano())),
	}

	for cardId, _ := range spec.Deck {
		result.Deck[cardId] = uint16(cardId)
	}
	return result
}

func (gs *GameState) AddPlayer(newPlayer *PlayerState) {
	gs.mutex.Lock()
	gs.Players = append(gs.Players, newPlayer)
	newPlayer.CurrentGame = gs
	gs.mutex.Unlock()
}

func (gs *GameState) RemovePlayer(player *PlayerState) {
	gs.mutex.Lock()
	for index, p := range gs.Players {
		if player.Id == p.Id {
			gs.Players[index] = gs.Players[len(gs.Players)-1]
			gs.Players = gs.Players[:len(gs.Players)-1]
			break
		}
	}
	gs.mutex.Unlock()
}

func (gs *GameState) BroadcastNotification(notify NotifyPlayerActionCommand) error {
	cmdLen := notify.CommandLength()
	if cmdLen > MaxNotifyPlayerActionCommandLength {
		return ErrInvalidLength
	}
	buffer, headerLen := WriteCommandHeader(CMD_NOTIFY_PLAYER_ACTION, uint16(cmdLen))
	SerialiseNotifyPlayerActionCommand(buffer[headerLen:], &notify, false)

	gs.mutex.Lock()
	var err error = nil
	for _, player := range gs.Players {
		if (player.Id == notify.playerId) || (player.Id == notify.targetPlayerId) {
			continue
		}

		err = player.SendCommandBuffer(buffer)
	}
	gs.mutex.Unlock()
	return err
}

func (gs *GameState) SendNotificationToSourcePlayer(notify NotifyPlayerActionCommand) error {
	return gs.SendNotificationToPlayer(notify, notify.playerId)
}

func (gs *GameState) SendNotificationToTargetPlayer(notify NotifyPlayerActionCommand) error {
	return gs.SendNotificationToPlayer(notify, notify.targetPlayerId)
}

func (gs *GameState) SendNotificationToPlayer(notify NotifyPlayerActionCommand, destinationPlayerId uint64) error {
	cmdLen := notify.CommandLength()
	if cmdLen > MaxNotifyPlayerActionCommandLength {
		return ErrInvalidLength
	}
	buffer, headerLen := WriteCommandHeader(CMD_NOTIFY_PLAYER_ACTION, uint16(cmdLen))
	SerialiseNotifyPlayerActionCommand(buffer[headerLen:], &notify, false)

	var err error = nil
	playerFound := false
	for _, player := range gs.Players {
		if player.Id != destinationPlayerId {
			continue
		}

		err = player.SendCommandBuffer(buffer)
		playerFound = true
		break
	}

	if (err == nil) && (!playerFound) {
		return ErrInvalidPlayerId
	}
	return err
}

func (gs *GameState) Draw(deckId uint16, count int) []uint16 {
	gs.mutex.Lock()
	if len(gs.Deck) < count {
		count = len(gs.Deck)
	}

	result := make([]uint16, count)
	for i := 0; i < count; i++ {
		result[i] = gs.Deck[len(gs.Deck)-i-1]
	}
	gs.Deck = gs.Deck[:len(gs.Deck)-count]
	gs.mutex.Unlock()
	return result
}

func (gs *GameState) ShuffleDeck() {
	gs.rng.Shuffle(len(gs.Deck), func(i, j int) {
		gs.Deck[i], gs.Deck[j] = gs.Deck[j], gs.Deck[i]
	})
}

func (gs *GameState) FindDeck(deckId uint16) int {
	return 0
}

func (gs *GameState) FindPlayer(playerId uint64) int {
	if playerId == PLAYER_ID_ANY {
		return gs.rng.Intn(len(gs.Players))
	}

	for index, player := range gs.Players {
		if player.Id == playerId {
			return index
		}
	}
	return -1
}

func (gs *GameState) FindCard(player *PlayerState, cardId uint16) int {
	if player == nil {
		return -1
	}

	if cardId == CARD_ID_ANY {
		return gs.rng.Intn(len(player.Hand))
	}

	for index, cid := range player.Hand {
		if cid == cardId {
			return index
		}
	}
	return -1
}
