package data

import (
	"encoding/json"
	"testing"
)

func TestDirectResponseNormalization(t *testing.T) {
	for _, side := range []struct{ input, want string }{{"buy", "BUY"}, {"SeLl", "SELL"}, {"other", ""}, {"", ""}} {
		var a Activity
		if err := json.Unmarshal([]byte(`{"proxyWallet":"`+testAddress+`","timestamp":0,"type":"REWARD","size":0,"usdcSize":0,"transactionHash":"`+testHash+`","asset":"","conditionId":"","title":"","side":"`+side.input+`"}`), &a); err != nil {
			t.Fatal(err)
		}
		if a.Asset != nil || a.ConditionID != nil || a.Title != nil {
			t.Fatalf("%+v", a)
		}
		if side.want == "" && a.Side != nil || side.want != "" && (a.Side == nil || *a.Side != side.want) {
			t.Fatalf("side %q: %v", side.input, a.Side)
		}
	}
	var p Position
	if err := json.Unmarshal([]byte(testPosition), &p); err != nil || p.EventID != nil || p.EndDate != nil {
		t.Fatalf("%+v %v", p, err)
	}
	var entry TraderLeaderboardEntry
	if err := json.Unmarshal([]byte(`{"rank":"1","proxyWallet":"`+testAddress+`","vol":0,"pnl":0,"userName":"","profileImage":""}`), &entry); err != nil || entry.UserName != nil || entry.ProfileImage != nil {
		t.Fatalf("%+v %v", entry, err)
	}
}

func TestTradeFilterDecimalValidation(t *testing.T) {
	for _, tc := range []struct {
		value Number
		valid bool
	}{{"0", true}, {"0.100000000000000001", true}, {"-1", false}, {"1/2", false}, {"0x10", false}, {"", false}} {
		err := validateQuery(TradesRequest{TradeFilter: &TradeFilter{FilterType: FilterCash, FilterAmount: tc.value}})
		if (err == nil) != tc.valid {
			t.Errorf("%q: %v", tc.value, err)
		}
	}
}
