package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// TODO: Have a protocol version in the handshake
// TODO: Binary/smaller protocol for sending data
// TODO: Add a list of setup commands to the game spec (e.g shuffle, each player a defuse, each player draw 5, burn 20 from the deck, etc)

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

func clientReadSocketInput(conn net.Conn, inGameChan chan bool, quitChan chan bool) {
	socketRead := bufio.NewReader(conn)
	for {
		socketInput, err := socketRead.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Println("ERROR READING SOCKET RESPONSE: ", err)
			}
			quitChan <- true
			break
		}

		socketInput = strings.Trim(socketInput, "\r\n\t ")
		if (socketInput == "Successfully joined the game") || strings.HasPrefix(socketInput, "Successfully created a new game and joined it") {
			inGameChan <- true
		} else if socketInput == "You have left the game" {
			inGameChan <- false
		}

		if len(socketInput) > 0 {
			fmt.Println("Message from server: " + socketInput)
		}
	}
}

func runClient() {
	stdInRead := bufio.NewReader(os.Stdin)
	fmt.Print("Please enter your name: ")
	playerName, err := stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET NAME FROM STDIN", err)
		return
	}
	playerName = strings.Trim(playerName, "\n\r\t ")

	fmt.Print("Please enter the server you wish to connect to (or just hit enter to use the default): ")
	serverHost, err := stdInRead.ReadString('\n')
	if err != nil {
		fmt.Println("FAILED TO GET QUEST FROM STDIN", err)
		return
	}
	serverHost = strings.Trim(serverHost, "\n\r\t ")
	if len(serverHost) == 0 {
		serverHost = "localhost"
		//serverHost = "app-server-1.jacquesheunis.com"
	}

	fmt.Println("Connecting to " + serverHost + "...")
	conn, err := net.Dial("tcp", serverHost+":43831")
	if err != nil {
		// TODO: Direct people to some form of contact for me, or that they can host their own server too (--server), if you know how to do that and have the infrastructure
		fmt.Println("ERROR CONNECTING TO SERVER: ", err)
		return
	}
	conn.Write([]byte(playerName + "\n"))

	stdInChan := make(chan string)
	inGameChan := make(chan bool)
	quitChan := make(chan bool)
	go clientReadConsoleInput(stdInRead, stdInChan)
	go clientReadSocketInput(conn, inGameChan, quitChan)

	inGame := false
	fmt.Println("Connected successfully. Type 'help' (without the quotes) to see a list of possible commands")
	for {
		shouldQuit := false
		select {
		case inputLine := <-stdInChan:
			if inGame {
				switch inputLine {
				case "help":
					// TODO: Add a 'start' command to start a game (and none of the other commands work until then, notify a player when they join a game that is already in progress)
					// TODO: Add a "showhand" command to show your hand to another player
					// TODO: Add 'draw n' maybe? For convenience
					// TODO: Add a 'draw <this-card-name>' to be able to pull specific cards out of the deck? Important for setup, required for exploding kittens
					// TODO: Add a burn-up and burn-down command pair (lots of games work on a "reveal the top card of this deck" basis, and we can use it for that)

					// Exploding Kittens Expansions: Shuffle hand (and then its hidden), look at hand (for the curse). Draw from the bottom, Rearrange top n (could be solved by having multiple hands? But thats a bunch of extra complication, just trust)
					fmt.Println("You are currently in a game.")
					fmt.Println("The following commands are available while in-game:")
					fmt.Println("Long form     | Short Form | Description")
					fmt.Println("==============|====TODO====|============")
					fmt.Println("decks         |      decks | Show a list of all card decks in the game")
					fmt.Println("players       |          p | Show a list of all players in the game")
					fmt.Println("hand          |          h | Show a list of all the cards in your hand")
					fmt.Println("draw          |          d | Draw a card from the deck into your hand")
					fmt.Println("putback x y   |     pb x y | Put the card with ID x from your hand back into the deck y cards from the top")
					fmt.Println("reveal x      |        r x | Reveal the card with ID x in your hand to all other players")
					fmt.Println("discardup     |       du x | Discard the card with id x from your hand, face up")
					fmt.Println("discarddown x |       dd x | Discard the card with id x from your hand, face down")
					fmt.Println("showcard x y  |     sc x y | Show the card with id x in your hand to player y (in secret)")
					fmt.Println("give x y      |   give x y | Give the card with id x in your hand to player y")
					fmt.Println("giverand y    | giverand x | Give a random card from your hand to player y")
					fmt.Println("peek n        |     peek n | Look at the top n cards from the deck")
					fmt.Println("shuffle       |    shuffle | Shuffle the deck")
					fmt.Println("leave         |      leave | Leave the game that you are currently in and return to the menu")
					fmt.Println("quit          |       quit | Leave the current game (if you are in one) and close this application")
				default:
					fmt.Fprintf(conn, inputLine+"\n")
				}
			} else {
				switch inputLine {
				// TODO: When we get to sending game specs to the server, we'll need to do some extra work here for other commands
				case "help":
					fmt.Println("You are currently in the menu (and not in a game)")
					fmt.Println("The following commands are available from the menu:")
					fmt.Println("Command  | Description")
					fmt.Println("=========|============")
					fmt.Println("create x | Create a new game for others to join, using the specification  in the file 'x.yml'")
					fmt.Println("join x   | Join the existing game with ID x that was started by another player")
					fmt.Println("quit     | Quit the game")
				default:
					// TODO
					inputTokens := strings.Split(inputLine, " ")
					if (len(inputTokens) == 0) || (len(inputTokens[0]) == 0) {
						break
					}

					switch inputTokens[0] {
					case "create":
						if len(inputTokens) != 2 {
							fmt.Println("The 'create' command requires one parameter specifying the game name. You can enter the name of 'default' to get a generic 52-card deck")
							break
						}

						var spec string
						if inputTokens[1] == "default" {
							spec = DefaultSerializedGameSpec()
						} else {
							spec, err = SerializeSpecFromName(inputTokens[1])
							if err != nil {
								fmt.Println("Error reading local game specification: " + err.Error())
								break
							}
						}
						fmt.Fprintf(conn, "create %s\n", spec)

					default:
						fmt.Fprintf(conn, "%s\n", inputLine)
					}
				}
			}

		case newInGame := <-inGameChan:
			inGame = newInGame

		case <-quitChan:
			shouldQuit = true
		}

		if shouldQuit {
			break
		}
	}

	conn.Close()
}

func main() {
	var isServer = flag.Bool("server", false, "whether this instance should run as a server")
	flag.Parse()

	if *isServer {
		Run()
	} else {
		runClient()
	}

	fmt.Println("Thanks for playing!")
}
