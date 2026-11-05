package rtds

import (
	"encoding/json"
	"testing"
)

func TestReviewEnvelopeExactFields(t *testing.T) {
	messages, err := ParseMessages([]byte(`{"topic":"crypto_prices","TOPIC":"comments","type":"update","timestamp":1,"payload":{}}`))
	if err != nil || len(messages) != 1 || messages[0].Topic != "crypto_prices" {
		t.Fatalf("unknown field changed topic: %+v, %v", messages, err)
	}
	if _, err := ParseMessages([]byte(`{"topic":"a","topic":"b","type":"update","timestamp":1,"payload":{}}`)); err == nil {
		t.Fatal("accepted duplicate topic")
	}
}

func TestReviewTypedExactFields(t *testing.T) {
	price, ok := (Message{Topic: "crypto_prices", Payload: json.RawMessage(`{"symbol":"good","SYMBOL":"bad","timestamp":1,"value":"2","VALUE":null}`)}).AsCryptoPrice()
	if !ok || price.Symbol != "good" || price.Value != "2" {
		t.Fatalf("unknown fields changed price: %+v, %v", price, ok)
	}
	price, ok = (Message{Topic: "crypto_prices", Payload: json.RawMessage(`{"symbol":"a","symbol":"b","timestamp":1,"value":2}`)}).AsCryptoPrice()
	if !ok || price.Symbol != "b" {
		t.Fatalf("duplicate payload key should use last value: %+v, %v", price, ok)
	}
	body := `{"id":"1","body":"hi","createdAt":"2025-07-25T14:49:35Z","parentEntityID":0,"parentEntityType":"Event","profile":{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"good","NAME":"bad"},"userAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","BODY":"bad","reactionCount":2,"REACTIONCOUNT":null}`
	comment, ok := (Message{Topic: "comments", Payload: json.RawMessage(body)}).AsComment()
	if !ok || comment.Body != "hi" || comment.Profile.Name != "good" || comment.ReactionCount != 2 {
		t.Fatalf("unknown fields changed comment: %+v, %v", comment, ok)
	}
	body = `{"id":"1","body":"hi","createdAt":"2025-07-25T14:49:35Z","parentEntityID":0,"parentEntityType":"Event","profile":{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"a","name":"b"},"userAddress":"0xce533188d53a16ed580fd5121dedf166d3482677"}`
	comment, ok = (Message{Topic: "comments", Payload: json.RawMessage(body)}).AsComment()
	if !ok || comment.Profile.Name != "b" {
		t.Fatalf("duplicate profile key should use last value: %+v, %v", comment, ok)
	}
}
