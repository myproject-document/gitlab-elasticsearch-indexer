package indexer

import (
	"fmt"

	logkit "gitlab.com/gitlab-org/labkit/log"

	"gitlab.com/lupine/icu"
)

type Encoder struct {
	detector  *icu.CharsetDetector
	converter *icu.CharsetConverter
}

func NewEncoder(limitFileSize int64) *Encoder {
	encoder := &Encoder{}
	detector, err := icu.NewCharsetDetector()
	if err != nil {
		panic(err)
	}

	encoder.detector = detector
	encoder.converter = icu.NewCharsetConverter(int(limitFileSize))

	return encoder
}

func (e *Encoder) tryEncodeString(s string) string {
	encoded, err := e.encodeString(s)
	if err != nil {
		logkit.WithError(err).Error("Encode string failed")
		return s // TODO: Run it through the UTF-8 replacement encoder
	}

	return encoded
}

func (e *Encoder) tryEncodeBytes(b []byte) string {
	encoded, err := e.encodeBytes(b)
	if err != nil {
		logkit.WithError(err).Error("Encode bytes failed")
		s := string(b)
		return s // TODO: Run it through the UTF-8 replacement encoder
	}

	return encoded
}

func (e *Encoder) encodeString(s string) (string, error) {
	return e.encodeBytes([]byte(s))
}

// encodeString converts a string from an arbitrary encoding to UTF-8
func (e *Encoder) encodeBytes(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}

	matches, err := e.detector.GuessCharset(b)
	if err != nil {
		return "", fmt.Errorf("Couldn't guess charset: %s", err)
	}

	// Try encoding for each match, returning the first that succeeds
	for _, match := range matches {
		utf8, err := e.converter.ConvertToUtf8(b, match.Charset)
		if err == nil {
			return string(utf8), nil
		}
	}

	// `detector.GuessCharset` may return err == nil && len(matches) == 0
	bestGuess := "unknown"
	if len(matches) > 0 {
		bestGuess = matches[0].Charset
	}

	return "", fmt.Errorf("Failed to convert from %s to UTF-8", bestGuess)
}
