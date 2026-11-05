package ws

import (
	"encoding/json"
	"errors"
	"math/big"
)

// MidpointUpdate contains the exact midpoint of a book's first bid and ask.
// Timestamp is the original millisecond timestamp, without numeric conversion.
type MidpointUpdate struct {
	AssetID   string
	Market    string
	Timestamp json.Number
	Midpoint  *big.Rat
}

// Midpoint calculates (best bid + best ask) / 2 for a book event, using exact
// rational arithmetic. It returns ok=false for other events or empty book sides.
func (m Message) Midpoint() (update MidpointUpdate, ok bool, err error) {
	if m.EventType != "book" {
		return update, false, nil
	}
	var book struct {
		AssetID   string      `json:"asset_id"`
		Market    string      `json:"market"`
		Timestamp json.Number `json:"timestamp"`
		Bids      []struct {
			Price json.Number `json:"price"`
		} `json:"bids"`
		Asks []struct {
			Price json.Number `json:"price"`
		} `json:"asks"`
	}
	if err := m.Decode(&book); err != nil {
		return update, false, err
	}
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return update, false, nil
	}
	bid, valid := new(big.Rat).SetString(book.Bids[0].Price.String())
	if !valid {
		return update, false, errors.New("invalid best bid price")
	}
	ask, valid := new(big.Rat).SetString(book.Asks[0].Price.String())
	if !valid {
		return update, false, errors.New("invalid best ask price")
	}
	midpoint := new(big.Rat).Quo(new(big.Rat).Add(bid, ask), big.NewRat(2, 1))
	return MidpointUpdate{AssetID: book.AssetID, Market: book.Market, Timestamp: book.Timestamp, Midpoint: midpoint}, true, nil
}
