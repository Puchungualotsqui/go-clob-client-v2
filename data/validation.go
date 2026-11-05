package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var numberPattern = regexp.MustCompile(`^([+-]?)([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE]([+-]?[0-9]+))?$`)
var hexPattern = regexp.MustCompile(`^[0-9a-fA-F]+$`)
var decimalMax, _ = new(big.Int).SetString("79228162514264337593543950335", 10)

// validateDecimal accepts only values representable exactly by rust_decimal's
// 96-bit coefficient and scale of at most 28, without converting via float64.
func validateDecimal(s string) error {
	m := numberPattern.FindStringSubmatch(s)
	if m == nil {
		return fmt.Errorf("invalid decimal %q", s)
	}
	exponent := int64(0)
	if m[3] != "" {
		var err error
		exponent, err = strconv.ParseInt(m[3], 10, 32)
		if err != nil || exponent > 1000 || exponent < -1000 {
			return fmt.Errorf("decimal exponent out of range: %q", s)
		}
	}
	parts := strings.SplitN(m[2], ".", 2)
	scale := -exponent
	digits := parts[0]
	if len(parts) == 2 {
		digits += parts[1]
		scale += int64(len(parts[1]))
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return nil
	}
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = digits[:len(digits)-1]
		scale--
	}
	if scale > 28 || scale < -29 || int64(len(digits))-scale > 29 {
		return fmt.Errorf("decimal out of range: %q", s)
	}
	if scale < 0 {
		digits += strings.Repeat("0", int(-scale))
	}
	coefficient, ok := new(big.Int).SetString(digits, 10)
	if !ok || coefficient.Cmp(decimalMax) > 0 {
		return fmt.Errorf("decimal out of range: %q", s)
	}
	return nil
}

func validateU256(s string) error {
	base := 10
	digits := s
	if strings.HasPrefix(s, "0x") {
		base, digits = 16, s[2:]
	}
	if digits == "" || (base == 16 && !hexPattern.MatchString(digits)) || (base == 10 && strings.Trim(digits, "0123456789") != "") {
		return fmt.Errorf("invalid U256 %q", s)
	}
	n, ok := new(big.Int).SetString(digits, base)
	if !ok || n.BitLen() > 256 {
		return fmt.Errorf("U256 out of range: %q", s)
	}
	return nil
}

func validateNumberText(s string) error {
	if strings.HasPrefix(s, "0x") {
		return validateU256(s)
	}
	// Token identifiers can be larger than a Decimal; field-specific checks
	// distinguish them when decoding a response.
	if validateU256(s) == nil {
		return nil
	}
	return validateDecimal(s)
}

func validateHex(s string, size int) error {
	if !strings.HasPrefix(s, "0x") || len(s) != 2+size*2 || !hexPattern.MatchString(s[2:]) {
		return fmt.Errorf("expected 0x-prefixed %d-byte hex identifier, got %q", size, s)
	}
	return nil
}

// validateTimestamp also checks the numeric timezone offset: time.Parse accepts
// hour 24 and minute 60 even though RFC 3339 and chrono reject them.
func validateTimestamp(s string) error {
	if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
		return err
	}
	if !strings.HasSuffix(s, "Z") {
		offset := s[len(s)-6:]
		if offset[1:3] > "23" || offset[4:6] > "59" {
			return fmt.Errorf("invalid timezone offset %q", offset)
		}
	}
	return nil
}

// rejectDuplicateFields follows serde's derived struct behavior: known fields
// cannot occur twice, but unknown fields are ignored, including duplicates.
func rejectDuplicateFields(b []byte, known map[string]bool) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		name := token.(string)
		if known[name] && seen[name] {
			return fmt.Errorf("duplicate field %q", name)
		}
		seen[name] = true
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	return nil
}

// decodeResponse applies serde's required-field rules before decoding into an
// alias. Pointer fields are optional; LiveVolume.markets is DefaultOnNull.
func decodeResponse(b []byte, dst any) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("expected response object")
	}
	t := reflect.TypeOf(dst).Elem()
	known := make(map[string]bool, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		known[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	if err := rejectDuplicateFields(b, known); err != nil {
		return err
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		raw, present := fields[name]
		// Only LiveVolume has a markets field among response structs.
		optional := f.Type.Kind() == reflect.Pointer || name == "markets"
		if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if !optional {
				return fmt.Errorf("missing or null required field %q", name)
			}
			continue
		}
		if f.Tag.Get("empty") == "nil" && bytes.Equal(raw, []byte(`""`)) {
			// In particular, an empty optional U256 must be normalized before
			// Number's unmarshaler sees it.
			fields[name] = json.RawMessage("null")
			continue
		}
		var s string
		if f.Type == reflect.TypeOf(Number("")) || f.Type == reflect.TypeOf((*Number)(nil)) {
			if len(raw) > 0 && raw[0] == '"' {
				if err := json.Unmarshal(raw, &s); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			} else {
				s = string(raw)
			}
			var err error
			if name == "asset" || name == "oppositeAsset" || name == "token" {
				err = validateU256(s)
			} else {
				err = validateDecimal(s)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		switch name {
		case "proxyWallet", "user", "conditionId", "transactionHash", "market", "endDate", "dt":
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			var err error
			switch name {
			case "proxyWallet", "user":
				err = validateHex(s, 20)
			case "conditionId", "transactionHash":
				err = validateHex(s, 32)
			case "market":
				if s != "global" && s != "GLOBAL" && s != "Global" {
					err = validateHex(s, 32)
				}
			case "dt":
				err = validateTimestamp(s)
			case "endDate":
				if f.Type.Kind() == reflect.Pointer {
					_, err = time.Parse("2006-01-02", s)
				} else {
					err = validateTimestamp(s)
				}
			}
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	// encoding/json matches field names case-insensitively. Drop unknown keys
	// so they cannot overwrite an exact, validated field during decoding.
	for name := range fields {
		if !known[name] {
			delete(fields, name)
		}
	}
	clean, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	decoded := reflect.New(t)
	if err := json.Unmarshal(clean, decoded.Interface()); err != nil {
		return err
	}
	reflect.ValueOf(dst).Elem().Set(decoded.Elem())
	return nil
}
