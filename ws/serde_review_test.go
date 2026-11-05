package ws

import (
	"encoding/json"
	"testing"
)

func TestReviewTypedExactFields(t *testing.T) {
	body := `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"1","MARKET":"corrupt","bids":[{"price":"0.5","size":"2","SIZE":null}],"BIDS":null}`
	v, err := (Message{EventType: "book", Raw: json.RawMessage(body)}).Typed()
	if err != nil {
		t.Fatal(err)
	}
	book := v.(*BookUpdate)
	if book.Market != "0x0000000000000000000000000000000000000000000000000000000000000001" || len(book.Bids) != 1 || book.Bids[0].Size != "2" {
		t.Fatalf("unknown keys changed validated data: %+v", book)
	}
	body = `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"1","timestamp":"2","bids":[{"price":"1","size":"2","size":"3"}]}`
	v, err = (Message{EventType: "book", Raw: json.RawMessage(body)}).Typed()
	if err != nil || v.(*BookUpdate).Timestamp != "2" || v.(*BookUpdate).Bids[0].Size != "3" {
		t.Fatalf("duplicate key should use last value: %+v, %v", v, err)
	}
}

func TestReviewTypedRejectsDuplicateAliases(t *testing.T) {
	for _, tc := range []struct{ event, body string }{
		{"new_market", `{"id":"1","question":"q","market":"0x0000000000000000000000000000000000000000000000000000000000000001","slug":"s","description":"d","assets_ids":["1"],"asset_ids":["2"],"outcomes":[],"timestamp":"1"}`},
		{"trade", `{"id":"t","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","size":"1","price":"1","status":"MATCHED","matchtime":"1","match_time":"2"}`},
	} {
		if _, err := (Message{EventType: tc.event, Raw: json.RawMessage(tc.body)}).Typed(); err == nil {
			t.Errorf("accepted duplicate aliases: %s", tc.body)
		}
	}
}
