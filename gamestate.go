package main

import (
	"math/rand"
	"sync"
)

type GameState struct {
	Deck    []string
	Players []*PlayerState
	mutex   *sync.Mutex
	Id      uint64
}

func CreateGameFromSpec(spec *GameSpecification) GameState {
	result := GameState{
		make([]string, 0),
		make([]*PlayerState, 0),
		&sync.Mutex{},
		uint64(0),
	}

	for _, card := range spec.Deck {
		result.Deck = append(result.Deck, card)
	}

	return result
}

func (gs *GameState) RemovePlayer(player *PlayerState) {
	for index, p := range gs.Players {
		if player.Id == p.Id {
			gs.Players[index] = gs.Players[len(gs.Players)-1]
			gs.Players = gs.Players[:len(gs.Players)-1]
			break
		}
	}
}

func (gs *GameState) Broadcast(msg string) {
	for _, player := range gs.Players {
		player.SendMessage(msg)
	}
}

func (gs *GameState) Draw() string {
	result := ""
	if len(gs.Deck) > 0 {
		result = gs.Deck[len(gs.Deck)-1]
		gs.Deck = gs.Deck[:len(gs.Deck)-1]
	}
	return result
}

func (gs *GameState) ShuffleDeck() {
	rand.Shuffle(len(gs.Deck), func(i, j int) {
		gs.Deck[i], gs.Deck[j] = gs.Deck[j], gs.Deck[i]
	})
}
