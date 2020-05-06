package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type ServerState struct {
	mutex        *sync.Mutex
	nextPlayerId uint64
	nextGameId   uint64
	allPlayers   []*PlayerState
	allGames     []*GameState
}

func (ss *ServerState) AddPlayer(socket net.Conn, name string) *PlayerState {
	ss.mutex.Lock()
	playerId := ss.nextPlayerId
	ss.nextPlayerId += 1

	ps := PlayerState{
		playerId,
		socket,
		name,
		make([]uint16, 0),
		nil,
	}
	ss.allPlayers = append(ss.allPlayers, &ps)
	ss.mutex.Unlock()
	return &ps
}

func (ss *ServerState) RemovePlayer(playerId uint64) {
	var player *PlayerState = nil
	ss.mutex.Lock()
	for index, p := range ss.allPlayers {
		if p.Id == playerId {
			player = p
			ss.allPlayers[index] = ss.allPlayers[len(ss.allPlayers)-1]
			ss.allPlayers = ss.allPlayers[:len(ss.allPlayers)-1]
			break
		}
	}
	ss.mutex.Unlock()

	if player != nil {
		player.Conn.Close()
		if player.CurrentGame != nil {
			notifyAction := NewPlayerActionNotify(player.Id, CMD_GAME_LEAVE, DECK_ID_NONE, PLAYER_ID_NONE, nil)
			player.CurrentGame.BroadcastNotification(notifyAction)
			player.CurrentGame.RemovePlayer(player)
		}
	}
}

func (ss *ServerState) CreateNewGame(spec *GameSpecification, firstPlayer *PlayerState) *GameState {
	gs := CreateGameFromSpec(spec)
	gs.ShuffleDeck()

	gs.Players = append(gs.Players, firstPlayer)
	firstPlayer.CurrentGame = &gs

	ss.mutex.Lock()
	// Cleanup any old/empty games before creating the new one
	for i := 0; i < len(ss.allGames); i++ {
		game := ss.allGames[i]
		if len(game.Players) == 0 {
			game.mutex.Lock()
			shouldRemove := (len(game.Players) == 0)

			if shouldRemove {
				fmt.Println("Clean up game " + strconv.FormatUint(game.Id, 10))
				ss.allGames[i] = ss.allGames[len(ss.allGames)-1]
				ss.allGames = ss.allGames[:len(ss.allGames)-1]
			}
			game.mutex.Unlock()
		}
	}

	gs.Id = ss.nextGameId
	ss.nextGameId += 1
	ss.allGames = append(ss.allGames, &gs)
	ss.mutex.Unlock()
	fmt.Println("Created game", gs.Id)
	return &gs
}

func (ss *ServerState) FindGame(gameId uint64) *GameState {
	var result *GameState = nil
	ss.mutex.Lock()
	for _, game := range ss.allGames {
		if game.Id != gameId {
			continue
		}

		result = game
		break
	}
	ss.mutex.Unlock()
	return result
}

func (ss *ServerState) Shutdown() {
	notifyBuffer, _ := WriteCommandHeader(CMD_NOTIFY_SERVER_SHUTDOWN, 0)
	ss.mutex.Lock()
	for _, player := range ss.allPlayers {
		player.SendCommandBuffer(notifyBuffer)
		player.Conn.Close()
	}
	ss.allPlayers = ss.allPlayers[:0]
	ss.allGames = ss.allGames[:0]
	ss.mutex.Unlock()
}

func serverReadConsoleInput(cmdChan chan string) {
	fmt.Println("Reading input from stdin...")
	stdInRead := bufio.NewReader(os.Stdin)
	for {
		inputLine, err := stdInRead.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to read from stdin: ", err)
			return
		}

		cmdChan <- inputLine
	}
}

func serverListenForConnections(listener net.Listener, server *ServerState) {
	fmt.Println("Listening for new connections...")
	for {
		newConn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error while accepting connection: ", err)
			return
		}

		fmt.Printf("Received connection from %s\n", newConn.RemoteAddr().String())
		go runServerPlayer(server, newConn)
	}
}

func runServerPlayer(server *ServerState, playerConn net.Conn) {
	var player *PlayerState = nil
	playerName := "UNKNOWN - Awaiting handshake"

	for {
		headerBytes, err := ReadExactlyNBytes(playerConn, CommandHeaderLength)
		if err != nil {
			fmt.Printf("Error: Failed to read command header from player '%s': %s\n", playerName, err)
			break
		}

		var cmdHeader CommandHeader
		err = SerialiseCommandHeader(headerBytes, &cmdHeader, true)
		if err != nil {
			fmt.Printf("Error: Failed to deserialise command header from player '%s': %s\n", playerName, err)
			break
		}

		err = ValidateCommandHeader(cmdHeader)
		if err != nil {
			fmt.Printf("ERROR: Invalid command header {id=%d,len=%d} received from player '%s': %s\n",
				cmdHeader.id, cmdHeader.len, playerName, err)
			break
		}

		cmdBuffer, err := ReadExactlyNBytes(playerConn, cmdHeader.len)
		if err != nil {
			fmt.Printf("ERROR: Failed to read command buffer of length %d for command %d from player '%s': %s\n",
				cmdHeader.len, cmdHeader.id, playerName, err)
			break
		}

		wantsToCloseConnection := false
		if player == nil {
			if cmdHeader.id != CMD_HANDSHAKE {
				fmt.Printf("Connection from %s sent command ID %d first instead of a handshake, disconnecting...\n", playerConn.RemoteAddr().String(), cmdHeader.id)
				break
			} else {
				var cmd HandshakeCommand
				err = SerialiseHandshakeCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error! Failed to deserialise handshake command %+v: %s\n", cmdBuffer, err)
					break
				}

				if (cmd.magicNumber != PROTOCOL_MAGIC_NUMBER) || (cmd.protocolId != PROTOCOL_ID) {
					fmt.Printf("Connection from %s sent invalid handshake, disconnecting...\n", playerConn.RemoteAddr().String())
					break
				} else {
					playerName = cmd.localName
					player = server.AddPlayer(playerConn, playerName)
					if player == nil {
						fmt.Println("Failed to add new player to the server. The server is full")
						sendInputErrorTo(playerConn, cmdHeader.id, ERROR_SERVER_FULL)
						break
					}

					if (len(playerName) > MaxPlayerNameLength) || (strings.ContainsAny(playerName, " \t\n\r")) {
						fmt.Printf("Player attempted to join with invalid name '%s'. Rejecting...\n", playerName)
						sendInputError(player, cmdHeader.id, ERROR_INVALID_PLAYER_NAME)
						break
					}
					fmt.Printf("Received player name from %s - %s\n", playerConn.RemoteAddr(), playerName)

					response := HandshakeResponseCommand{
						player.Id,
					}
					respBuffer, respHeaderLen := WriteCommandHeader(CMD_HANDSHAKE_RESPONSE, HandshakeResponseCommandLength)
					err = SerialiseHandshakeResponseCommand(respBuffer[respHeaderLen:], &response, false)
					if err != nil {
						fmt.Printf("Error! Failed to serialise handshake command %+v: %s\n", response, err)
						break
					}
					err = player.SendCommandBuffer(respBuffer)
					if err != nil {
						fmt.Printf("Error! Failed to send response for command %d to player %d: %s\n", cmdHeader.id, player.Id, err)
					}
				}
			}

		} else if player.InGame() {
			game := player.CurrentGame
			switch cmdHeader.id {
			case CMD_KEEPALIVE:
				// Do nothing

			case CMD_INFO_PLAYERS:
				fmt.Printf("Show player info\n")
				game.mutex.Lock()
				playerIds := make([]uint64, 0, len(game.Players))
				playerNames := make([]string, 0, len(game.Players))
				handSizes := make([]uint16, 0, len(game.Players))
				for _, p := range game.Players {
					playerIds = append(playerIds, p.Id)
					playerNames = append(playerNames, p.Name)
					handSizes = append(handSizes, uint16(len(p.Hand)))
				}
				game.mutex.Unlock()

				respCmd := PlayerInfoResponseCommand{
					playerIds,
					playerNames,
					handSizes,
				}
				respBuffer, respHeaderLen := WriteCommandHeader(CMD_INFO_PLAYERS_RESPONSE, uint16(respCmd.CommandLength()))
				err = SerialisePlayerInfoResponseCommand(respBuffer[respHeaderLen:], &respCmd, false)
				if err != nil {
					fmt.Printf("Error! Failed to serialise player info command %+v: %s\n", respCmd, err)
					break
				}
				fmt.Printf("Send players response: %+v = %+v\n", respCmd, respBuffer)
				err = player.SendCommandBuffer(respBuffer)
				if err != nil {
					fmt.Printf("Error! Failed to send response for command %d to player %d: %s\n", cmdHeader.id, player.Id, err)
				}

			case CMD_INFO_DECKS:
				fmt.Printf("Show deck info\n")
				game.mutex.Lock()
				respCmd := DeckInfoResponseCommand{
					[]uint16{0},
					[]uint16{uint16(len(game.Deck))},
				}
				game.mutex.Unlock()
				respBuffer, respHeaderLen := WriteCommandHeader(CMD_INFO_DECKS_RESPONSE, uint16(respCmd.CommandLength()))
				err = SerialiseDeckInfoResponseCommand(respBuffer[respHeaderLen:], &respCmd, false)
				if err != nil {
					fmt.Printf("Error! Failed to serialise deck info command %+v: %s\n", respCmd, err)
					break
				}
				err = player.SendCommandBuffer(respBuffer)
				if err != nil {
					fmt.Printf("Error! Failed to send response for command %d to player %d: %s\n", cmdHeader.id, player.Id, err)
				}
				// TODO: Shows if the top card is face up, what it is? If face-up in a deck is a thing we support?

			case CMD_INFO_CARDS:
				fmt.Printf("Show card info\n")
				game.mutex.Lock()
				respCmd := CardInfoResponseCommand{
					player.Hand,
				}
				respBuffer, respHeaderLen := WriteCommandHeader(CMD_INFO_CARDS_RESPONSE, uint16(respCmd.CommandLength()))
				err = SerialiseCardInfoResponseCommand(respBuffer[respHeaderLen:], &respCmd, false)
				if err != nil {
					fmt.Printf("Error! Failed to serialise card info command %+v: %s\n", respCmd, err)
					break
				}
				game.mutex.Unlock()

				err = player.SendCommandBuffer(respBuffer)
				if err != nil {
					fmt.Printf("Error! Failed to send response for command %d to player %d: %s\n", cmdHeader.id, player.Id, err)
				}

			case CMD_CARD_DRAW:
				var cmd CardDrawCommand
				err := SerialiseCardDrawCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to serialise command body of command %d from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Draw %d cards from deck %d\n", cmd.count, cmd.deckId)

				newCards := game.Draw(cmd.deckId, int(cmd.count))
				game.mutex.Lock()
				for _, newCard := range newCards {
					player.Draw(newCard)
				}
				game.mutex.Unlock()

				publicNotifyCards := newCards
				if !cmd.faceUp {
					publicNotifyCards = makeFilledIdSlice(len(newCards), CARD_ID_ANY)
				}
				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, cmd.deckId, PLAYER_ID_NONE, publicNotifyCards)
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("ERROR: Failed to broadcast draw command notification to all players: %s\n", err)
				}

				notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, cmd.deckId, player.Id, newCards)
				err = game.SendNotificationToTargetPlayer(notifyAction)
				if err != nil {
					fmt.Printf("ERROR: Failed to broadcast draw command notification to the drawing player: %s\n", err)
				}

			case CMD_CARD_SHOW:
				var cmd CardShowCommand
				err := SerialiseCardShowCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s\n", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Show card %d to player %d\n", cmd.cardId, cmd.playerId)

				game.mutex.Lock()
				var cardIndex int
				cardIndex = game.FindCard(player, cmd.cardId)
				targetPlayerIndex := game.FindPlayer(cmd.playerId)
				var targetPlayer *PlayerState = nil
				if targetPlayerIndex >= 0 {
					targetPlayer = game.Players[targetPlayerIndex]
				}
				game.mutex.Unlock()

				if (cardIndex < 0) && (cmd.cardId != CARD_ID_ALL) {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_CARD_ID)
					break
				} else if (targetPlayerIndex < 0) && (cmd.playerId != PLAYER_ID_ANY) && (cmd.playerId != PLAYER_ID_ALL) {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_PLAYER_ID)
					break
				}

				var visibleCardSlice []uint16
				if cmd.cardId == CARD_ID_ALL {
					visibleCardSlice = player.Hand
				} else {
					visibleCardSlice = []uint16{player.Hand[cardIndex]}
				}

				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, cmd.playerId, visibleCardSlice)
				err = game.SendNotificationToSourcePlayer(notifyAction)
				if err != nil {
					fmt.Printf("Error! Failed to send card show notification to source player: %s\n", err)
				}

				if cmd.playerId == PLAYER_ID_ALL {
					err = game.BroadcastNotification(notifyAction)
					if err != nil {
						fmt.Printf("ERROR: Failed to broadcast card show notification: %s\n", err)
					}
				} else {
					hiddenCardSlice := makeFilledIdSlice(len(visibleCardSlice), CARD_ID_ANY)
					notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, targetPlayer.Id, hiddenCardSlice)
					err = game.BroadcastNotification(notifyAction)
					if err != nil {
						fmt.Printf("ERROR: Failed to broadcast anonymised card show notification: %s\n", err)
					}
				}

			case CMD_CARD_PUTBACK:
				var cmd CardPutbackCommand
				err := SerialiseCardPutbackCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s\n", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Received putback: %+v\n", cmd)
				fmt.Printf("Put card %d back onto deck %d, %d cards from the top\n", cmd.cardId, cmd.deckId, cmd.cardsFromTop)

				game.mutex.Lock()
				cardIndex := game.FindCard(player, cmd.cardId)
				deckIndex := game.FindDeck(cmd.deckId)
				if cardIndex < 0 {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_CARD_ID)
					break
				}
				if deckIndex < 0 {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_DECK_ID)
					break
				}
				if (cmd.cardsFromTop < 0) || (int(cmd.cardsFromTop) > len(game.Deck)) {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_DATA)
					break
				}

				cardIndexInDeck := len(game.Deck) - int(cmd.cardsFromTop)
				game.Deck = sliceInsert(game.Deck, player.Hand[cardIndex], cardIndexInDeck)
				player.Discard(cardIndex)
				game.mutex.Unlock()

				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, cmd.deckId, PLAYER_ID_NONE, []uint16{CARD_ID_ANY})
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("ERROR: Failed to broadcast card putback notification: %s\n", err)
				}
				notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, cmd.deckId, PLAYER_ID_NONE, []uint16{cmd.cardId})
				err = game.SendNotificationToSourcePlayer(notifyAction)
				if err != nil {
					fmt.Printf("ERROR: Failed to send card putback notification to %s: %s\n", player.Name, err)
				}

			case CMD_CARD_DISCARD:
				var cmd CardDiscardCommand
				err := SerialiseCardDiscardCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Discard card %d. Face up? %t\n", cmd.cardId, cmd.faceUp)

				game.mutex.Lock()
				cardIndex := game.FindCard(player, cmd.cardId)
				if cardIndex < 0 {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_CARD_ID)
					game.mutex.Unlock()
					break
				}
				player.Discard(cardIndex)
				game.mutex.Unlock()

				displayedCardId := cmd.cardId
				if !cmd.faceUp {
					displayedCardId = CARD_ID_ANY
				}
				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, PLAYER_ID_NONE, []uint16{displayedCardId})
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("Failed to broadcast card discard notification: %s\n", err)
				}
				notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, PLAYER_ID_NONE, []uint16{cmd.cardId})
				err = game.SendNotificationToTargetPlayer(notifyAction)
				if err != nil {
					fmt.Printf("Failed to send card discard notification: %s\n", err)
				}

			case CMD_CARD_GIVE:
				var cmd CardGiveCommand
				err := SerialiseCardGiveCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Give card %d to player %d. Visible to all players? %t\n", cmd.cardId, cmd.playerId, cmd.faceUp)

				if player.Id == cmd.playerId {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_PLAYER_ID)
					break
				}

				game.mutex.Lock()
				cardIndex := game.FindCard(player, cmd.cardId)
				playerIndex := game.FindPlayer(cmd.playerId)
				if cardIndex < 0 {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_CARD_ID)
					game.mutex.Unlock()
					break
				}
				if playerIndex < 0 {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_PLAYER_ID)
					game.mutex.Unlock()
					break
				}
				targetPlayer := game.Players[playerIndex]
				targetPlayer.Hand = append(targetPlayer.Hand, player.Hand[cardIndex])
				player.Discard(cardIndex)
				game.mutex.Unlock()

				displayedCardId := cmd.cardId
				if !cmd.faceUp {
					displayedCardId = CARD_ID_ANY
				}
				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, cmd.playerId, []uint16{cmd.cardId})
				err = game.SendNotificationToTargetPlayer(notifyAction)
				if err != nil {
					fmt.Printf("Error! Failed to send card give notification to source player: %s\n", err)
				}
				err = game.SendNotificationToSourcePlayer(notifyAction)
				if err != nil {
					fmt.Printf("Error! Failed to send card give notification to target player: %s\n", err)
				}
				notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, cmd.playerId, []uint16{displayedCardId})
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("Failed to broadcast card give notification: %s\n", err)
				}

			case CMD_DECK_PEEK:
				var cmd DeckPeekCommand
				err := SerialiseDeckPeekCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Peek at the top %d cards of deck %d. Public? %t\n", cmd.count, cmd.deckId, cmd.public)

				cardList := make([]uint16, 0, cmd.count)
				game.mutex.Lock()
				deckSlice := game.Deck
				if int(cmd.count) <= len(deckSlice) {
					deckSlice = deckSlice[len(deckSlice)-int(cmd.count):]
				}
				for i := len(deckSlice) - 1; i >= 0; i-- {
					cardList = append(cardList, deckSlice[i])
				}
				game.mutex.Unlock()

				var publicCardList []uint16
				if cmd.public {
					publicCardList = cardList
				} else {
					publicCardList = makeFilledIdSlice(len(cardList), CARD_ID_ANY)
				}
				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, PLAYER_ID_NONE, cardList)
				err = game.SendNotificationToSourcePlayer(notifyAction)
				if err != nil {
					fmt.Printf("ERROR: Failed send deck peek response notification to source player: %s\n", err)
				}
				notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, PLAYER_ID_NONE, publicCardList)
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("ERROR: Failed broadcast deck peek response notification: %s\n", err)
				}

			case CMD_DECK_SHUFFLE:
				var cmd DeckShuffleCommand
				err := SerialiseDeckShuffleCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Shuffle deck %d\n", cmd.deckId)
				game.ShuffleDeck()
				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, cmd.deckId, PLAYER_ID_NONE, nil)
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("Error while broadcasting notification of command %d: %s\n", cmdHeader.id, err)
				}

			case CMD_GAME_LEAVE:
				fmt.Println("Request to leave game")
				notifyAction := NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, PLAYER_ID_NONE, nil)
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("Error while broadcasting notification of command %d: %s\n", cmdHeader.id, err)
				}

				notifyAction = NewPlayerActionNotify(player.Id, cmdHeader.id, DECK_ID_NONE, player.Id, nil)
				err = game.SendNotificationToTargetPlayer(notifyAction)
				if err != nil {
					fmt.Printf("Error while sending notification of command %d: %s\n", cmdHeader.id, err)
				}
				game.RemovePlayer(player)
				player.CurrentGame = nil

			case CMD_DISCONNECT:
				fmt.Printf("Request to disconnect")
				game.RemovePlayer(player)
				player.CurrentGame = nil
				wantsToCloseConnection = true

				notifyAction := NewPlayerActionNotify(player.Id, CMD_GAME_LEAVE, DECK_ID_NONE, PLAYER_ID_NONE, nil)
				err = game.BroadcastNotification(notifyAction)
				if err != nil {
					fmt.Printf("Error while broadcasting notification of command %d: %s\n", cmdHeader.id, err)
				}

			default:
				fmt.Printf("Received unexpected command %d from %s, disconnecting...\n", cmdHeader.id, player.Name)
				sendInputError(player, cmdHeader.id, ERROR_INVALID_CMD_ID)
				wantsToCloseConnection = true
			}

		} else {
			switch cmdHeader.id {
			case CMD_KEEPALIVE:
				fmt.Printf("Keep-alive\n")
				// Do nothing

			case CMD_GAME_CREATE:
				var cmd GameCreateCommand
				err := SerialiseGameCreateCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Create game\n")

				spec, err := NewSpec(cmd.specData)
				if err != nil {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_DATA)
					fmt.Printf("Invalid specification provided for the 'create' command: " + err.Error())
					break
				}

				playerNameIsValidForGame := true
				lowerPlayerName := strings.ToLower(player.Name)
				for _, cardName := range spec.Deck {
					if lowerPlayerName == strings.ToLower(cardName) {
						playerNameIsValidForGame = false
						break
					}
				}
				if !playerNameIsValidForGame {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_PLAYER_NAME)
					fmt.Printf("Player '%s' could not create a game because they share a name with a card\n", player.Name)
					break
				}
				newGame := server.CreateNewGame(spec, player)

				respCmd := NotifyGameJoinedCommand{
					newGame.Id,
					cmd.specData,
					[]uint64{player.Id},
					[]string{player.Name},
					[][]uint16{nil},
					uint16(len(player.CurrentGame.Deck)),
				}
				respBuffer, respHeaderLen := WriteCommandHeader(CMD_NOTIFY_GAME_JOINED, uint16(respCmd.CommandLength()))
				err = SerialiseNotifyGameJoinedCommand(respBuffer[respHeaderLen:], &respCmd, false)
				if err != nil {
					fmt.Printf("Error! Failed to serialise game_create command %+v: %s\n", respCmd, err)
					break
				}
				err = player.SendCommandBuffer(respBuffer)
				if err != nil {
					fmt.Printf("Error! Failed to send response for command %d to player %d: %s\n", cmdHeader.id, player.Id, err)
				}

			case CMD_GAME_JOIN:
				var cmd GameJoinCommand
				err := SerialiseGameJoinCommand(cmdBuffer, &cmd, true)
				if err != nil {
					fmt.Printf("Error: Failed to read command %d body from player '%s': %s", cmdHeader.id, playerName, err)
					server.RemovePlayer(player.Id)
					return
				}
				fmt.Printf("Join game %d\n", cmd.gameId)

				gameToJoin := server.FindGame(cmd.gameId)
				if gameToJoin == nil {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_GAME_ID)
					fmt.Printf("Player '%s' failed not join unrecognised game ID %d\n", player.Name, cmd.gameId)
					break
				}

				lowerPlayerName := strings.ToLower(player.Name)
				nameAlreadyExists := false
				gameToJoin.mutex.Lock()
				for _, existingPlayer := range gameToJoin.Players {
					if lowerPlayerName == strings.ToLower(existingPlayer.Name) {
						nameAlreadyExists = true
						break
					}
				}
				gameToJoin.mutex.Unlock()
				if !nameAlreadyExists {
					for _, cardName := range gameToJoin.spec.Deck {
						if lowerPlayerName == strings.ToLower(cardName) {
							nameAlreadyExists = true
							break
						}
					}
				}
				if nameAlreadyExists {
					sendInputError(player, cmdHeader.id, ERROR_INVALID_PLAYER_NAME)
				}

				// Notify other players
				notify := NotifyGameJoinedCommand{
					gameToJoin.Id,
					nil,
					[]uint64{player.Id},
					[]string{player.Name},
					nil,
					0,
				}
				notifyBuffer, notifyHeaderLen := WriteCommandHeader(CMD_NOTIFY_GAME_JOINED, uint16(notify.CommandLength()))
				err = SerialiseNotifyGameJoinedCommand(notifyBuffer[notifyHeaderLen:], &notify, false)
				if err != nil {
					fmt.Printf("Error! Failed to serialise notify game join command %+v: %s\n", notify, err)
					break
				}
				gameToJoin.mutex.Lock()
				for _, player := range gameToJoin.Players {
					player.SendCommandBuffer(notifyBuffer)
				}
				gameToJoin.mutex.Unlock()

				// NOTE: Its important that we add the new player *after* sending the broadcast so that they do not get
				//	 	 the "you are already in the game" version of the new-player notification
				gameToJoin.AddPlayer(player)

				// Send all the relevant information to the new player
				specData, err := SerialiseSpecFromSpec(player.CurrentGame.spec)
				if err != nil {
					fmt.Printf("Failed to re-serialise game spec for sending to a new player\n")
					server.RemovePlayer(player.Id)
				}

				gameToJoin.mutex.Lock()
				allPlayerIds := make([]uint64, len(player.CurrentGame.Players))
				allPlayerNames := make([]string, len(player.CurrentGame.Players))
				allPlayerHands := make([][]uint16, len(player.CurrentGame.Players))
				for index, tempPlayer := range player.CurrentGame.Players {
					allPlayerIds[index] = tempPlayer.Id
					allPlayerNames[index] = tempPlayer.Name
					allPlayerHands[index] = tempPlayer.Hand
				}
				gameToJoin.mutex.Unlock()
				respCmd := NotifyGameJoinedCommand{
					player.CurrentGame.Id,
					specData,
					allPlayerIds,
					allPlayerNames,
					allPlayerHands,
					uint16(len(player.CurrentGame.Deck)),
				}
				respBuffer, respHeaderLen := WriteCommandHeader(CMD_NOTIFY_GAME_JOINED, uint16(respCmd.CommandLength()))
				err = SerialiseNotifyGameJoinedCommand(respBuffer[respHeaderLen:], &respCmd, false)
				if err != nil {
					fmt.Printf("Error! Failed to serialise notify game join command %+v: %s\n", notify, err)
					break
				}
				err = player.SendCommandBuffer(respBuffer)
				if err != nil {
					fmt.Printf("Error! Failed to send response for command %d to player %d: %s\n", cmdHeader.id, player.Id, err)
					server.RemovePlayer(player.Id)
				}

			case CMD_DISCONNECT:
				fmt.Printf("Request to disconnect from lobby\n")
				wantsToCloseConnection = true

			default:
				fmt.Printf("Unsupported command ID %d\n", cmdHeader.id)
				sendInputError(player, cmdHeader.id, ERROR_INVALID_CMD_ID)
				wantsToCloseConnection = true
			}
		}

		if wantsToCloseConnection {
			break
		}
	}

	if player != nil {
		server.RemovePlayer(player.Id)
	} else {
		playerConn.Close()
	}
	fmt.Println(playerName + " has disconnected")
}

func runServer() {
	fmt.Println("Launching server...")
	stdinChan := make(chan string)

	serverState := ServerState{
		&sync.Mutex{},
		uint64(1),
		uint64(1),
		make([]*PlayerState, 0),
		make([]*GameState, 0),
	}

	listener, err := net.Listen("tcp", ":43831")
	if err != nil {
		fmt.Println("Error: Failed to listen on TCP socket. ", err)
		return
	}

	go serverListenForConnections(listener, &serverState)
	go serverReadConsoleInput(stdinChan)

	for {
		select {
		case stdinCmd := <-stdinChan:
			if strings.Trim(stdinCmd, "\r\n\t ") == "quit" {
				fmt.Println("Shutting down the server...")
				listener.Close()
				fmt.Println("Listener stopped")
				serverState.Shutdown()
				fmt.Println("Game stopped")
				return
			}
		}
	}
}

func sendInputError(player *PlayerState, inputCmdId byte, cmdErr byte) {
	errCmd := NotifyInputErrorCommand{
		inputCmdId,
		cmdErr,
	}
	errBuffer, errHeaderLen := WriteCommandHeader(CMD_NOTIFY_INPUT_ERROR, uint16(NotifyInputErrorCommandLength))
	err := SerialiseNotifyInputErrorCommand(errBuffer[errHeaderLen:], &errCmd, false)
	if err != nil {
		fmt.Printf("ERROR: Failed to serialise input error notification %+v: %s\n", errCmd, err)
		return
	}
	err = player.SendCommandBuffer(errBuffer)
	if err != nil {
		fmt.Printf("ERROR: Failed to transmit input error notification to %s/%s: %s\n", player.Name, player.Conn.RemoteAddr().String(), err)
	}
}

func sliceInsert(slice []uint16, val uint16, index int) []uint16 {
	result := append(slice, 0)
	copy(result[index+1:], result[index:])
	result[index] = val
	return result
}
func makeFilledIdSlice(sliceLen int, val uint16) []uint16 {
	result := make([]uint16, sliceLen)
	for i := range result {
		result[i] = val
	}
	return result
}
