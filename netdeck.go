package main

import (
	"fmt"
	"os"

	"github.com/akamensky/argparse"
)

// TODO: Have a protocol version in the handshake
// TODO: Binary/smaller protocol for sending data
// TODO: Add a list of setup commands to the game spec (e.g shuffle, each player a defuse, each player draw 5, burn 20 from the deck, etc)

// TODO: Add a 'start' command to start a game (and none of the other commands work until then, notify a player when they join a game that is already in progress)
// TODO: Add a "showhand" command to show your hand to another player
// TODO: Add 'draw n' maybe? For convenience
// TODO: Add a 'draw <this-card-name>' to be able to pull specific cards out of the deck? Important for setup, required for exploding kittens
// TODO: Add a burn-up and burn-down command pair (lots of games work on a "reveal the top card of this deck" basis, and we can use it for that)
// TODO: Let all commands reference entities by name rather than by ID (can we just require taht all players have unique names and not the same as that from a card)
// TODO: Consider adding chat so that you can use it without the video channel?
// TODO: Required for Exploding Kittens Expansions: Shuffle hand (and then its hidden), look at hand (for the curse). Draw from the bottom, Rearrange top n (could be solved by having multiple hands? But thats a bunch of extra complication, just trust)
// TODO: Enforce that no card/player names contain spaces and that no player shares a name with any card in the game. Then we can unambiguously translate any token of input into either a card or player. This would also mean we can use "randomcard" or "randomplayer" or "randomdeck" as valid input tokens and that we don't need arguments to be in a specific order.

// TODO: Support for multiple decks

func main() {
	const DefaultServerAddr = "localhost"
	//const DefaultServerAddr = "app-server-1.jacquesheunis.com"
	parser := argparse.NewParser("netdeck", "Helps you play card- and boardgames with your friends over the internet")
	//isServer := parser.Flag("s", "server", &argparse.Options{Default: false, Help: "Run as a server, rather than as a client"})
	mode := parser.Selector("m", "mode", []string{"client", "server"}, &argparse.Options{Default: "client", Help: "Whether to run as a client (and connect to a server) or as a server (that other clients can connect to)"})
	playerName := parser.String("n", "name", &argparse.Options{Help: "The name you wish to be known by to other players in the game"})
	serverAddr := parser.String("s", "server", &argparse.Options{Default: DefaultServerAddr, Help: "The address of the server to connect to (only valid when running in client mode)"})

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
	}

	if *mode == "server" {
		runServer()
	} else {
		runClient(*playerName, *serverAddr)
	}

	fmt.Println("Thanks for playing!")
}
