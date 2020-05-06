package main

import (
	"errors"
	"io"
	"math"
	"net"
)

/*
Serialisation notes:
- All multi-byte integers are transmitted in little-endian format
*/

const (
	PROTOCOL_MAGIC_NUMBER = 0x342F // Just some specific random bytes at the beginning of the connection to help verify that the remote client knows about the protocol
	PROTOCOL_ID           = 0x0001 // Incremented by one for every backwards-incompatible change to the protocol/API
)

const (
	// Connection management
	CMD_UNKNOWN byte = iota
	CMD_KEEPALIVE
	CMD_HANDSHAKE
	CMD_HANDSHAKE_RESPONSE
	CMD_DISCONNECT

	// Info sync
	CMD_INFO_PLAYERS
	CMD_INFO_DECKS
	CMD_INFO_CARDS
	CMD_INFO_PLAYERS_RESPONSE
	CMD_INFO_DECKS_RESPONSE
	CMD_INFO_CARDS_RESPONSE

	// Card actions
	CMD_CARD_DRAW
	CMD_CARD_SHOW
	CMD_CARD_PUTBACK
	CMD_CARD_DISCARD
	CMD_CARD_GIVE

	// Deck actions
	CMD_DECK_PEEK
	CMD_DECK_SHUFFLE

	// Game Actions
	CMD_GAME_CREATE
	CMD_GAME_JOIN
	CMD_GAME_LEAVE

	// Notifications
	CMD_NOTIFY_PLAYER_ACTION
	CMD_NOTIFY_GAME_JOINED
	CMD_NOTIFY_SERVER_SHUTDOWN
	CMD_NOTIFY_INPUT_ERROR

	NUM_CMDS
)

var ErrInvalidCommandId = errors.New("Invalid command ID")
var ErrInvalidPlayerId = errors.New("Invalid player ID")
var ErrInvalidHeader = errors.New("Invalid command header")
var ErrInvalidLength = errors.New("Invalid command length")
var ErrInvalidData = errors.New("Invalid command data")
var ErrHeaderMismatch = errors.New("Header deserialisation mismatch")

var ErrIncompleteWrite = errors.New("Failed to write complete packet")

const MaxPlayerNameLength = 64

const (
	ERROR_INVALID_CMD_ID byte = iota
	ERROR_INVALID_GAME_ID
	ERROR_INVALID_PLAYER_ID
	ERROR_INVALID_DECK_ID
	ERROR_INVALID_CARD_ID

	ERROR_INVALID_PLAYER_NAME
	ERROR_INVALID_DATA

	ERROR_SERVER_FULL
)

const (
	PLAYER_ID_ALL  = math.MaxUint64
	PLAYER_ID_ANY  = math.MaxUint64 - 1
	PLAYER_ID_NONE = math.MaxUint64 - 2
	PLAYER_ID_MAX  = math.MaxUint64 - 3

	DECK_ID_ALL  = math.MaxUint16
	DECK_ID_ANY  = math.MaxUint16 - 1
	DECK_ID_NONE = math.MaxUint16 - 2
	DECK_ID_MAX  = math.MaxUint16 - 3

	CARD_ID_ALL  = math.MaxUint16
	CARD_ID_ANY  = math.MaxUint16 - 1
	CARD_ID_NONE = math.MaxUint16 - 2
	CARD_ID_MAX  = math.MaxUint16 - 3
)

// Command Header
const CommandHeaderLength = 3

type CommandHeader struct {
	id  byte
	len uint16
}

func SerialiseCommandHeader(buffer []byte, header *CommandHeader, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseByte(&header.id)
	ctx.serialiseUint16(&header.len)
	return ctx.complete()
}

func WriteCommandHeader(cmdId byte, cmdLen uint16) ([]byte, uint16) {
	buffer := make([]byte, CommandHeaderLength+cmdLen)
	header := CommandHeader{
		cmdId,
		cmdLen,
	}
	SerialiseCommandHeader(buffer, &header, false)
	return buffer, CommandHeaderLength
}

type CommandContainer struct {
	header  CommandHeader
	payload []byte
}

func commandLengthLimits(id byte) (uint16, uint16) {
	var minCmdLen uint16 = 0
	var maxCmdLen uint16 = 0
	switch id {
	case CMD_HANDSHAKE:
		minCmdLen = MinHandshakeCommandLength
		maxCmdLen = MaxHandshakeCommandLength
	case CMD_HANDSHAKE_RESPONSE:
		minCmdLen = HandshakeResponseCommandLength
		maxCmdLen = HandshakeResponseCommandLength
	case CMD_INFO_PLAYERS_RESPONSE:
		minCmdLen = MinPlayerInfoResponseCommandLength
		maxCmdLen = MaxPlayerInfoResponseCommandLength
	case CMD_INFO_DECKS_RESPONSE:
		minCmdLen = MinDeckInfoResponseCommandLength
		maxCmdLen = MaxDeckInfoResponseCommandLength
	case CMD_INFO_CARDS_RESPONSE:
		minCmdLen = MinCardInfoResponseCommandLength
		maxCmdLen = MaxCardInfoResponseCommandLength
	case CMD_CARD_DRAW:
		minCmdLen = CardDrawCommandLength
		maxCmdLen = CardDrawCommandLength
	case CMD_CARD_SHOW:
		minCmdLen = CardShowCommandLength
		maxCmdLen = CardShowCommandLength
	case CMD_CARD_PUTBACK:
		minCmdLen = CardPutbackCommandLength
		maxCmdLen = CardPutbackCommandLength
	case CMD_CARD_DISCARD:
		minCmdLen = CardDiscardCommandLength
		maxCmdLen = CardDiscardCommandLength
	case CMD_CARD_GIVE:
		minCmdLen = CardGiveCommandLength
		maxCmdLen = CardGiveCommandLength
	case CMD_DECK_PEEK:
		minCmdLen = DeckPeekCommandLength
		maxCmdLen = DeckPeekCommandLength
	case CMD_DECK_SHUFFLE:
		minCmdLen = DeckShuffleCommandLength
		maxCmdLen = DeckShuffleCommandLength
	case CMD_GAME_CREATE:
		minCmdLen = MinGameCreateCommandLength
		maxCmdLen = MaxGameCreateCommandLength
	case CMD_GAME_JOIN:
		minCmdLen = GameJoinCommandLength
		maxCmdLen = GameJoinCommandLength
	case CMD_NOTIFY_PLAYER_ACTION:
		minCmdLen = MinNotifyPlayerActionCommandLength
		maxCmdLen = MaxNotifyPlayerActionCommandLength
	case CMD_NOTIFY_GAME_JOINED:
		minCmdLen = MinNotifyGameJoinedCommandLength
		maxCmdLen = MaxNotifyGameJoinedCommandLength
	case CMD_NOTIFY_INPUT_ERROR:
		minCmdLen = NotifyInputErrorCommandLength
		maxCmdLen = NotifyInputErrorCommandLength
	}
	return minCmdLen, maxCmdLen
}

func ValidateCommandHeader(header CommandHeader) error {
	if header.id >= NUM_CMDS {
		return ErrInvalidHeader
	}

	// NOTE: Its important for us to validate the packet length here because otherwise somebody can
	//		 send us a packet that claims to be very long but not send us any data and we'll sit
	//		 trying to read it all from the socket
	minCmdLen, maxCmdLen := commandLengthLimits(header.id)
	if (header.len < minCmdLen) || (header.len > maxCmdLen) {
		panic("Invalid length")
		//return ErrInvalidLength
	}
	return nil
}

const MinHandshakeCommandLength = 6
const MaxHandshakeCommandLength = 6 + MaxPlayerNameLength

type HandshakeCommand struct {
	magicNumber uint16
	protocolId  uint16
	localName   string
}

func (hc *HandshakeCommand) CommandLength() uint16 {
	return 6 + uint16(len(hc.localName))
}

func SerialiseHandshakeCommand(buffer []byte, cmd *HandshakeCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.magicNumber)
	ctx.serialiseUint16(&cmd.protocolId)
	ctx.serialiseString(&cmd.localName)
	return ctx.complete()
}

const HandshakeResponseCommandLength = 8

type HandshakeResponseCommand struct {
	playerId uint64
}

func SerialiseHandshakeResponseCommand(buffer []byte, cmd *HandshakeResponseCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint64(&cmd.playerId)
	return ctx.complete()
}

const MinPlayerInfoResponseCommandLength = 6
const MaxPlayerInfoResponseCommandLength = math.MaxUint16

type PlayerInfoResponseCommand struct {
	ids       []uint64
	names     []string
	handSizes []uint16
}

func (cmd *PlayerInfoResponseCommand) CommandLength() int {
	result := MinPlayerInfoResponseCommandLength + (10 * len(cmd.ids))
	for _, name := range cmd.names {
		result += 2 + len(name)
	}
	return result
}

func SerialisePlayerInfoResponseCommand(buffer []byte, cmd *PlayerInfoResponseCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint64Slice(&cmd.ids)
	ctx.serialiseStringSlice(&cmd.names)
	ctx.serialiseUint16Slice(&cmd.handSizes)
	ctx.assert(len(cmd.ids) == len(cmd.names))
	ctx.assert(len(cmd.ids) == len(cmd.handSizes))
	return ctx.complete()
}

const MinDeckInfoResponseCommandLength = 4
const MaxDeckInfoResponseCommandLength = math.MaxUint16

type DeckInfoResponseCommand struct {
	ids        []uint16
	cardCounts []uint16
}

func (cmd *DeckInfoResponseCommand) CommandLength() int {
	return MinDeckInfoResponseCommandLength + (4 * len(cmd.ids))
}

func SerialiseDeckInfoResponseCommand(buffer []byte, cmd *DeckInfoResponseCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16Slice(&cmd.ids)
	ctx.serialiseUint16Slice(&cmd.cardCounts)
	return ctx.complete()
}

const MinCardInfoResponseCommandLength = 2
const MaxCardInfoResponseCommandLength = math.MaxUint16

type CardInfoResponseCommand struct {
	ids []uint16
}

func (cmd *CardInfoResponseCommand) CommandLength() int {
	return MinCardInfoResponseCommandLength + (2 * len(cmd.ids))
}

func SerialiseCardInfoResponseCommand(buffer []byte, cmd *CardInfoResponseCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16Slice(&cmd.ids)
	return ctx.complete()
}

const CardDrawCommandLength = 5

type CardDrawCommand struct {
	deckId uint16
	count  uint16
	faceUp bool
}

func SerialiseCardDrawCommand(buffer []byte, cmd *CardDrawCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.deckId)
	ctx.serialiseUint16(&cmd.count)
	ctx.serialiseBool(&cmd.faceUp)
	return ctx.complete()
}

const CardShowCommandLength = 10

type CardShowCommand struct {
	cardId   uint16
	playerId uint64
}

func SerialiseCardShowCommand(buffer []byte, cmd *CardShowCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.cardId)
	ctx.serialiseUint64(&cmd.playerId)
	return ctx.complete()
}

const CardPutbackCommandLength = 6

type CardPutbackCommand struct {
	cardId       uint16
	deckId       uint16
	cardsFromTop uint16
}

func SerialiseCardPutbackCommand(buffer []byte, cmd *CardPutbackCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.cardId)
	ctx.serialiseUint16(&cmd.deckId)
	ctx.serialiseUint16(&cmd.cardsFromTop)
	return ctx.complete()
}

const CardDiscardCommandLength = 3

type CardDiscardCommand struct {
	cardId uint16
	faceUp bool
}

func SerialiseCardDiscardCommand(buffer []byte, cmd *CardDiscardCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.cardId)
	ctx.serialiseBool(&cmd.faceUp)
	return ctx.complete()
}

const CardGiveCommandLength = 11

type CardGiveCommand struct {
	cardId   uint16
	playerId uint64
	faceUp   bool
}

func SerialiseCardGiveCommand(buffer []byte, cmd *CardGiveCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.cardId)
	ctx.serialiseUint64(&cmd.playerId)
	ctx.serialiseBool(&cmd.faceUp)
	return ctx.complete()
}

const DeckPeekCommandLength = 5

type DeckPeekCommand struct {
	deckId uint16
	count  uint16
	public bool
}

func SerialiseDeckPeekCommand(buffer []byte, cmd *DeckPeekCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.deckId)
	ctx.serialiseUint16(&cmd.count)
	ctx.serialiseBool(&cmd.public)
	return ctx.complete()
}

const DeckShuffleCommandLength = 2

type DeckShuffleCommand struct {
	deckId uint16
}

func SerialiseDeckShuffleCommand(buffer []byte, cmd *DeckShuffleCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint16(&cmd.deckId)
	return ctx.complete()
}

const MinGameCreateCommandLength = 2
const MaxGameCreateCommandLength = math.MaxUint16

var MaxGameCreateSpecDataLength = MaxGameCreateCommandLength - GameCreateCommandLength(0)

func GameCreateCommandLength(specDataLength int) int {
	return MinGameCreateCommandLength + specDataLength
}

type GameCreateCommand struct {
	specData []byte
}

func SerialiseGameCreateCommand(buffer []byte, cmd *GameCreateCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseByteSlice(&cmd.specData)
	return ctx.complete()
}

const GameJoinCommandLength = 8

type GameJoinCommand struct {
	gameId uint64
}

func SerialiseGameJoinCommand(buffer []byte, cmd *GameJoinCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint64(&cmd.gameId)
	return ctx.complete()
}

type GameLeaveCommand struct{}

const MinNotifyPlayerActionCommandLength = 21
const MaxNotifyPlayerActionCommandLength = math.MaxUint16

type NotifyPlayerActionCommand struct {
	playerId       uint64
	cmdId          byte
	targetDeckId   uint16
	targetPlayerId uint64
	targetCardIds  []uint16
}

func (cmd *NotifyPlayerActionCommand) CommandLength() int {
	return MinNotifyPlayerActionCommandLength + (2 * len(cmd.targetCardIds))
}

func NewPlayerActionNotify(playerId uint64, cmdId byte, targetDeckId uint16, targetPlayerId uint64, targetCardIds []uint16) NotifyPlayerActionCommand {
	return NotifyPlayerActionCommand{
		playerId,
		cmdId,
		targetDeckId,
		targetPlayerId,
		targetCardIds,
	}
}

func SerialiseNotifyPlayerActionCommand(buffer []byte, cmd *NotifyPlayerActionCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint64(&cmd.playerId)
	ctx.serialiseByte(&cmd.cmdId)
	ctx.serialiseUint16(&cmd.targetDeckId)
	ctx.serialiseUint64(&cmd.targetPlayerId)
	ctx.serialiseUint16Slice(&cmd.targetCardIds)
	return ctx.complete()
}

const MinNotifyGameJoinedCommandLength = 18
const MaxNotifyGameJoinedCommandLength = math.MaxUint16

type NotifyGameJoinedCommand struct {
	gameId      uint64
	specData    []byte
	playerIds   []uint64
	playerNames []string
	playerHands [][]uint16
	deckSize    uint16
}

func (cmd *NotifyGameJoinedCommand) CommandLength() int {
	result := 8
	result += 2 + len(cmd.specData)
	result += 2 + 8*len(cmd.playerIds)
	result += 2
	for _, str := range cmd.playerNames {
		result += 2 + len(str)
	}
	result += 2
	for _, hand := range cmd.playerHands {
		result += 2 + 2*len(hand)
	}
	result += 2
	return result
}

func SerialiseNotifyGameJoinedCommand(buffer []byte, cmd *NotifyGameJoinedCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseUint64(&cmd.gameId)
	ctx.serialiseByteSlice(&cmd.specData)
	ctx.serialiseUint64Slice(&cmd.playerIds)
	ctx.serialiseStringSlice(&cmd.playerNames)
	ctx.serialiseUint16SliceSlice(&cmd.playerHands)
	ctx.serialiseUint16(&cmd.deckSize)
	return ctx.complete()
}

type NotifyServerShutdownCommand struct{}

const NotifyInputErrorCommandLength = 2

type NotifyInputErrorCommand struct {
	cmdId   byte
	errorId byte
}

func SerialiseNotifyInputErrorCommand(buffer []byte, cmd *NotifyInputErrorCommand, isReading bool) error {
	ctx := newSerialisation(buffer, isReading)
	ctx.serialiseByte(&cmd.cmdId)
	ctx.serialiseByte(&cmd.errorId)
	return ctx.complete()
}

func ReadExactlyNBytes(reader io.Reader, n uint16) ([]byte, error) {
	bytes := make([]byte, n)
	bytesRead := 0
	for bytesRead < int(n) {
		newBytes, err := reader.Read(bytes[bytesRead:])
		if (newBytes == 0) && (err != nil) {
			return nil, err
		}
		bytesRead += newBytes
	}

	return bytes, nil
}

func SendCommandBufferTo(conn net.Conn, buffer []byte) error {
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
