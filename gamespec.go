package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type GameSpecification struct {
	Deck []string
}

func (gs *GameSpecification) CardName(cardId uint16) string {
	if (cardId == CARD_ID_ANY) || (cardId == CARD_ID_ALL) {
		return "<RANDOM-CARD>"
	} else if cardId == CARD_ID_NONE {
		return "<NO-CARD>"
	} else if int(cardId) >= len(gs.Deck) {
		return "<ERROR-UNKNOWN-CARD>"
	}
	return gs.Deck[cardId]
}

func NewSpec(data []byte) (*GameSpecification, error) {
	gzipDecoder, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("Specification data header is corrupt or not compressed")
	}

	decompressedData, err := ioutil.ReadAll(gzipDecoder)
	if err != nil {
		return nil, errors.New("Specification data is corrupt or not compressed")
	}

	var spec GameSpecification
	err = yaml.Unmarshal(decompressedData, &spec)
	if err != nil {
		return nil, errors.New("Specification data does not form a valid game specification")
	}

	return &spec, nil
}

func SerialiseSpecFromName(specName string) ([]byte, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, errors.New("Failed to get the directory for local specification files")
	}

	dir, err := os.Open(workingDir)
	if err != nil {
		return nil, errors.New("Failed to open the directory for local specification files")
	}

	workingDirEntries, err := dir.Readdirnames(0)
	if err != nil {
		return nil, errors.New("Failed to list local specification files")
	}

	specFileName := ""
	for _, filename := range workingDirEntries {
		ext := filepath.Ext(filename)
		if (ext != ".yml") && (ext != ".yaml") {
			continue
		}

		if filename == specName+ext {
			specFileName = filename
			break
		}
	}

	if len(specFileName) == 0 {
		return nil, errors.New("Specification '" + specName + "' not found in the local specification files")
	}

	specFilePath := filepath.Join(workingDir, specFileName)
	specData, err := ioutil.ReadFile(specFilePath)
	if err != nil {
		return nil, errors.New("Failed to read local specification file '" + specFilePath + "'")
	}

	return SerialiseSpecFromBytes(specData), nil
}

func SerialiseSpecFromSpec(spec *GameSpecification) ([]byte, error) {
	specData, err := yaml.Marshal(spec)
	return SerialiseSpecFromBytes(specData), err
}

func SerialiseSpecFromBytes(specBytes []byte) []byte {
	var gzipBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuffer)
	gzipWriter.Write(specBytes)
	gzipWriter.Close()
	return gzipBuffer.Bytes()
}

func DefaultSerializedGameSpec() []byte {
	specStr := `---
deck:
- Ace Of Spades
- 2 Of Spades
- 3 Of Spades
- 4 Of Spades
- 5 Of Spades
- 6 Of Spades
- 7 Of Spades
- 8 Of Spades
- 9 Of Spades
- 10 Of Spades
- Jack Of Spades
- Queen Of Spades
- King Of Spades
- Ace Of Clubs
- 2 Of Clubs
- 3 Of Clubs
- 4 Of Clubs
- 5 Of Clubs
- 6 Of Clubs
- 7 Of Clubs
- 8 Of Clubs
- 9 Of Clubs
- 10 Of Clubs
- Jack Of Clubs
- Queen Of Clubs
- King Of Clubs
- Ace Of Diamonds
- 2 Of Diamonds
- 3 Of Diamonds
- 4 Of Diamonds
- 5 Of Diamonds
- 6 Of Diamonds
- 7 Of Diamonds
- 8 Of Diamonds
- 9 Of Diamonds
- 10 Of Diamonds
- Jack Of Diamonds
- Queen Of Diamonds
- King Of Diamonds
- Ace Of Hearts
- 2 Of Hearts
- 3 Of Hearts
- 4 Of Hearts
- 5 Of Hearts
- 6 Of Hearts
- 7 Of Hearts
- 8 Of Hearts
- 9 Of Hearts
- 10 Of Hearts
- Jack Of Hearts
- Queen Of Hearts
- King Of Hearts
`
	return SerialiseSpecFromBytes([]byte(specStr))
}
