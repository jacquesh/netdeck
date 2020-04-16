package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

type PlayerState struct {
	Socket net.Conn
	Name   string
	Hand   []string
}

func (ps *PlayerState) SendMessage(msg string) {
	if len(msg) == 0 {
		return
	}
	if msg[len(msg)-1] != '\n' {
		msg = msg + "\n"
	}

	ps.Socket.Write([]byte(msg))
}

func (ps *PlayerState) Draw(card string) {
	ps.Hand = append(ps.Hand, card)
}

func (ps *PlayerState) Discard(i int) {
	ps.Hand[i] = ps.Hand[len(ps.Hand)-1]
	ps.Hand = ps.Hand[:len(ps.Hand)-1]
}

func (ps *PlayerState) Kick() {
	ps.Socket.Close()
}

type GameState struct {
	Deck    []string
	Players []PlayerState
	mutex   *sync.Mutex
}

func nappend(slice []string, val string, n int) []string {
	for i := 0; i < n; i++ {
		slice = append(slice, val)
	}
	return slice
}
func NewGame() GameState {
	gs := GameState{
		make([]string, 0),
		make([]PlayerState, 0),
		&sync.Mutex{},
	}

	gs.Deck = nappend(gs.Deck, "Princess", 1)
	gs.Deck = nappend(gs.Deck, "Countess", 1)
	gs.Deck = nappend(gs.Deck, "King", 1)
	gs.Deck = nappend(gs.Deck, "Prince", 2)
	gs.Deck = nappend(gs.Deck, "Handmaid", 2)
	gs.Deck = nappend(gs.Deck, "Baron", 2)
	gs.Deck = nappend(gs.Deck, "Priest", 2)
	gs.Deck = nappend(gs.Deck, "Guard", 5)

	rand.Shuffle(len(gs.Deck), func(i, j int) {
		gs.Deck[i], gs.Deck[j] = gs.Deck[j], gs.Deck[i]
	})

	return gs
}

func (gs *GameState) NewPlayer(socket net.Conn, name string) *PlayerState {
	ps := PlayerState{
		socket,
		name,
		make([]string, 0),
	}
	gs.mutex.Lock()
	gs.Players = append(gs.Players, ps)
	gs.mutex.Unlock()
	return &gs.Players[len(gs.Players)-1]
}

func (gs *GameState) Broadcast(msg string) {
	gs.mutex.Lock()
	for _, player := range gs.Players {
		player.SendMessage(msg)
	}
	gs.mutex.Unlock()
}

func (gs *GameState) Draw() string {
	gs.mutex.Lock()
	result := ""
	if len(gs.Deck) > 0 {
		result = gs.Deck[len(gs.Deck)-1]
		gs.Deck = gs.Deck[:len(gs.Deck)-1]
	}
	gs.mutex.Unlock()
	return result
}

func (gs *GameState) Shutdown() {
	gs.Broadcast("Shutting down the server...")
	gs.mutex.Lock()
	for _, player := range gs.Players {
		player.Kick()
	}
	gs.mutex.Unlock()
}

func runServerPlayer(game *GameState, player *PlayerState) {
	socketRead := bufio.NewReader(player.Socket)
	for {
		inputLine, err := socketRead.ReadString('\n')
		if err != nil {
			fmt.Println("Error: Failed to read from socket", err)
			return
		}

		inputLine = strings.Trim(inputLine, "\r\n\t ")
		fmt.Println("Received input from " + player.Name + ": " + inputLine)

		tokens := strings.Split(inputLine, " ")
		if (len(tokens) == 0) || (len(tokens[0]) == 0) {
			continue
		}

		switch tokens[0] {
		case "players":
			fmt.Println("List players")
			response := "Players:\n"
			for i, p := range game.Players {
				response += "  " + strconv.Itoa(i) + "  " + p.Name + "\n"
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
			if len(tokens) != 2 {
				player.SendMessage("The 'reveal' command requires a single parameter")
			} else {
				cardIndex, err := strconv.Atoi(tokens[1])
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

		case "discardup":
			fmt.Println("Discard a card face up")
			if len(tokens) != 2 {
				player.SendMessage("The 'discardup' command requires a single parameter")
			} else {
				cardIndex, err := strconv.Atoi(tokens[1])
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
			if len(tokens) != 2 {
				player.SendMessage("The 'discarddown' command requires a single parameter")
			} else {
				cardIndex, err := strconv.Atoi(tokens[1])
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
			game.mutex.Lock()
			if len(tokens) != 3 {
				player.SendMessage("The 'showcard' command requires a two parameters")
			} else {
				cardIndex, err1 := strconv.Atoi(tokens[1])
				playerIndex, err2 := strconv.Atoi(tokens[2])
				if (err1 != nil) || (err2 != nil) {
					player.SendMessage("You must provide integers for the arguments to 'showcard'")
				} else {
					if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
						player.SendMessage("Invalid card index for 'showcard' command!")
					} else if (playerIndex < 0) || (playerIndex > len(game.Players)) {
						player.SendMessage("Invalid player index for 'showcard' command!")
					} else {
						game.Broadcast(player.Name + " showed a card from their hand to " + game.Players[playerIndex].Name)
						game.Players[playerIndex].Socket.Write([]byte(player.Name + " showed you their card: " + player.Hand[cardIndex] + "\n"))
					}
				}
			}
			game.mutex.Unlock()

		case "quit":
			return

		default:
			player.SendMessage("Unrecognised command '" + inputLine + "'")
		}
	}
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

func serverListenForConnections(listener net.Listener, connChan chan net.Conn) {
	fmt.Println("Listening for new connections...")
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error while accepting connection: ", err)
			return
		}

		connChan <- conn
	}
}

func runServer() {
	fmt.Println("Launching server...")
	game := NewGame()

	stdinChan := make(chan string)
	listenChan := make(chan net.Conn)

	listener, err := net.Listen("tcp", ":43831")
	if err != nil {
		fmt.Println("Error: Failed to listen on TCP socket. ", err)
		return
	}

	go serverListenForConnections(listener, listenChan)
	go serverReadConsoleInput(stdinChan)

	for {
		select {
		case newConn := <-listenChan:
			fmt.Println("Received connection from ", newConn.RemoteAddr().String())
			socketRead := bufio.NewReader(newConn)
			playerNameLine, err := socketRead.ReadString('\n')
			if err != nil {
				fmt.Println("Failed to read player name from socket", err)
				_ = newConn.Close()
				return
			}
			playerName := strings.Trim(playerNameLine, "\r\n\t ")
			game.Broadcast("Player join: " + playerName)
			newPlayer := game.NewPlayer(newConn, playerName)
			go runServerPlayer(&game, newPlayer)

		case stdinCmd := <-stdinChan:
			if strings.Trim(stdinCmd, "\r\n\t ") == "quit" {
				fmt.Println("Shutting down the server...")
				//os.Stdin.Close()
				//fmt.Println("Stdin closed")
				listener.Close()
				fmt.Println("Listener stopped")
				game.Shutdown()
				fmt.Println("Game stopped")
				return
			}
		}
	}
}

func clientReadConsoleInput(stdInRead *bufio.Reader, cmdChan chan string) {
	for {
		inputLine, err := stdInRead.ReadString('\n')
		if err != nil {
			fmt.Println("ERROR READING FROM STD INPUT")
			return
		}

		inputLine = strings.Trim(inputLine, "\r\n\t ")
		cmdChan <- inputLine
	}
}

func clientReadSocketInput(conn net.Conn) {
	socketRead := bufio.NewReader(conn)
	for {
		socketInput, err := socketRead.ReadString('\n')
		if err != nil {
			fmt.Println("ERROR READING SOCKET RESPONSE: ", err)
			break
		}

		socketInput = strings.Trim(socketInput, "\r\n\t ")
		if len(socketInput) > 0 {
			fmt.Println("Message from server: " + socketInput)
		}
	}
}

func runClient() {
	stdInRead := bufio.NewReader(os.Stdin)
	fmt.Print("WHAT is your name? ")
	playerName, err := stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET NAME FROM STDIN", err)
		return
	}
	playerName = strings.Trim(playerName, "\n\r\t ")

	fmt.Print("WHAT is your quest? ")
	serverHost, err := stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET QUEST FROM STDIN", err)
		return
	}
	serverHost = strings.Trim(serverHost, "\n\r\t ")

	fmt.Print("WHAT is your favourite colour? ")
	_, err = stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET FAVOURITE COLOUR FROM STDIN", err)
		return
	}
	fmt.Println("Jk I didn't need that, but I thought it'd be prudent to complete the trilogy")

	fmt.Println("Connecting to " + serverHost + "...")
	conn, err := net.Dial("tcp", serverHost+":43831")
	if err != nil {
		fmt.Println("ERROR CONNECTING TO SERVER: ", err)
		return
	}

	fmt.Println("Connected to ", conn.RemoteAddr().String())
	conn.Write([]byte(playerName + "\n"))

	stdInChan := make(chan string)
	go clientReadConsoleInput(stdInRead, stdInChan)
	go clientReadSocketInput(conn)

	for {
		inputLine := <-stdInChan
		fmt.Fprintf(conn, inputLine+"\n")
	}
}

func main() {
	var isServer = flag.Bool("server", false, "whether this instance should run as a server")
	flag.Parse()

	if *isServer {
		runServer()
	} else {
		runClient()
	}

	fmt.Println("Thanks for playing!")
}
