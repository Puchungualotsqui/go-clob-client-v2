package ws

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTypedEvents(t *testing.T) {
	tests := []struct {
		data string
		want any
	}{
		{`{"event_type":"book","asset_id":"123","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"1234567890","bids":[{"price":"0.123456789123456789","size":"100"}],"asks":[]}`, &BookUpdate{AssetID: "123", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", Timestamp: "1234567890", Bids: []OrderBookLevel{{Price: "0.123456789123456789", Size: "100"}}, Asks: []OrderBookLevel{}}},
		{`{"event_type":"price_change","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"2","price_changes":[{"asset_id":"123","price":"0.5","side":"BUY","best_bid":"0.4"}]}`, &PriceChange{Market: "0x0000000000000000000000000000000000000000000000000000000000000001", Timestamp: "2", PriceChanges: []PriceChangeBatchEntry{{AssetID: "123", Price: "0.5", Side: "BUY", BestBid: number("0.4")}}}},
		{`{"event_type":"tick_size_change","asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","old_tick_size":"0.01","new_tick_size":"0.001","timestamp":"3"}`, &TickSizeChange{AssetID: "1", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", OldTickSize: "0.01", NewTickSize: "0.001", Timestamp: "3"}},
		{`{"event_type":"last_trade_price","asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","price":"0.5","timestamp":"4"}`, &LastTradePrice{AssetID: "1", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", Price: "0.5", Timestamp: "4"}},
		{`{"event_type":"best_bid_ask","asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","best_bid":"0.4","best_ask":"0.6","spread":"0.2","timestamp":"5"}`, &BestBidAsk{AssetID: "1", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", BestBid: "0.4", BestAsk: "0.6", Spread: "0.2", Timestamp: "5"}},
		{`{"event_type":"new_market","id":"1","question":"q","market":"0x0000000000000000000000000000000000000000000000000000000000000001","slug":"s","description":"d","asset_ids":["123"],"outcomes":["Yes"],"event_message":{"id":"e","ticker":"t","slug":"s","title":"title","description":"d"},"timestamp":"6"}`, &NewMarket{ID: "1", Question: "q", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", Slug: "s", Description: "d", AssetIDs: []string{"123"}, Outcomes: []string{"Yes"}, EventMessage: &EventMessage{ID: "e", Ticker: "t", Slug: "s", Title: "title", Description: "d"}, Timestamp: "6"}},
		{`{"event_type":"market_resolved","id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","assets_ids":["123"],"outcomes":null,"winning_asset_id":"123","winning_outcome":"Yes","timestamp":"7"}`, &MarketResolved{ID: "1", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", AssetIDs: []string{"123"}, Outcomes: []string{}, WinningAssetID: "123", WinningOutcome: "Yes", Timestamp: "7"}},
		{`{"event_type":"trade","id":"t","asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","side":"BUY","price":"0.5","size":"10","status":"FUTURE","type":"future","match_time":"123","transaction_hash":"","maker_orders":[{"asset_id":"1","matched_amount":"2","order_id":"o","outcome":"Yes","owner":"123e4567-e89b-12d3-a456-426614174000","price":"0.5"}]}`, &TradeMessage{ID: "t", AssetID: "1", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", Side: "BUY", Price: "0.5", Size: "10", Status: "FUTURE", Type: str("future"), Matchtime: number("123"), MakerOrders: []MakerOrder{{AssetID: "1", MatchedAmount: "2", OrderID: "o", Outcome: "Yes", Owner: "123e4567-e89b-12d3-a456-426614174000", Price: "0.5"}}}},
		{`{"event_type":"order","id":"o","asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","side":"SELL","price":"0.6","type":"UPDATE","size_matched":"0","timestamp":"123","status":"NEW_STATUS","associate_trades":["t"]}`, &OrderMessage{ID: "o", AssetID: "1", Market: "0x0000000000000000000000000000000000000000000000000000000000000001", Side: "SELL", Price: "0.6", Type: str("UPDATE"), SizeMatched: number("0"), Timestamp: number("123"), Status: str("NEW_STATUS"), AssociateTrades: []string{"t"}}},
	}
	for _, tt := range tests {
		messages, err := ParseMessages([]byte(tt.data))
		if err != nil || len(messages) != 1 {
			t.Fatalf("parse %s: %v", tt.data, err)
		}
		t.Run(messages[0].EventType, func(t *testing.T) {
			got, err := messages[0].Typed()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func number(s string) *json.Number { n := json.Number(s); return &n }
func str(s string) *string         { return &s }

func TestTypedAliasesAndErrors(t *testing.T) {
	for _, key := range []string{"matchtime", "match_time"} {
		messages, err := ParseMessages([]byte(`{"event_type":"trade","id":"t","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","size":"1","price":"0.5","status":"MATCHED","` + key + `":"123","transaction_hash":"0x0000000000000000000000000000000000000000000000000000000000000001"}`))
		if err != nil {
			t.Fatal(err)
		}
		got, err := messages[0].Typed()
		if err != nil {
			t.Fatal(err)
		}
		trade := got.(*TradeMessage)
		if trade.Matchtime == nil || *trade.Matchtime != "123" || trade.TransactionHash == nil || *trade.TransactionHash != "0x0000000000000000000000000000000000000000000000000000000000000001" {
			t.Fatalf("%s: %+v", key, trade)
		}
	}
	for _, event := range []string{"new_market", "market_resolved"} {
		for _, key := range []string{"assets_ids", "asset_ids"} {
			data := []byte(`{"event_type":"` + event + `","id":"1","question":"q","market":"0x0000000000000000000000000000000000000000000000000000000000000001","slug":"s","description":"d","outcomes":[],"winning_asset_id":"123","winning_outcome":"Yes","timestamp":"1","` + key + `":["123"]}`)
			messages, err := ParseMessages(data)
			if err != nil {
				t.Fatal(err)
			}
			got, err := messages[0].Typed()
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			switch v := got.(type) {
			case *NewMarket:
				ids = v.AssetIDs
			case *MarketResolved:
				ids = v.AssetIDs
			}
			if !reflect.DeepEqual(ids, []string{"123"}) {
				t.Fatalf("%s %s: %v", event, key, ids)
			}
		}
	}
	for _, data := range []string{`{"event_type":"book","bids":[{"price":"bad"}]}`, `{"event_type":"unknown"}`} {
		messages, err := ParseMessages([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := messages[0].Typed(); err == nil {
			t.Fatalf("expected error for %s", data)
		}
	}
}
