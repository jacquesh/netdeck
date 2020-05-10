package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrInsufficientArguments = errors.New("Fewer than the required number of arguments were provided")

func clientReadConsoleInput(stdInRead *bufio.Reader, stdinChan chan string) {
	for {
		inputLine, err := stdInRead.ReadString('\n')
		if err != nil {
			fmt.Println("ERROR READING FROM STD INPUT")
			return
		}

		inputLine = strings.TrimSpace(inputLine)
		stdinChan <- inputLine
	}
}

func clientReadSocketInput(conn net.Conn, cmdChan chan CommandContainer, quitChan chan bool) {
	for {
		headerBytes, err := ReadExactlyNBytes(conn, CommandHeaderLength)
		if err != nil {
			fmt.Printf("Error: Failed to read command header from server: %s\n", err)
			quitChan <- true
			return
		}

		var cmdHeader CommandHeader
		err = SerialiseCommandHeader(headerBytes, &cmdHeader, true)
		if err != nil {
			fmt.Printf("Error: Failed to read command header from server: %s\n", err)
			quitChan <- true
			return
		}

		err = ValidateCommandHeader(cmdHeader)
		if err != nil {
			fmt.Printf("ERROR: Invalid command header {id=%d,len=%d} received from server: %s\n",
				cmdHeader.id, cmdHeader.len, err)
			quitChan <- true
			return
		}

		cmdBuffer, err := ReadExactlyNBytes(conn, cmdHeader.len)
		if err != nil {
			fmt.Printf("ERROR: Failed to read command buffer of length %d for command %d from server: %s\n",
				cmdHeader.len, cmdHeader.id, err)
			quitChan <- true
			return
		}

		cmdContainer := CommandContainer{cmdHeader, cmdBuffer}
		cmdChan <- cmdContainer
	}
}

func handleInputFromStdin(inputLine string, conn net.Conn, game *GameState, inGame bool, localPlayer *PlayerState) {
	inputTokens := strings.Split(inputLine, " ")
	if len(inputTokens) == 0 {
		return
	}

	cmdStr := inputTokens[0]
	unusedCmdArgs := inputTokens[1:]
	if inGame {
		if cmdStr == "help" {
			fmt.Print(`
You are currently in a game.

From here you can play the game by executing any of the commands below. There is no enforcement of 'turns' or 'rules'
"in netdeck, in the same way that you could pick up a card from the deck at any time while sitting around a table,
so the same is here. However just as when at a table, everybody will know if you do something you shouldn't!

In the list of commands below, some take arguments to determine what they do. For example the "draw" command takes
one argument (called "n" here) that specifies the number of cards to draw. Note that in the table below, the "n"
appears enclosed in square brackets. This means that the argument is optional and you can leave it out if you're
happy with the default. The "showcard" command takes two arguments: One to specify the card to show and one to
specify the player to which that card should be shown. Both of these arguments are required but with all commands
("showcard" included), the order in which the arguments appear is not important. This means that the following
commands will both give the same result: "showcard PlayerA CardB" or "showcard CardB PlayerA".

When specifying player or card names, the arguments you give are not case-sensitive. So "Duke" and "duke" and "dUkE"
are all effectively the same. Sometimes players or cards also have long names, you can also give any text that is a
prefix for the card/player you'd like to specify instead of typing out the full name each time.
For example if you want to show the "Fireball" card to player "FooBarrington" you could use "show fire foo".
Note that this needs to be unambiguous though, so if there were another player in the game whose name was
"FooBarringstead" then you'd need to type out at least "foobarringt" to be clear which player you are referring to.

One special case is the "discard" command that has an optional "facedown" parameter. If you wish to discard a card
face-up, then simply leave this parameter out and specify only the card. If you wish to discard a card face down,
then one of the parameters you give should be the text "facedown" (or the shorter form "down").

The final piece of information that you need here is that whenever you specify a card or player as an argument,
you can also use the text "anycard"/"allcards" or "anyplayer"/"allplayers" respectively. The "anycard"/"anyplayer"
arguments instruct the server to select one at random (allowing you to discard a random card, or show a card to
a random player). The "allcards"/"allplayers" arguments instruct the server to apply the command to every one of
the relevant entity (which allows you to discard your entire hand, or show a card to all players, for example).

The following commands are currently available to you:
=======================================================================================================================
Long form             |   Short Form   | Description
======================|================|============
decks                 |              - | Show a list of all card decks in the game
players               |             pl | Show a list of all players in the game
hand                  |             ha | Show a list of all the cards in your hand
draw [n]              |            d n | Draw n cards from the deck into your hand. By default n is 1
putback x y           |         pb x y | Put card x from your hand back into the deck y cards from the top
discard x [facedown]  |   dis x [down] | Discard card id x from your hand
showcard x y          |       show x y | Show card x in your hand to player y
givecard x y          |       give x y | Give card x in your hand to player y
peek n                |              - | Look at the top n cards from the deck
shuffle               |              - | Shuffle the deck
leave                 |              - | Leave the game that you are currently in and return to the menu
help                  |              - | Show the currently-available commands and basic instructions.
quit                  |              - | Leave the current game (if you are in one) and close this application
=======================================================================================================================

`)

		} else if cmdStr == "decks" {
			buffer, _ := WriteCommandHeader(CMD_INFO_DECKS, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "players") || (cmdStr == "pl") {
			buffer, _ := WriteCommandHeader(CMD_INFO_PLAYERS, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "hand") || (cmdStr == "ha") {
			buffer, _ := WriteCommandHeader(CMD_INFO_CARDS, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "draw") || (cmdStr == "d") {
			cardCount, err := parseInputUint16(unusedCmdArgs[:])
			if err != nil {
				cardCount = 1
			}
			buffer, headerLen := WriteCommandHeader(CMD_CARD_DRAW, CardDrawCommandLength)
			cmd := CardDrawCommand{
				0,
				cardCount,
				false,
			}
			SerialiseCardDrawCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "putback") || (cmdStr == "pb") {
			cardId, err := parseCardIdFromHand(game, localPlayer, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error: Failed to parse the <card> argument for '%s': %s\n", cmdStr, err)
				return
			}

			cardsFromTop, err := parseInputUint16(unusedCmdArgs[:])
			if err != nil {
				fmt.Printf("Error! Failed to parse the <cardsFromTop> argument for '%s': %s\n", cmdStr, err)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_CARD_PUTBACK, CardPutbackCommandLength)
			cmd := CardPutbackCommand{
				cardId,
				0,
				cardsFromTop,
			}
			SerialiseCardPutbackCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "discard") || (cmdStr == "dis") {
			cardId, err := parseCardIdFromHand(game, localPlayer, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error! Failed to parse the <card> argument for '%s': %s\n", cmdStr, err)
				return
			}

			faceUp := true
			if stringInSlice("facedown", unusedCmdArgs) ||
				stringInSlice("down", unusedCmdArgs) {
				faceUp = false
			}

			buffer, headerLen := WriteCommandHeader(CMD_CARD_DISCARD, CardDiscardCommandLength)
			cmd := CardDiscardCommand{
				cardId,
				faceUp,
			}
			SerialiseCardDiscardCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "givecard") || (cmdStr == "give") {
			cardId, err := parseCardIdFromHand(game, localPlayer, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error! Failed to parse the <card> argument for '%s': %s\n", cmdStr, err)
				return
			}

			playerId, err := parsePlayerId(game, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error! Failed to parse the <player> argument for '%s': %s\n", cmdStr, err)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_CARD_GIVE, CardGiveCommandLength)
			cmd := CardGiveCommand{
				cardId,
				playerId,
				false,
			}
			SerialiseCardGiveCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if (cmdStr == "showcard") || (cmdStr == "show") {
			cardId, err := parseCardIdFromHand(game, localPlayer, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error! Failed to parse the <card> argument for '%s': %s\n", cmdStr, err)
				return
			}

			playerId, err := parsePlayerId(game, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error! Failed to parse the <playerId> argument for '%s': %s\n", cmdStr, err)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_CARD_SHOW, CardShowCommandLength)
			cmd := CardShowCommand{
				cardId,
				playerId,
			}
			SerialiseCardShowCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if cmdStr == "giverand" {
			playerId, err := parsePlayerId(game, &unusedCmdArgs)
			if err != nil {
				fmt.Printf("Error! Failed to parse the <player> argument for '%s': %s\n", cmdStr, err)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_CARD_GIVE, CardGiveCommandLength)
			cmd := CardGiveCommand{
				CARD_ID_ANY,
				playerId,
				false,
			}
			SerialiseCardGiveCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if cmdStr == "peek" {
			count, err := parseInputUint16(unusedCmdArgs[:])
			if err != nil {
				fmt.Printf("Error! Failed to parse arguments for '%s': %s\n", cmdStr, err)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_DECK_PEEK, DeckPeekCommandLength)
			cmd := DeckPeekCommand{
				0,
				count,
				false,
			}
			SerialiseDeckPeekCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if cmdStr == "shuffle" {
			buffer, headerLen := WriteCommandHeader(CMD_DECK_SHUFFLE, DeckShuffleCommandLength)
			cmd := DeckShuffleCommand{
				0,
			}
			SerialiseDeckShuffleCommand(buffer[headerLen:], &cmd, false)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if cmdStr == "leave" {
			buffer, _ := WriteCommandHeader(CMD_GAME_LEAVE, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if cmdStr == "quit" {
			buffer, _ := WriteCommandHeader(CMD_DISCONNECT, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else {
			fmt.Printf("Unrecognised command: '%s', enter 'help' for a list of available commands\n", inputLine)
		}
	} else {
		if cmdStr == "help" {
			fmt.Print(`
You are currently in the menu (and not in a game)

From here you can create a new game which will give you an ID that your friends can use to join your game, or you can
join an existing game using the game ID of a game that a friend has already created.
If you wish to create a game, use the 'create' command along with the name of a game-specification file located in the
same folder as netdeck. You can find out more about game specifications online at https://github.com/jacquesh/netdeck

Even if you have not created any specification files, you can always create a game that uses a single, standard 52-card
deck (the Ace-to-King kind) by entering 'create default' (without the quotes). This uses a built-in specification file.

The following commands are currently available to you:
=======================================================================================================================
Command         | Description
================|============
create <name>   | Create a new game for others to join, using the specification in the local file 'name.yml'
join <game-id>  | Join the existing game with ID x that was started by another player
help            | Show the currently-available commands and basic instructions. Shows different info while in-game.
quit            | Quit netdeck
=======================================================================================================================

`)

		} else if cmdStr == "create" {
			if len(inputTokens) != 2 {
				fmt.Println("The 'create' command requires one argument specifying the game name. You can enter the name of 'default' to get a generic 52-card deck")
				return
			}

			var spec []byte
			if inputTokens[1] == "default" {
				spec = DefaultSerializedGameSpec()
			} else {
				var err error
				spec, err = SerialiseSpecFromName(inputTokens[1])
				if err != nil {
					fmt.Println("Error reading local game specification: " + err.Error())
					return
				}
			}
			if len(spec) > MaxGameCreateSpecDataLength {
				fmt.Printf("Game spec for '%s' is %d bytes, which is larger than the max allowed %d bytes\n",
					inputTokens[1], len(spec), MaxGameCreateSpecDataLength)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_GAME_CREATE, uint16(GameCreateCommandLength(len(spec))))
			cmd := GameCreateCommand{spec}
			SerialiseGameCreateCommand(buffer[headerLen:], &cmd, false)

			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}
			fmt.Printf("Sent game creation for '%s'...\n", inputTokens[1])

		} else if cmdStr == "join" {
			gameId, err := parseInputUint64(inputTokens[1:])
			if err != nil {
				fmt.Printf("Error! Failed to parse arguments for '%s': %s\n", cmdStr, err)
				return
			}

			buffer, headerLen := WriteCommandHeader(CMD_GAME_JOIN, GameJoinCommandLength)
			cmd := GameJoinCommand{gameId}
			SerialiseGameJoinCommand(buffer[headerLen:], &cmd, false)
			err = sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else if cmdStr == "quit" {
			buffer, _ := WriteCommandHeader(CMD_DISCONNECT, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send '%s' command to the server: %s\n", cmdStr, err)
			}

		} else {
			fmt.Printf("Unrecognised command: '%s', enter 'help' for a list of available commands\n", inputLine)
		}
	}
}

func runClient(playerName string, serverHost string) {
	stdInRead := bufio.NewReader(os.Stdin)
	if len(playerName) == 0 {
		for len(playerName) == 0 {
			var err error
			fmt.Print("Please enter your name: ")
			playerName, err = stdInRead.ReadString('\n')
			if err != nil {
				fmt.Println("FAILED TO GET NAME FROM STDIN", err)
				return
			}
			playerName = strings.TrimSpace(playerName)
			if strings.ContainsAny(playerName, " \t\r\n") {
				fmt.Println("Sorry, but your alias/name on this service cannot contain any spaces. Please enter a different name.")
			}
			fmt.Printf("Playername = %s\n", playerName)
		}

	} else if strings.ContainsAny(playerName, " \t\r\n") {
		fmt.Println("Sorry, but your alias/name on this service cannot contain any spaces.")
		return
	}

	fmt.Println("Connecting to " + serverHost + "...")
	conn, err := net.Dial("tcp", serverHost+":43831")
	if err != nil {
		// TODO: Direct people to some form of contact for me, or that they can host their own server too (--server), if you know how to do that and have the infrastructure
		fmt.Println("ERROR CONNECTING TO SERVER: ", err)
		return
	}

	handshake := HandshakeCommand{
		PROTOCOL_MAGIC_NUMBER,
		PROTOCOL_ID,
		playerName,
	}
	handshakeBuffer, handshakeHeaderLen := WriteCommandHeader(CMD_HANDSHAKE, handshake.CommandLength())
	SerialiseHandshakeCommand(handshakeBuffer[handshakeHeaderLen:], &handshake, false)
	err = sendCommandBuffer(handshakeBuffer, conn)
	if err != nil {
		fmt.Printf("Failed to send handshake to the server, disconnecting: %s\n", err)
		conn.Close()
		return
	}

	stdInChan := make(chan string)
	cmdChan := make(chan CommandContainer)
	quitChan := make(chan bool)
	keepAliveTimer := time.NewTimer(10 * time.Second)
	go clientReadConsoleInput(stdInRead, stdInChan)
	go clientReadSocketInput(conn, cmdChan, quitChan)

	game := GameState{}
	inGame := false
	var localPlayer *PlayerState = nil
	var localPlayerId uint64 = 0

	fmt.Println("Connected successfully. Waiting for handshake response...")
	for {
		shouldQuit := false
		select {
		case <-keepAliveTimer.C:
			buffer, _ := WriteCommandHeader(CMD_KEEPALIVE, 0)
			err := sendCommandBuffer(buffer, conn)
			if err != nil {
				fmt.Printf("Error! Failed to send keep-alive packet to the server: %s\n", err)
			}

		case inputLine := <-stdInChan:
			handleInputFromStdin(inputLine, conn, &game, inGame, localPlayer)

		case cmdContainer := <-cmdChan:
			cmdId := cmdContainer.header.id
			switch cmdId {
			case CMD_HANDSHAKE_RESPONSE:
				var cmd HandshakeResponseCommand
				SerialiseHandshakeResponseCommand(cmdContainer.payload, &cmd, true)
				localPlayerId = cmd.playerId
				fmt.Println("Handshake completed successfully. Type 'help' (without the quotes) to see a list of possible commands")

			case CMD_INFO_PLAYERS_RESPONSE:
				var cmd PlayerInfoResponseCommand
				err := SerialisePlayerInfoResponseCommand(cmdContainer.payload, &cmd, true)
				if err != nil {
					fmt.Printf("Received invalid PlayerInfoResponseCommand: %s - %+v - %+v\n", err, cmd, cmdContainer.payload)
				}
				diverged := (len(cmd.ids) != len(game.Players))
				fmt.Print("Players:\n")
				for i := 0; i < len(cmd.ids); i++ {
					fmt.Printf("  %s  %d cards in-hand", cmd.names[i], cmd.handSizes[i])
					if cmd.ids[i] == localPlayer.Id {
						fmt.Println("  <-- This is you")
					} else {
						fmt.Println()
					}
					if !diverged && (cmd.ids[i] != game.Players[i].Id) {
						diverged = true
					}
				}
				if diverged {
					fmt.Println("ERROR: Local view of the players in the game has diverged from the server. This is a bug.")
				}

			case CMD_INFO_DECKS_RESPONSE:
				var cmd DeckInfoResponseCommand
				SerialiseDeckInfoResponseCommand(cmdContainer.payload, &cmd, true)
				fmt.Printf("The deck contains %d cards\n", cmd.cardCounts[0])

			case CMD_INFO_CARDS_RESPONSE:
				var cmd CardInfoResponseCommand
				SerialiseCardInfoResponseCommand(cmdContainer.payload, &cmd, true)
				diverged := (len(cmd.ids) != len(localPlayer.Hand))
				if len(cmd.ids) == 0 {
					fmt.Println("You have no cards in your hand")
				} else {
					fmt.Println("Cards in your hand:")
					for cardIndex, cardId := range cmd.ids {
						fmt.Printf("  - %s\n", game.spec.CardName(cardId))
						if !diverged && (cardId != localPlayer.Hand[cardIndex]) {
							diverged = true
						}
					}
				}
				if diverged {
					fmt.Println("ERROR: Local view of the cards in your hand has diverged from the server. This is a bug.")
				}

			case CMD_NOTIFY_GAME_JOINED:
				var cmd NotifyGameJoinedCommand
				SerialiseNotifyGameJoinedCommand(cmdContainer.payload, &cmd, true)

				if inGame {
					// A new player (that isn't us) has joined the game
					newPlayerCount := len(cmd.playerIds)
					for i := 0; i < newPlayerCount; i++ {
						newPlayer := NewPlayerState(cmd.playerIds[i], cmd.playerNames[i], &game)
						game.AddPlayer(&newPlayer)
						fmt.Printf("%s has joined the game\n", newPlayer.Name)
					}

				} else {
					// We just joined a game, set it up
					spec, err := NewSpec(cmd.specData)
					if err != nil {
						fmt.Printf("Failed to create local game tracker from spec: %s\n", err)
						shouldQuit = true
						break
					}
					game = CreateGameFromSpec(spec)
					game.Id = cmd.gameId
					game.Deck = make([]uint16, int(cmd.deckSize))
					for i := 0; i < len(game.Deck); i++ {
						game.Deck[i] = CARD_ID_ANY
					}

					game.Players = make([]*PlayerState, len(cmd.playerIds))
					for i := 0; i < len(cmd.playerIds); i++ {
						player := PlayerState{
							cmd.playerIds[i],
							nil,
							cmd.playerNames[i],
							cmd.playerHands[i],
							&game,
						}
						if player.Id == localPlayerId {
							localPlayer = &player
						}
						game.Players[i] = &player
					}
					fmt.Printf("Successfully joined a game. Your friends can join using the game ID: %d. Type 'help' to get a list of in-game commands.\n", game.Id)
				}
				inGame = true

			case CMD_NOTIFY_INPUT_ERROR:
				var cmd NotifyInputErrorCommand
				SerialiseNotifyInputErrorCommand(cmdContainer.payload, &cmd, true)
				switch cmd.errorId {
				case ERROR_INVALID_CMD_ID:
					fmt.Printf("ERROR: Unsupported command ID %d\n", cmd.cmdId)
				case ERROR_INVALID_GAME_ID:
					fmt.Printf("ERROR: Invalid game ID\n")
				case ERROR_INVALID_PLAYER_ID:
					fmt.Printf("ERROR: Invalid player ID\n")
				case ERROR_INVALID_DECK_ID:
					fmt.Printf("ERROR: Invalid deck ID\n")
				case ERROR_INVALID_CARD_ID:
					fmt.Printf("ERROR: Invalid card ID\n")
				case ERROR_INVALID_PLAYER_NAME:
					fmt.Printf("ERROR: Invalid player name. All players must have distinct names and cannot share a name with a card\n") // TODO: Redirect to docs for name specifications? This doesn't mention the name requirements (max length, no whitespace)
				case ERROR_SERVER_FULL:
					fmt.Printf("ERROR: The server you are trying to connect to is full.\n") // TODO: Print instructions for hosting your own or contact details or whatever
				case ERROR_INVALID_DATA:
					switch cmd.cmdId {
					case CMD_GAME_CREATE:
						fmt.Printf("ERROR: Invalid specification provided for the 'create' command\n")
					case CMD_CARD_PUTBACK:
						fmt.Printf("ERROR: Invalid depth in the deck for 'putback' command\n")
					case CMD_GAME_JOIN:
						fmt.Printf("ERROR: Failed to join the game, there may already be a player named '%s'. Please try again with a different username.\n", localPlayer.Name)
					}
				}

			case CMD_NOTIFY_PLAYER_ACTION:
				var cmd NotifyPlayerActionCommand
				SerialiseNotifyPlayerActionCommand(cmdContainer.payload, &cmd, true)
				srcPlayerIndex := game.FindPlayer(cmd.playerId)
				if srcPlayerIndex < 0 {
					fmt.Printf("Received an action notification for unrecognised player ID: %d. Ignoring...\n", cmd.playerId)
					break
				}
				srcPlayerName := game.Players[srcPlayerIndex].Name
				if cmd.playerId == localPlayer.Id {
					srcPlayerName = "You"
				}

				targetPlayerName := "Everyone"
				if (cmd.targetPlayerId != PLAYER_ID_NONE) && (cmd.targetPlayerId != PLAYER_ID_ALL) {
					targetPlayerIndex := game.FindPlayer(cmd.targetPlayerId)
					if targetPlayerIndex < 0 {
						fmt.Printf("Received an action notification targetting an unknown player ID: %d. Ignoring...\n", cmd.targetPlayerId)
						break
					}
					if cmd.targetPlayerId == localPlayer.Id {
						targetPlayerName = "You"
					} else {
						targetPlayerName = game.Players[targetPlayerIndex].Name
					}
				}

				faceDownCardCount := 0
				cardList := ""
				for index, newCardId := range cmd.targetCardIds {
					if index > 0 {
						cardList += ", "
					}
					cardList += game.spec.CardName(newCardId)
					if newCardId == CARD_ID_ANY {
						faceDownCardCount++
					}
				}

				switch cmd.cmdId {
				case CMD_CARD_DRAW:
					if cmd.playerId == localPlayer.Id {
						if len(cmd.targetCardIds) == 0 {
							fmt.Printf("No cards left to draw!\n")
						} else {
							for _, cardId := range cmd.targetCardIds {
								if (cardId != CARD_ID_ANY) && (cardId != CARD_ID_ALL) && (cardId != CARD_ID_NONE) {
									localPlayer.Draw(cardId)
								}
							}
							fmt.Printf("You drew: %s. You now have the following cards in your hand:\n", cardList)
							for _, cardId := range localPlayer.Hand {
								fmt.Printf("  - %s\n", game.spec.CardName(cardId))
							}
						}
					} else {
						if len(cmd.targetCardIds) == 0 {
							fmt.Printf("%s tried to draw a card, but there were no cards left!\n", srcPlayerName)
						} else {
							if faceDownCardCount == 0 {
								fmt.Printf("%s drew: %s\n", srcPlayerName, cardList)
							} else {
								if len(cmd.targetCardIds) == 1 {
									fmt.Printf("%s drew a card\n", srcPlayerName)
								} else {
									fmt.Printf("%s drew %d cards\n", srcPlayerName, len(cmd.targetCardIds))
								}
							}
						}
					}

				case CMD_CARD_DISCARD:
					if cmd.playerId == localPlayer.Id {
						for _, cardId := range cmd.targetCardIds {
							cardIndex := game.FindCard(localPlayer, cardId)
							if cardIndex < 0 {
								fmt.Printf("ERROR: Received a discard notification for a card (%d) that is not in your hand!\n", cardId)
								break
							}
							localPlayer.Discard(cardIndex)
						}
					}
					if faceDownCardCount == 0 {
						fmt.Printf("%s discarded %s from their hand\n", srcPlayerName, cardList)
					} else {
						fmt.Printf("%s discarded %d cards from their hand\n", srcPlayerName, len(cmd.targetCardIds))
					}

				case CMD_CARD_GIVE:
					if cmd.targetPlayerId == localPlayer.Id {
						if len(cmd.targetCardIds) > 0 {
							for _, cardId := range cmd.targetCardIds {
								if (cardId != CARD_ID_ANY) && (cardId != CARD_ID_ALL) && (cardId != CARD_ID_NONE) {
									localPlayer.Draw(cardId)
								}
							}
							fmt.Printf("%s gave you %s from their hand. You now have the following cards in your hand:\n", srcPlayerName, cardList)
							for _, cardId := range localPlayer.Hand {
								fmt.Printf("  %s\n", game.spec.CardName(cardId))
							}
						}
						break
					}
					if cmd.playerId == localPlayer.Id {
						for _, cardId := range cmd.targetCardIds {
							cardIndex := game.FindCard(localPlayer, cardId)
							if cardIndex < 0 {
								fmt.Printf("ERROR: Received a give notification for a card (%d) that is not in your hand!\n", cardId)
								break
							}
							localPlayer.Discard(cardIndex)
						}
						fmt.Printf("You gave %s from your hand to %s\n", cardList, targetPlayerName)
						break
					}
					if faceDownCardCount == 0 {
						fmt.Printf("%s gave %s from their hand to %s\n", srcPlayerName, cardList, targetPlayerName)
					} else {
						fmt.Printf("%s gave a card from their hand to %s\n", srcPlayerName, targetPlayerName)
					}

				case CMD_CARD_PUTBACK:
					if cmd.playerId == localPlayer.Id {
						for _, cardId := range cmd.targetCardIds {
							cardIndex := game.FindCard(localPlayer, cardId)
							if cardIndex < 0 {
								fmt.Printf("ERROR: Received a discard notification for a card (%d) that is not in your hand!\n", cardId)
								break
							}
							localPlayer.Discard(cardIndex)
						}
					}
					if faceDownCardCount == 0 {
						fmt.Printf("%s put a %s from their hand back into the deck\n", srcPlayerName, cardList)
					} else {
						fmt.Printf("%s put a card from their hand back into the deck\n", srcPlayerName)
					}

				case CMD_CARD_SHOW:
					if faceDownCardCount == 0 {
						fmt.Printf("%s showed the following cards to %s: %s\n", srcPlayerName, targetPlayerName, cardList)
					} else {
						fmt.Printf("%s showed %d cards to %s\n", srcPlayerName, len(cmd.targetCardIds), targetPlayerName)
					}

				case CMD_DECK_PEEK:
					if faceDownCardCount == 0 {
						peekedCardList := ""
						if len(cmd.targetCardIds) > 0 {
							peekedCardList = fmt.Sprintf("  - %s  <-- Top of the deck\n", game.spec.CardName(cmd.targetCardIds[0]))
							for _, peekedCardId := range cmd.targetCardIds[1:] {
								peekedCardList += fmt.Sprintf("  - %s\n", game.spec.CardName(peekedCardId))
							}
						}
						fmt.Printf("%s looked at the top %d cards in the deck and ordered from top to bottom they are:\n%s", srcPlayerName, len(cmd.targetCardIds), peekedCardList)
					} else {
						fmt.Printf("%s looked at the top %d cards in the deck\n", srcPlayerName, len(cmd.targetCardIds))
					}

				case CMD_DECK_SHUFFLE:
					fmt.Printf("%s shuffled the deck\n", srcPlayerName)

				case CMD_GAME_LEAVE:
					fmt.Printf("%s left the game\n", srcPlayerName)
					game.RemovePlayer(game.Players[srcPlayerIndex])
					if cmd.playerId == localPlayer.Id {
						inGame = false
						game = GameState{}
					}

				default:
					fmt.Printf("Received unexpected command %d, ignoring...\n", cmd.cmdId)
				}

			case CMD_NOTIFY_SERVER_SHUTDOWN:
				fmt.Printf("Server is shutting down...\n")

			default:
				fmt.Printf("ERROR: Received unrecognised or unsupported command %d from server, disconnecting...\n", cmdId)
				quitChan <- true
				return
			}

		case <-quitChan:
			shouldQuit = true
		}

		if shouldQuit {
			break
		}
	}

	keepAliveTimer.Stop()
	conn.Close()
}

func sendCommandBuffer(buffer []byte, conn net.Conn) error {
	bytesWritten := 0
	for bytesWritten < len(buffer) {
		newBytesWritten, err := conn.Write(buffer)
		if err != nil {
			return err
		}
		bytesWritten += newBytesWritten
	}

	return nil
}

func parseInputUint16(inputTokens []string) (uint16, error) {
	if len(inputTokens) == 0 {
		return 0, ErrInsufficientArguments
	}

	fullValue, err := strconv.ParseUint(inputTokens[0], 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(fullValue), err
}

func parseInputUint64(inputTokens []string) (uint64, error) {
	if len(inputTokens) == 0 {
		return 0, ErrInsufficientArguments
	}

	fullValue, err := strconv.ParseUint(inputTokens[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return fullValue, err
}

func parsePlayerId(game *GameState, unusedArgs *[]string) (uint64, error) {
	for argIndex, arg := range *unusedArgs {
		if len(arg) == 0 {
			continue
		}

		lowerArg := strings.ToLower(arg)
		firstMatchedPlayerId := uint64(0)
		matchedPlayerNames := make([]string, 0)
		for _, player := range game.Players {
			lowerPlayer := strings.ToLower(player.Name)
			if strings.HasPrefix(lowerPlayer, lowerArg) {
				if len(matchedPlayerNames) == 0 {
					firstMatchedPlayerId = player.Id
				}

				if !stringInSlice(lowerPlayer, matchedPlayerNames) {
					matchedPlayerNames = append(matchedPlayerNames, lowerPlayer)
				}
			}
		}

		if len(matchedPlayerNames) == 1 {
			if argIndex < len(*unusedArgs)-1 {
				copy((*unusedArgs)[argIndex:], (*unusedArgs)[argIndex+1:])
			}
			*unusedArgs = (*unusedArgs)[:len(*unusedArgs)-1]
			return firstMatchedPlayerId, nil

		} else if len(matchedPlayerNames) > 1 {
			errMsg := fmt.Sprintf("Argument '%s' is ambiguous and could refer to any of: %s. Please try again with a more specific argument",
				arg, strings.Join(matchedPlayerNames, ", "))
			return 0, errors.New(errMsg)

		} else if lowerArg == "anyplayer" {
			return PLAYER_ID_ANY, nil
		} else if lowerArg == "allplayers" {
			return PLAYER_ID_ALL, nil
		}
	}

	return 0, errors.New("No valid arguments")
}

func parseCardIdFromHand(game *GameState, player *PlayerState, unusedArgs *[]string) (uint16, error) {
	for argIndex, arg := range *unusedArgs {
		if len(arg) == 0 {
			continue
		}

		lowerArg := strings.ToLower(arg)
		firstMatchedCardId := uint16(0)
		matchedCardNames := make([]string, 0)
		for _, cardId := range player.Hand {
			cardName := game.spec.CardName(cardId)
			lowerCard := strings.ToLower(cardName)
			if lowerCard == lowerArg {
				return uint16(cardId), nil
			}
			if strings.HasPrefix(lowerCard, lowerArg) {
				if len(matchedCardNames) == 0 {
					firstMatchedCardId = uint16(cardId)
				}

				if !stringInSlice(lowerCard, matchedCardNames) {
					matchedCardNames = append(matchedCardNames, lowerCard)
				}
			}
		}

		if len(matchedCardNames) == 1 {
			if argIndex < len(*unusedArgs)-1 {
				copy((*unusedArgs)[argIndex:], (*unusedArgs)[argIndex+1:])
			}
			*unusedArgs = (*unusedArgs)[:len(*unusedArgs)-1]
			return firstMatchedCardId, nil

		} else if len(matchedCardNames) > 1 {
			errMsg := fmt.Sprintf("Argument '%s' is ambiguous and could refer to any of: %s. Please try again with a more specific argument",
				arg, strings.Join(matchedCardNames, ", "))
			return 0, errors.New(errMsg)

		} else if lowerArg == "anycard" {
			return CARD_ID_ANY, nil
		} else if lowerArg == "allcards" {
			return CARD_ID_ALL, nil
		}
	}

	return 0, errors.New("No cards were found that matched any given arguments")
}

func stringInSlice(str string, slice []string) bool {
	for _, sliceStr := range slice {
		if str == sliceStr {
			return true
		}
	}
	return false
}
