package decimal

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/puchungualotsqui/go-clob-client-v2/data"
	"github.com/puchungualotsqui/go-clob-client-v2/gamma"
)

// These are Go model regression cases, not independent cross-SDK vectors.
func TestDecimalModels(t *testing.T) {
	cases := []struct {
		name, token, text string
		dataOK, gammaOK   bool
	}{
		{"zero-number", `0`, "0", true, true},
		{"trailing-zeros", `"1.2300"`, "1.2300", true, true},
		{"binary64-boundary", `9007199254740993`, "9007199254740993", true, true},
		{"precision-number", `0.1234567890123456789012345678`, "0.1234567890123456789012345678", true, true},
		{"precision-string", `"0.1234567890123456789012345678"`, "0.1234567890123456789012345678", true, true},
		{"scale-29", `"0.12345678901234567890123456789"`, "0.12345678901234567890123456789", false, true},
		{"tiny", `1e-29`, "1e-29", false, true},
		{"max", `79228162514264337593543950335`, "79228162514264337593543950335", true, true},
		{"overflow", `79228162514264337593543950336`, "", false, false},
		{"exponent", `1.25e2`, "1.25e2", true, true},
		{"leading-dot", `".5"`, ".5", true, true},
		{"invalid", `"not-a-number"`, "", false, false},
		{"empty", `""`, "", false, false},
		{"boolean", `true`, "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var value data.Value
			err := json.Unmarshal([]byte(`{"user":"0x0000000000000000000000000000000000000001","value":`+tc.token+`}`), &value)
			if (err == nil) != tc.dataOK {
				t.Fatalf("data acceptance: %v, want %t", err, tc.dataOK)
			}
			if tc.dataOK {
				if string(value.Value) != tc.text {
					t.Errorf("data value = %q, want %q", value.Value, tc.text)
				}
				checkWire(t, value, "value", tc.text)
			}
			for _, field := range []string{"liquidity", "volume", "openInterest", "spreadsMainLine"} {
				t.Run(field, func(t *testing.T) {
					var event gamma.Event
					err := json.Unmarshal([]byte(fmt.Sprintf(`{"id":"event-1",%q:%s}`, field, tc.token)), &event)
					if (err == nil) != tc.gammaOK {
						t.Fatalf("gamma acceptance: %v, want %t", err, tc.gammaOK)
					}
					if tc.gammaOK {
						checkWire(t, event, field, tc.text)
					}
				})
			}
		})
	}
}

func checkWire(t *testing.T, model any, field, text string) {
	t.Helper()
	b, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	if string(fields[field]) != string(want) {
		t.Errorf("%s wire = %s, want %s", field, fields[field], want)
	}
}

func TestOptionalGammaDecimal(t *testing.T) {
	for _, body := range []string{`{"id":"event-1"}`, `{"id":"event-1","liquidity":null}`} {
		var event gamma.Event
		if err := json.Unmarshal([]byte(body), &event); err != nil {
			t.Fatal(err)
		}
		if event.Liquidity != nil {
			t.Fatalf("%s: liquidity = %v", body, event.Liquidity)
		}
		b, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["liquidity"]; ok {
			t.Errorf("%s: liquidity not omitted: %s", body, b)
		}
	}
}
