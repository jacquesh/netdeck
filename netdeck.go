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

	gs.Deck = nappend(gs.Deck, "PRINCESS", 1)
	gs.Deck = nappend(gs.Deck, "COUNTESS", 1)
	gs.Deck = nappend(gs.Deck, "KING", 1)
	gs.Deck = nappend(gs.Deck, "PRINCE", 2)
	gs.Deck = nappend(gs.Deck, "HANDMAID", 2)
	gs.Deck = nappend(gs.Deck, "BARON", 2)
	gs.Deck = nappend(gs.Deck, "PRIEST", 2)
	gs.Deck = nappend(gs.Deck, "GUARD", 5)

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
	gs.Broadcast("SHUTTING DOWN THE SERVER...")
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
			fmt.Println("ERROR: FAILED TO READ FROM SOCKET", err)
			return
		}

		inputLine = strings.Trim(inputLine, "\r\n\t ")
		fmt.Println("RECEIVED INPUT FROM " + player.Name + ": " + inputLine)

		tokens := strings.Split(inputLine, " ")
		if (len(tokens) == 0) || (len(tokens[0]) == 0) {
			continue
		}

		switch tokens[0] {
		case "PLAYERS":
			fmt.Println("LIST PLAYERS")
			response := "PLAYERS:\n"
			for i, p := range game.Players {
				response += "  " + strconv.Itoa(i) + "  " + p.Name + "\n"
			}
			player.SendMessage(response)

		case "HAND":
			response := "CARDS IN YOUR HAND:\n"
			for i, c := range player.Hand {
				response += "  " + strconv.Itoa(i) + "  " + c + "\n"
			}
			player.SendMessage(response)

		case "DRAW":
			newCard := game.Draw()
			if len(newCard) > 0 {
				player.Draw(newCard)
				player.SendMessage("YOU DREW: " + newCard)
				game.Broadcast(player.Name + " DREW A CARD")
			} else {
				player.SendMessage("NO CARDS LEFT TO DRAW!")
				game.Broadcast(player.Name + " TRIED TO DRAW A CARD, BUT THERE WERE NO CARDS LEFT!")
			}

		case "REVEAL":
			if len(tokens) != 2 {
				player.SendMessage("THE 'REVEAL' COMMAND REQUIRES A SINGLE PARAMETER")
			} else {
				cardIndex, err := strconv.Atoi(tokens[1])
				if err != nil {
					player.SendMessage("YOU MUST PROVIDE AN INTEGER FOR THE ARGUMENT TO 'REVEAL'")
				} else {
					if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
						player.SendMessage("INVALID CARD INDEX FOR 'REVEAL' COMMAND!")
					} else {
						game.Broadcast(player.Name + " REVEALED A CARD FROM THEIR HAND: " + player.Hand[cardIndex])
					}
				}
			}

		case "DISCARDUP":
			fmt.Println("DISCARD A CARD FACE UP")
			if len(tokens) != 2 {
				player.SendMessage("THE 'DISCARDUP' COMMAND REQUIRES A SINGLE PARAMETER")
			} else {
				cardIndex, err := strconv.Atoi(tokens[1])
				if err != nil {
					player.SendMessage("YOU MUST PROVIDE AN INTEGER FOR THE ARGUMENT TO 'DISCARDUP'")
				} else {
					if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
						player.SendMessage("INVALID CARD INDEX FOR 'DISCARDUP' COMMAND!")
					} else {
						game.Broadcast(player.Name + " DISCARDED A CARD FROM THEIR HAND: " + player.Hand[cardIndex])
						player.Discard(cardIndex)
					}
				}
			}

		case "DISCARDDOWN":
			fmt.Println("DISCARD A CARD FACE DOWN")
			if len(tokens) != 2 {
				player.SendMessage("THE 'DISCARDDOWN' COMMAND REQUIRES A SINGLE PARAMETER")
			} else {
				cardIndex, err := strconv.Atoi(tokens[1])
				if err != nil {
					player.SendMessage("YOU MUST PROVIDE AN INTEGER FOR THE ARGUMENT TO 'DISCARDDOWN'")
				} else {
					if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
						player.SendMessage("INVALID CARD INDEX FOR 'DISCARDDOWN' COMMAND!")
					} else {
						game.Broadcast(player.Name + " DISCARDED A CARD FROM THEIR HAND FACE-DOWN")
						player.Discard(cardIndex)
					}
				}
			}

		case "SHOWCARD":
			fmt.Println("SHOW A CARD TO ANOTHER PLAYER")
			game.mutex.Lock()
			if len(tokens) != 3 {
				player.SendMessage("THE 'SHOWCARD' COMMAND REQUIRES A TWO PARAMETERS")
			} else {
				cardIndex, err1 := strconv.Atoi(tokens[1])
				playerIndex, err2 := strconv.Atoi(tokens[2])
				if (err1 != nil) || (err2 != nil) {
					player.SendMessage("YOU MUST PROVIDE INTEGERS FOR THE ARGUMENTS TO 'SHOWCARD'")
				} else {
					if (cardIndex < 0) || (cardIndex > len(player.Hand)) {
						player.SendMessage("INVALID CARD INDEX FOR 'SHOWCARD' COMMAND!")
					} else if (playerIndex < 0) || (playerIndex > len(game.Players)) {
						player.SendMessage("INVALID PLAYER INDEX FOR 'SHOWCARD' COMMAND!")
					} else {
						game.Broadcast(player.Name + " SHOWED A CARD FROM THEIR HAND TO " + game.Players[playerIndex].Name)
						game.Players[playerIndex].Socket.Write([]byte(player.Name + " SHOWED YOU THEIR CARD: " + player.Hand[cardIndex] + "\n"))
					}
				}
			}
			game.mutex.Unlock()

		case "QUIT":
			return

		default:
			player.SendMessage("UNRECOGNISED COMMAND '" + inputLine + "'")
		}
	}
}

func serverReadConsoleInput(cmdChan chan string) {
	fmt.Println("READING INPUT FROM STDIN...")
	stdInRead := bufio.NewReader(os.Stdin)
	for {
		inputLine, err := stdInRead.ReadString('\n')
		if err != nil {
			fmt.Println("FAILED TO READ FROM STDIN: ", err)
			return
		}

		cmdChan <- inputLine
	}
}

func serverListenForConnections(listener net.Listener, connChan chan net.Conn) {
	fmt.Println("LISTENING FOR NEW CONNECTIONS...")
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("ERROR WHILE ACCEPTING CONNECTION: ", err)
			return
		}

		connChan <- conn
	}
}

func runServer() {
	fmt.Println("LAUNCHING SERVER...")
	game := NewGame()

	stdinChan := make(chan string)
	listenChan := make(chan net.Conn)

	listener, err := net.Listen("tcp", ":43831")
	if err != nil {
		fmt.Println("ERROR: FAILED TO LISTEN ON TCP SOCKET. ", err)
		return
	}

	go serverListenForConnections(listener, listenChan)
	go serverReadConsoleInput(stdinChan)

	for {
		select {
		case newConn := <-listenChan:
			fmt.Println("RECEIVED CONNECTION FROM ", newConn.RemoteAddr().String())
			socketRead := bufio.NewReader(newConn)
			playerNameLine, err := socketRead.ReadString('\n')
			if err != nil {
				fmt.Println("FAILED TO READ PLAYER NAME FROM SOCKET", err)
				_ = newConn.Close()
				return
			}
			playerName := strings.Trim(playerNameLine, "\r\n\t ")
			game.Broadcast("PLAYER JOIN: " + playerName)
			newPlayer := game.NewPlayer(newConn, playerName)
			go runServerPlayer(&game, newPlayer)

		case stdinCmd := <-stdinChan:
			if strings.Trim(stdinCmd, "\r\n\t ") == "QUIT" {
				fmt.Println("SHUTTING DOWN THE SERVER...")
				//os.Stdin.Close()
				//fmt.Println("STDIN CLOSED")
				listener.Close()
				fmt.Println("LISTENER STOPPED")
				game.Shutdown()
				fmt.Println("GAME STOPPED")
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
	fmt.Print("WHAT IS YOUR NAME? ")
	playerName, err := stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET NAME FROM STDIN", err)
		return
	}
	playerName = strings.Trim(playerName, "\n\r\t ")

	fmt.Print("WHAT IS YOUR QUEST? ")
	serverHost, err := stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET QUEST FROM STDIN", err)
		return
	}
	serverHost = strings.Trim(serverHost, "\n\r\t ")

	fmt.Print("WHAT IS YOUR FAVOURITE COLOUR? ")
	_, err = stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET FAVOURITE COLOUR FROM STDIN", err)
		return
	}
	fmt.Println("JK I DIDN'T NEED THAT, BUT I THOUGHT IT'D BE PRUDENT TO COMPLETE THE TRILOGY")

	fmt.Println("CONNECTING TO " + serverHost + "...")
	conn, err := net.Dial("tcp", serverHost+":43831")
	if err != nil {
		fmt.Println("ERROR CONNECTING TO SERVER: ", err)
		return
	}

	fmt.Println("CONNECTED TO ", conn.RemoteAddr().String())
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
	var isServer = flag.Bool("server", false, "WHETHER THIS INSTANCE SHOULD RUN AS A SERVER")
	flag.Parse()

	if *isServer {
		runServer()
	} else {
		runClient()
	}

	fmt.Println("THANKS FOR PLAYING!")
}
