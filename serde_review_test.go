package clob

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestReviewPageNullElements(t *testing.T) {
	var numbers Page[int]
	if err := json.Unmarshal([]byte(`{"data":[1,null,3],"next_cursor":"LTE=","limit":3,"count":3}`), &numbers); err == nil {
		t.Fatal("null page item silently became zero")
	}
	var optional Page[*int]
	if err := json.Unmarshal([]byte(`{"data":[1,null,3],"next_cursor":"LTE=","limit":3,"count":3}`), &optional); err != nil || len(optional.Data) != 3 || optional.Data[1] != nil {
		t.Fatalf("nullable page item: %+v, %v", optional, err)
	}
}

func TestReviewCompactMarketFields(t *testing.T) {
	base := fmt.Sprintf(`"c":%q,"mts":"0.01"`, safetyHash)
	for _, field := range []string{`"c":"bad"`, `"mts":"0.1"`, `"nr":true,"nr":false`, `"mos":null`, `"rfqe":null`} {
		var got ClobMarketInfoResponse
		if err := json.Unmarshal([]byte(`{`+base+`,`+field+`}`), &got); err == nil {
			t.Errorf("accepted %s", field)
		}
	}
	var got ClobMarketInfoResponse
	if err := json.Unmarshal([]byte(`{`+base+`,"NR":true,"MOS":null,"rfqe":false}`), &got); err != nil || got.NegRisk {
		t.Fatalf("unknown fields changed market: %+v, %v", got, err)
	}
}
