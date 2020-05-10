package main

import (
	"fmt"
	"os"

	"github.com/akamensky/argparse"
)

// TODO: Add a list of setup commands to the game spec (e.g shuffle, each player a defuse, each player draw 5, burn 20 from the deck, etc)
// TODO: Add a 'start' command to start a game (and none of the other commands work until then, notify a player when they join a game that is already in progress)
// TODO: Add a 'draw <this-card-name>' to be able to pull specific cards out of the deck? Important for setup, required for exploding kittens
// TODO: Add a burn-up and burn-down command pair (lots of games work on a "reveal the top card of this deck" basis, and we can use it for that)
// TODO: Consider adding chat so that you can use it without the video channel?
// TODO: Required for Exploding Kittens Expansions: Shuffle hand (and then its hidden), look at hand (for the curse). Draw from the bottom, Rearrange top n (could be solved by having multiple hands? But thats a bunch of extra complication, just trust)
// TODO: Support for multiple decks (hinted at by GameState::FindDeck, would need support in NotifyGameJoinedCommand, would need verification in NewSpec())
// TODO: Add a panic handler that prints some info (contact/github/etc)
// TODO: Possibly support having face-up cards in players hands and/or in the deck? Then we could show extra info in CMD_INFO_DECKS if the top card is face-up and CMD_CARD_PUTBACK can take an extra face-up argument
// TODO: Do some actual proper logging (at least on the server) rather than just printing everything to stdout
// TODO: Possibly add a 'lastplay' command that shows what the last action taken was?
// TODO: Add a 'roll' command ("roll 2 d20", "roll 2d6", "roll d2" etc)
// TODO: Make game IDs be something other than consecutive integers so that its just a little harder to join random people's games (and probably easier to tell people what to join too)
// TODO: Spec file docs
// TODO: Add a field to the spec for "game-specific instructions/help". Basically "how-to-play" or reference material (useful in Love Letter, for example)
// TODO: Add a state reload to both the client and the server so that I can kill the server and restart and all the clients can reconnect and carry on playing. This would let me deploy without there needing to be no running games.
// TODO: Add a command for sending text to all connected players from the server (which allows me to send shutdown notifications).

func main() {
	const DefaultServerAddr = "app-server-1.jacquesheunis.com"
	parser := argparse.NewParser("netdeck", "Helps you play card- and boardgames with your friends over the internet by providing a mechanism for managing and sharing hidden information (basically cards in each player's hand)")
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
