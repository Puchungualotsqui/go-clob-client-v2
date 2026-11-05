package ws

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// foldedJSONKey follows Unicode simple folding, as encoding/json does when
// matching struct fields case-insensitively.
func foldedJSONKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		min := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < min {
				min = next
			}
		}
		b.WriteRune(min)
	}
	return b.String()
}

var strictReservedKeys = func() map[string]string {
	keys := make(map[string]string)
	for _, key := range []string{
		"event_type", "asset_id", "asset_ids", "assets_ids", "price_changes",
		"id", "market", "timestamp", "price", "size", "side", "hash", "bids", "asks",
		"best_bid", "best_ask", "spread", "old_tick_size", "new_tick_size", "fee_rate_bps",
		"question", "slug", "description", "outcomes", "event_message", "ticker", "title",
		"winning_asset_id", "winning_outcome",
	} {
		keys[foldedJSONKey(key)] = key
	}
	return keys
}()

// json.Valid accepts unpaired UTF-16 surrogate escapes and encoding/json replaces
// them with U+FFFD. Reject them rather than silently changing keys or values.
func validSurrogateEscapes(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if !inString {
			if data[i] == '"' {
				inString = true
			}
			continue
		}
		if data[i] == '"' {
			inString = false
			continue
		}
		if data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue
		}
		code := hexEscape(data[i+1 : i+5])
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low := hexEscape(data[i+3 : i+7])
			if low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

// hexEscape is called only after json.Valid has checked the escape syntax.
func hexEscape(digits []byte) uint16 {
	var value uint16
	for _, c := range digits {
		value <<= 4
		switch {
		case c >= '0' && c <= '9':
			value |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			value |= uint16(c-'a') + 10
		default:
			value |= uint16(c-'A') + 10
		}
	}
	return value
}

func validateStrictJSON(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("market message is not valid UTF-8")
	}
	if !json.Valid(data) {
		return errors.New("invalid market JSON")
	}
	if !validSurrogateEscapes(data) {
		return errors.New("market JSON contains an unpaired UTF-16 surrogate")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateStrictJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func validateStrictJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || key == "" || strings.IndexFunc(key, unicode.IsControl) >= 0 {
				return errors.New("invalid market JSON object key")
			}
			folded := foldedJSONKey(key)
			if _, exists := keys[folded]; exists {
				return errors.New("duplicate or case-variant market JSON object key")
			}
			keys[folded] = struct{}{}
			if canonical, reserved := strictReservedKeys[folded]; reserved && key != canonical {
				return fmt.Errorf("market JSON key must be spelled %s", canonical)
			}
			if err := validateStrictJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := validateStrictJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected market JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}
