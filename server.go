package main

import (
	"bufio"
	"fmt"
	"math/rand"
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
		make([]string, 0),
		nil,
	}
	ss.allPlayers = append(ss.allPlayers, &ps)
	ss.mutex.Unlock()
	return &ps
}

func (ss *ServerState) RemovePlayer(playerId uint64) {
	ss.mutex.Lock()
	for index, player := range ss.allPlayers {
		if player.Id == playerId {
			player.Conn.Close()
			ss.allPlayers[index] = ss.allPlayers[len(ss.allPlayers)-1]
			ss.allPlayers = ss.allPlayers[:len(ss.allPlayers)-1]
		}
	}
	ss.mutex.Unlock()
}

func (ss *ServerState) CreateNewGame(spec *GameSpecification, firstPlayer *PlayerState) {
	gs := CreateGameFromSpec(spec)
	gs.ShuffleDeck()

	gs.Players = append(gs.Players, firstPlayer)
	firstPlayer.CurrentGame = &gs

	ss.mutex.Lock()
	gs.Id = ss.nextGameId
	ss.nextGameId += 1
	ss.allGames = append(ss.allGames, &gs)
	ss.mutex.Unlock()
	firstPlayer.SendMessage("Successfully created a new game and joined it. Your friends can join using your game ID: " + strconv.FormatUint(gs.Id, 10))
	fmt.Println("Created game", gs.Id)
}

func (ss *ServerState) TryJoinGame(newPlayer *PlayerState, gameId string) bool {
	joined := false
	ss.mutex.Lock()
	for _, game := range ss.allGames {
		game.mutex.Lock()
		game.Broadcast(newPlayer.Name + " has joined the game")
		game.Players = append(game.Players, newPlayer)
		newPlayer.CurrentGame = game
		game.mutex.Unlock()
		newPlayer.CurrentGame = game
		joined = true
		break
	}
	ss.mutex.Unlock()
	return joined
}

func (ss *ServerState) Broadcast(msg string) {
	ss.mutex.Lock()
	for _, player := range ss.allPlayers {
		player.SendMessage(msg)
	}
	ss.mutex.Unlock()
}

func (ss *ServerState) Shutdown() {
	ss.Broadcast("Shutting down the server...")
	ss.mutex.Lock()
	for _, player := range ss.allPlayers {
		player.Conn.Close()
	}
	ss.allPlayers = ss.allPlayers[:0]
	ss.allGames = ss.allGames[:0]
	ss.mutex.Unlock()
}

type PlayerState struct {
	Id          uint64
	Conn        net.Conn
	Name        string
	Hand        []string
	CurrentGame *GameState
}

func (ps *PlayerState) InGame() bool {
	return ps.CurrentGame != nil
}

func (ps *PlayerState) SendMessage(msg string) {
	if len(msg) == 0 {
		return
	}
	if msg[len(msg)-1] != '\n' {
		msg = msg + "\n"
	}

	ps.Conn.Write([]byte(msg))
}

func (ps *PlayerState) Draw(card string) {
	ps.Hand = append(ps.Hand, card)
}

func (ps *PlayerState) Discard(i int) {
	ps.Hand[i] = ps.Hand[len(ps.Hand)-1]
	ps.Hand = ps.Hand[:len(ps.Hand)-1]
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

		fmt.Println("Received connection from ", newConn.RemoteAddr().String())
		go runServerPlayer(server, newConn)
	}
}

func runServerPlayer(server *ServerState, playerConn net.Conn) {
	connRead := bufio.NewReader(playerConn)
	playerNameLine, err := connRead.ReadString('\n')
	if err != nil {
		fmt.Println("Failed to read player name from remote socket", playerConn.RemoteAddr(), err)
		_ = playerConn.Close()
		return
	}
	playerName := strings.Trim(playerNameLine, "\r\n\t ")
	fmt.Println("Received player name from ", playerConn.RemoteAddr(), "-", playerName)

	player := server.AddPlayer(playerConn, playerName)

	for {
		inputLine, err := connRead.ReadString('\n')
		if err != nil {
			fmt.Println("Error: Failed to read from remote socket", playerConn.RemoteAddr(), err)
			server.RemovePlayer(player.Id)
			break
		}
		inputLine = strings.Trim(inputLine, "\r\n\t ")
		fmt.Println("Received input from " + playerName + ": " + inputLine)

		inputTokens := strings.Split(inputLine, " ")
		if (len(inputTokens) == 0) || (len(inputTokens[0]) == 0) {
			continue
		}

		wantsToCloseConnection := false
		if player.InGame() {
			game := player.CurrentGame
			game.mutex.Lock()
			switch inputTokens[0] {
			case "decks":
				// TODO: Shows if the top card is face up, what it is? If face-up in a deck is a thing we support?
				player.SendMessage("The deck contains " + strconv.Itoa(len(game.Deck)) + " cards")

			case "players":
				fmt.Println("List players")
				response := "Players:\n"
				for i, p := range game.Players {
					if p.Id == player.Id {
						response += "  " + strconv.Itoa(i) + "  " + p.Name + "  (You)\n"
					} else {
						response += "  " + strconv.Itoa(i) + "  " + p.Name + "\n"
					}
				}
				player.SendMessage(response)

			case "hand":
				response := "Cards in your hand:\n"
				for i, c := range player.Hand {
					response += "  " + strconv.Itoa(i) + "  " + c + "\n"
				}
				player.SendMessage(response)

			case "draw":
				newCard := game.Draw()
				if len(newCard) > 0 {
					player.Draw(newCard)
					player.SendMessage("You drew: " + newCard)
					game.Broadcast(player.Name + " drew a card")
				} else {
					player.SendMessage("No cards left to draw!")
					game.Broadcast(player.Name + " tried to draw a card, but there were no cards left!")
				}

			case "reveal":
				if len(inputTokens) != 2 {
					player.SendMessage("The 'reveal' command requires one parameter")
				} else {
					cardIndex, err := strconv.Atoi(inputTokens[1])
					if err != nil {
						player.SendMessage("You must provide an integer for the argument to 'reveal'")
					} else {
						if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
							player.SendMessage("Invalid card index for 'reveal' command!")
						} else {
							game.Broadcast(player.Name + " revealed a card from their hand: " + player.Hand[cardIndex])
						}
					}
				}

			case "putback":
				fmt.Println("Put a card back into the deck")
				var cardIndex int
				var deckDepth int
				if len(inputTokens) != 3 {
					player.SendMessage("The 'putback' command requires two parameters")
					break
				} else {
					var err1, err2 error
					cardIndex, err1 = strconv.Atoi(inputTokens[1])
					deckDepth, err2 = strconv.Atoi(inputTokens[2])
					if (err1 != nil) || (err2 != nil) {
						player.SendMessage("You must provide integers for the arguments to 'putback'")
						break
					}
				}

				if (cardIndex < 0) || (cardIndex >= len(player.Hand)) {
					player.SendMessage("Invalid card index for the 'putback' command")
					break
				}

				if (deckDepth < 0) || (deckDepth >= len(game.Deck)) {
					player.SendMessage("Invalid depth in the deck for 'putback' command!")
					break
				}

				fmt.Println("Put card", cardIndex, "into the deck at a depth of", deckDepth)
				deckIndex := len(game.Deck) - deckDepth
				game.Deck = sliceInsert(game.Deck, player.Hand[cardIndex], deckIndex)
				player.Discard(cardIndex)
				game.Broadcast(player.Name + " put a card from their hand back into the deck")

			case "discardup":
				fmt.Println("Discard a card face up")
				if len(inputTokens) != 2 {
					player.SendMessage("The 'discardup' command requires one parameter")
				} else {
					cardIndex, err := strconv.Atoi(inputTokens[1])
					if err != nil {
						player.SendMessage("You must provide an integer for the argument to 'discardup'")
					} else {
						if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
							player.SendMessage("Invalid card index for 'discardup' command!")
						} else {
							game.Broadcast(player.Name + " discarded a card from their hand: " + player.Hand[cardIndex])
							player.Discard(cardIndex)
						}
					}
				}

			case "discarddown":
				fmt.Println("Discard a card face down")
				if len(inputTokens) != 2 {
					player.SendMessage("The 'discarddown' command requires a single parameter")
				} else {
					cardIndex, err := strconv.Atoi(inputTokens[1])
					if err != nil {
						player.SendMessage("You must provide an integer for the argument to 'discarddown'")
					} else {
						if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
							player.SendMessage("Invalid card index for 'discarddown' command!")
						} else {
							game.Broadcast(player.Name + " discarded a card from their hand face-down")
							player.Discard(cardIndex)
						}
					}
				}

			case "showcard":
				fmt.Println("Show a card to another player")
				var cardIndex int
				var playerIndex int
				if len(inputTokens) != 3 {
					player.SendMessage("The 'showcard' command requires two parameters")
					break
				} else {
					var err1, err2 error
					cardIndex, err1 = strconv.Atoi(inputTokens[1])
					playerIndex, err2 = strconv.Atoi(inputTokens[2])
					if (err1 != nil) || (err2 != nil) {
						player.SendMessage("You must provide integers for the arguments to 'showcard'")
						break
					}
				}

				fmt.Println("Show a card", cardIndex, "to player", playerIndex)
				if (cardIndex < 0) || (cardIndex >= len(player.Hand)) {
					player.SendMessage("Invalid card index for 'showcard' command!")
					break
				}

				if (playerIndex < 0) || (playerIndex >= len(game.Players)) {
					player.SendMessage("Invalid player index for 'showcard' command!")
					break
				}
				targetPlayer := game.Players[playerIndex]
				targetPlayer.SendMessage(player.Name + " showed you their card: " + player.Hand[cardIndex])
				game.Broadcast(player.Name + " showed a card from their hand to " + targetPlayer.Name)

			case "give":
				fmt.Println("Give a card to another player")
				var cardIndex int
				var playerIndex int
				if len(inputTokens) != 3 {
					player.SendMessage("The 'give' command requires two parameters")
					break
				} else {
					var err1, err2 error
					cardIndex, err1 = strconv.Atoi(inputTokens[1])
					playerIndex, err2 = strconv.Atoi(inputTokens[2])
					if (err1 != nil) || (err2 != nil) {
						player.SendMessage("You must provide integers for the arguments to 'give'")
						break
					}
				}
				fmt.Println("Give card", cardIndex, "to player", playerIndex)
				if (cardIndex < 0) || (cardIndex >= len(player.Hand)) {
					player.SendMessage("Invalid card index for 'give' command!")
					break
				}
				if (playerIndex < 0) || (playerIndex >= len(game.Players)) {
					player.SendMessage("Invalid player index for 'give' command!")
					break
				}
				targetPlayer := game.Players[playerIndex]
				targetPlayer.SendMessage(player.Name + " gave you their card: " + player.Hand[cardIndex])
				targetPlayer.Hand = append(targetPlayer.Hand, player.Hand[cardIndex])
				player.Discard(cardIndex)
				game.Broadcast(player.Name + " gave a card from their hand to " + targetPlayer.Name)

			case "giverand":
				fmt.Println("Give a random card to another player")
				var playerIndex int
				if len(inputTokens) != 2 {
					player.SendMessage("The 'giverand' command requires one parameter")
					break
				} else {
					playerIndex, err = strconv.Atoi(inputTokens[1])
					if err != nil {
						player.SendMessage("You must provide an integer for the argument to 'giverand'")
						break
					}
				}

				if len(player.Hand) == 0 {
					player.SendMessage("You have no cards in your hand to give!")
					break
				}

				if (playerIndex < 0) || (playerIndex >= len(game.Players)) {
					player.SendMessage("Invalid player index for 'give' command!")
					break
				}

				cardIndex := rand.Intn(len(player.Hand))
				fmt.Println("Give card", cardIndex, "to player", playerIndex)
				targetPlayer := game.Players[playerIndex]
				player.SendMessage("You gave your " + player.Hand[cardIndex] + " card to " + targetPlayer.Name)
				targetPlayer.SendMessage(player.Name + " gave you their card: " + player.Hand[cardIndex])
				targetPlayer.Hand = append(targetPlayer.Hand, player.Hand[cardIndex])
				player.Discard(cardIndex)
				game.Broadcast(player.Name + " gave a card from their hand to " + targetPlayer.Name)

			case "peek":
				if len(inputTokens) != 2 {
					player.SendMessage("The 'peek' command requires one parameter")
					break
				}
				cardCount, err := strconv.Atoi(inputTokens[1])
				if err != nil {
					player.SendMessage("You must provide an integer for the argument to 'peek'")
					break
				}
				var response string
				deckSlice := game.Deck
				if cardCount <= len(deckSlice) {
					response = "The top " + inputTokens[1] + " cards in the deck (ordered from top to bottom) are:\n"
					deckSlice = deckSlice[len(deckSlice)-cardCount:]
				} else {
					response = "The deck contains only " + strconv.Itoa(len(deckSlice)) + " cards. They are (ordered from top to bottom):\n"
				}
				for i := len(deckSlice) - 1; i >= 0; i-- {
					response += "  " + deckSlice[i] + "\n"
				}
				game.Broadcast(player.Name + " looked at the top " + inputTokens[1] + " cards of the deck")
				player.SendMessage(response)

			case "shuffle":
				game.ShuffleDeck()
				game.Broadcast(player.Name + " shuffled the deck")

			case "leave":
				game.RemovePlayer(player)
				game.Broadcast(player.Name + " has left the game")
				player.CurrentGame = nil
				// TODO: This should probably clean up the game if there are no more players in it
				player.SendMessage("You have left the game")

			case "quit":
				game.RemovePlayer(player)
				game.Broadcast(player.Name + " has left the game")
				player.CurrentGame = nil
				wantsToCloseConnection = true

			default:
				player.SendMessage("Unrecognised command '" + inputLine + "'")
				// TODO: Add validation on the client-side and then kill their connection immediately here
			}
			game.mutex.Unlock()
		} else {
			switch inputTokens[0] {
			case "create":
				if len(inputTokens) != 2 {
					player.SendMessage("The 'create' command requires two parameters")
					break
				}
				spec, err := NewSpec(inputTokens[1])
				if err != nil {
					player.SendMessage("Invalid specification provided for the 'create' command: " + err.Error())
					break
				}
				server.CreateNewGame(spec, player)

			case "join":
				if len(inputTokens) != 2 {
					player.SendMessage("The 'join' command requires one parameter")
					break
				}

				fmt.Println("Join a game")
				if server.TryJoinGame(player, inputTokens[1]) {
					player.SendMessage("Successfully joined the game")
				} else {
					player.SendMessage("Error: Could not join the game with ID " + inputTokens[1])
				}

			case "quit":
				wantsToCloseConnection = true
			}
		}

		if wantsToCloseConnection {
			break
		}
	}

	player.SendMessage("Goodbye")
	server.RemovePlayer(player.Id)
	fmt.Println(player.Name + " has disconnected")
}

func Run() {
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

func appendN(slice []string, val string, n int) []string {
	// TODO: This could be made way faster if we just allocated the space once
	for i := 0; i < n; i++ {
		slice = append(slice, val)
	}
	return slice
}
func sliceInsert(slice []string, val string, index int) []string {
	result := append(slice, "")
	copy(result[index+1:], result[index:])
	result[index] = val
	return result
}
