package gamma

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSportsTagsRoundTrip(t *testing.T) {
	var v SportsMetadata
	if err := json.Unmarshal([]byte(`{"sport":"nba","image":"","resolution":"","ordering":"","tags":"1,2,3","series":""}`), &v); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["tags"] != "1,2,3" {
		t.Fatalf("tags: %s", b)
	}
	var again SportsMetadata
	if err := json.Unmarshal(b, &again); err != nil || !reflect.DeepEqual(v.Tags, again.Tags) {
		t.Fatalf("round trip: %+v %v", again, err)
	}
}

func TestEmptySportsTagsSerialize(t *testing.T) {
	b, err := json.Marshal(SportsMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["tags"] != "" {
		t.Fatalf("tags: %s", b)
	}
}

func TestMarketDirectEmptyNormalization(t *testing.T) {
	var v Event
	if err := json.Unmarshal([]byte(`{"id":"1","markets":[{"id":"1","conditionId":"","marketMakerAddress":"","questionID":""}]}`), &v); err != nil {
		t.Fatal(err)
	}
	m := (*v.Markets)[0]
	if m.ConditionID != nil || m.MarketMakerAddress != nil || m.QuestionID != nil {
		t.Fatalf("%+v", m)
	}
}
