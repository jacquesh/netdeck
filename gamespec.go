package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type GameSpecification struct {
	Deck []string
}

func NewSpec(dataStr string) (*GameSpecification, error) {
	data, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		return nil, errors.New("Specification data is corrupt")
	}

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

func SerializeSpecFromName(specName string) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", errors.New("Failed to get the directory for local specification files")
	}

	dir, err := os.Open(workingDir)
	if err != nil {
		return "", errors.New("Failed to open the directory for local specification files")
	}

	workingDirEntries, err := dir.Readdirnames(0)
	if err != nil {
		return "", errors.New("Failed to list local specification files")
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
		return "", errors.New("Specification '" + specName + "' not found in the local specification files")
	}

	specFilePath := filepath.Join(workingDir, specFileName)
	specData, err := ioutil.ReadFile(specFilePath)
	if err != nil {
		return "", errors.New("Failed to read local specification file '" + specFilePath + "'")
	}

	return SerializeSpecFromBytes(specData), nil
}

func SerializeSpecFromBytes(specBytes []byte) string {
	var gzipBuffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuffer)
	gzipWriter.Write(specBytes)
	gzipWriter.Close()

	result := base64.StdEncoding.EncodeToString(gzipBuffer.Bytes())
	return result
}

func DefaultSerializedGameSpec() string {
	specStr := `---
deck:
- Ace Of Spades
- 1 Of Spades
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
- 1 Of Clubs
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
- 1 Of Diamonds
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
- 1 Of Hearts
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
	return SerializeSpecFromBytes([]byte(specStr))
}
