package clob

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// responseFields checks presence independently of Go's zero-value decoding.
func responseFields(b []byte, optional []string, required ...string) (map[string]json.RawMessage, error) {
	return responseFieldsWithCaseCheck(b, true, optional, required...)
}

func responseFieldsWithCaseCheck(b []byte, checkCase bool, optional []string, required ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("expected response object")
	}
	// encoding/json also matches field names case-insensitively. Reject those
	// spellings so a key ignored by the validation cannot alter decoded data.
	names := append(append([]string(nil), required...), optional...)
	for key := range fields {
		exact := false
		for _, name := range names {
			if key == name {
				exact = true
				break
			}
		}
		if checkCase && !exact {
			for _, name := range names {
				if strings.EqualFold(key, name) {
					return nil, fmt.Errorf("%s: noncanonical spelling of %s", key, name)
				}
			}
		}
	}
	// A map keeps only the last duplicate key, while decoding into a struct
	// may merge duplicate map fields. Reject duplicates of recognized fields.
	decoder := json.NewDecoder(bytes.NewReader(b))
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		for _, name := range names {
			if key == name {
				if seen[key] {
					return nil, fmt.Errorf("%s: duplicate field", key)
				}
				seen[key] = true
				break
			}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
	}
	for _, name := range required {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%s: required non-null field", name)
		}
	}
	return fields, nil
}

func responseUUID(fields map[string]json.RawMessage, name string) error {
	var value string
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if (len(value) == 38 && (value[0] != '{' || value[37] != '}')) ||
		(len(value) == 45 && !strings.HasPrefix(value, "urn:uuid:")) {
		return fmt.Errorf("%s: invalid UUID wrapper", name)
	}
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("%s: invalid UUID: %w", name, err)
	}
	return nil
}

func responseHash(fields map[string]json.RawMessage, name string) error {
	var value string
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if _, err := ParseB256(value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func responseAddress(fields map[string]json.RawMessage, name string) error {
	var value string
	if err := json.Unmarshal(fields[name], &value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if _, err := ParseAddress(value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// responseStringList validates elements too: encoding/json accepts null as a string.
func responseStringList(fields map[string]json.RawMessage, name string, hashes bool) error {
	raw, ok := fields[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for i, value := range values {
		var s *string
		if err := json.Unmarshal(value, &s); err != nil || s == nil {
			return fmt.Errorf("%s[%d]: expected string", name, i)
		}
		if hashes {
			if _, err := ParseB256(*s); err != nil {
				return fmt.Errorf("%s[%d]: %w", name, i, err)
			}
		}
	}
	return nil
}

func validatePostOrderResponse(b []byte) error {
	fields, err := responseFields(b, []string{"errorMsg", "transactionHashes", "transactionsHashes", "tradeIds", "tradeIDs"}, "makingAmount", "takingAmount", "orderID", "status", "success")
	if err != nil {
		return err
	}
	for _, pair := range [][2]string{{"transactionHashes", "transactionsHashes"}, {"tradeIds", "tradeIDs"}} {
		if _, ok := fields[pair[0]]; ok {
			if _, ok := fields[pair[1]]; ok {
				return fmt.Errorf("%s: duplicate field via alias %s", pair[0], pair[1])
			}
		}
	}
	for _, item := range []struct {
		name   string
		hashes bool
	}{
		{"transactionHashes", true}, {"transactionsHashes", true}, {"tradeIds", false}, {"tradeIDs", false},
	} {
		if err := responseStringList(fields, item.name, item.hashes); err != nil {
			return err
		}
	}
	return nil
}

func validateOpenOrderResponse(b []byte) error {
	fields, err := responseFields(b, []string{"associate_trades"}, "id", "status", "owner", "maker_address", "market", "asset_id", "side", "original_size", "size_matched", "price", "outcome", "created_at", "expiration", "order_type")
	if err != nil {
		return err
	}
	if err := responseUUID(fields, "owner"); err != nil {
		return err
	}
	if err := responseHash(fields, "market"); err != nil {
		return err
	}
	if err := responseAddress(fields, "maker_address"); err != nil {
		return err
	}
	return responseStringList(fields, "associate_trades", false)
}

func validateTradeResponse(b []byte) error {
	fields, err := responseFields(b, []string{"maker_orders", "transaction_hash", "error_msg"}, "id", "taker_order_id", "market", "asset_id", "side", "size", "fee_rate_bps", "price", "status", "match_time", "last_update", "outcome", "bucket_index", "owner", "maker_address", "trader_side")
	if err != nil {
		return err
	}
	if err := responseUUID(fields, "owner"); err != nil {
		return err
	}
	if err := responseHash(fields, "market"); err != nil {
		return err
	}
	return responseAddress(fields, "maker_address")
}

func validateBalanceAllowanceResponse(b []byte) error {
	fields, err := responseFields(b, []string{"allowances"}, "balance")
	if err != nil {
		return err
	}
	raw, ok := fields["allowances"]
	if !ok {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("allowances: expected map, got null")
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("allowances: %w", err)
	}
	for address, value := range values {
		if _, err := ParseAddress(address); err != nil {
			return fmt.Errorf("allowances[%q]: %w", address, err)
		}
		var s *string
		if err := json.Unmarshal(value, &s); err != nil || s == nil {
			return fmt.Errorf("allowances[%q]: expected string", address)
		}
	}
	return nil
}

// responsePresentFields validates non-null fields when partial responses are supported.
func responsePresentFields(b []byte, names ...string) (map[string]json.RawMessage, error) {
	fields, err := responseFields(b, names)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if raw, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("%s: expected non-null field", name)
		}
	}
	return fields, nil
}

func validatePresentResponseFields(fields map[string]json.RawMessage, hashes, addresses []string, names ...string) error {
	for _, name := range append(append(names, hashes...), addresses...) {
		if raw, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s: expected non-null field", name)
		}
	}
	return responseOptionalIdentifiers(fields, hashes, addresses)
}

func responseDates(fields map[string]json.RawMessage, names ...string) error {
	for _, name := range names {
		raw, ok := fields[name]
		if !ok {
			continue
		}
		var date string
		if err := json.Unmarshal(raw, &date); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if len(date) != len("2006-01-02") {
			return fmt.Errorf("%s: invalid date %q", name, date)
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func responseOptionalIdentifiers(fields map[string]json.RawMessage, hashes, addresses []string) error {
	for _, name := range hashes {
		if _, ok := fields[name]; ok {
			if err := responseHash(fields, name); err != nil {
				return err
			}
		}
	}
	for _, name := range addresses {
		if _, ok := fields[name]; ok {
			if err := responseAddress(fields, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MakerOrder) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "order_id", "owner", "maker_address", "matched_amount", "price", "fee_rate_bps", "asset_id", "outcome", "side")
	if err != nil {
		return err
	}
	if err := responseUUID(fields, "owner"); err != nil {
		return err
	}
	if err := responseAddress(fields, "maker_address"); err != nil {
		return err
	}
	type plain MakerOrder
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = MakerOrder(v)
	return nil
}
