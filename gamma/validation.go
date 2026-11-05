package gamma

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// These checks preserve the public string fields while validating the types used
// by the Rust Gamma models. Unknown fields and optional nulls remain accepted.
type responseRules struct {
	required, timestamps, dates, addresses, hashes, uints []string
}

var responseValidation = map[string]responseRules{
	"Team":                      {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt"}},
	"SportsMetadata":            {required: []string{"sport", "image", "resolution", "ordering", "tags", "series"}, timestamps: []string{"createdAt"}},
	"SportsMarketTypesResponse": {required: []string{"marketTypes"}},
	"Tag":                       {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt"}},
	"RelatedTag":                {required: []string{"id"}},
	"Category":                  {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt"}},
	"EventCreator":              {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt"}},
	"Chat":                      {required: []string{"id"}, timestamps: []string{"startTime", "endTime"}},
	"Template":                  {required: []string{"id"}},
	"Collection":                {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt"}},
	"Event":                     {required: []string{"id"}, timestamps: []string{"startDate", "creationDate", "endDate", "createdAt", "updatedAt", "closedTime", "startTime", "finishedTimestamp", "deployingTimestamp", "scheduledDeploymentTimestamp"}, dates: []string{"eventDate"}, hashes: []string{"negRiskMarketID"}},
	"Market":                    {required: []string{"id"}, timestamps: []string{"endDate", "startDate", "createdAt", "updatedAt", "readyTimestamp", "fundedTimestamp", "acceptingOrdersTimestamp", "deployingTimestamp", "scheduledDeploymentTimestamp", "eventStartTime"}, dates: []string{"lowerBoundDate", "upperBoundDate", "endDateIso", "startDateIso", "umaEndDateIso"}, addresses: []string{"marketMakerAddress"}, hashes: []string{"conditionId", "questionID", "negRiskRequestID", "negRiskMarketID"}, uints: []string{"denominationToken"}},
	"ClobReward":                {dates: []string{"startDate", "endDate"}, addresses: []string{"assetAddress"}, hashes: []string{"conditionId"}},
	"Series":                    {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt", "startDate"}},
	"CommentPosition":           {uints: []string{"tokenId"}},
	"CommentProfile":            {addresses: []string{"proxyWallet", "baseAddress"}},
	"Reaction":                  {required: []string{"id"}, timestamps: []string{"createdAt"}, addresses: []string{"userAddress"}},
	"Comment":                   {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt"}, addresses: []string{"userAddress", "replyAddress"}},
	"PublicProfile":             {timestamps: []string{"createdAt"}, addresses: []string{"proxyWallet"}},
	"Profile":                   {required: []string{"id"}, timestamps: []string{"createdAt", "updatedAt", "certReqDate"}, addresses: []string{"proxyWallet"}},
}

func validateResponse(b []byte, name string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("%s: expected object", name)
	}
	rules := responseValidation[name]
	for _, field := range rules.required {
		raw, ok := fields[field]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s: missing or null required field %q", name, field)
		}

	}
	check := func(names []string, fn func(string) error) error {
		for _, field := range names {
			raw, ok := fields[field]
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("%s.%s: %w", name, field, err)
			}
			if err := fn(value); err != nil {
				return fmt.Errorf("%s.%s: %w", name, field, err)
			}
		}
		return nil
	}
	if err := check(rules.timestamps, validateResponseTimestamp); err != nil {
		return err
	}
	if err := check(rules.dates, func(s string) error { _, err := time.Parse("2006-01-02", s); return err }); err != nil {
		return err
	}
	if err := check(rules.addresses, func(s string) error { return validateHex(s, 20) }); err != nil {
		return err
	}
	if err := check(rules.hashes, func(s string) error { return validateHex(s, 32) }); err != nil {
		return err
	}
	for _, field := range rules.uints {
		raw, ok := fields[field]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var n Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return fmt.Errorf("%s.%s: %w", name, field, err)
		}
		if err := validateUint256(string(n)); err != nil {
			return fmt.Errorf("%s.%s: %w", name, field, err)
		}
	}
	return nil
}

func validateHex(s string, size int) error {
	if s == "" {
		return nil
	} // NoneAsEmptyString
	if !strings.HasPrefix(s, "0x") || len(s) != 2+size*2 {
		return fmt.Errorf("expected %d-byte 0x-prefixed hex value", size)
	}
	if _, err := hex.DecodeString(s[2:]); err != nil {
		return fmt.Errorf("invalid hex value: %w", err)
	}
	return nil
}

func parseUint256(s string) (*big.Int, error) {
	digits, base := s, 10
	if strings.HasPrefix(s, "0x") {
		digits, base = s[2:], 16
	}
	if digits == "" {
		return nil, fmt.Errorf("invalid U256 %q", s)
	}
	for _, c := range digits {
		if !('0' <= c && c <= '9') && !(base == 16 && (('a' <= c && c <= 'f') || ('A' <= c && c <= 'F'))) {
			return nil, fmt.Errorf("invalid U256 %q", s)
		}
	}
	// Limit big.Int parsing to a U256-sized input, while retaining leading-zero
	// compatibility. Parsing an unbounded decimal integer is superlinear.
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return new(big.Int), nil
	}
	if (base == 10 && len(digits) > 78) || (base == 16 && len(digits) > 64) {
		return nil, fmt.Errorf("U256 out of range: %q", s)
	}
	n, ok := new(big.Int).SetString(digits, base)
	if !ok || n.BitLen() > 256 {
		return nil, fmt.Errorf("U256 out of range: %q", s)
	}
	return n, nil
}

func validateUint256(s string) error {
	_, err := parseUint256(s)
	return err
}

func validateResponseTimestamp(s string) error {
	if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
		return err
	}
	if len(s) >= 6 && (s[len(s)-6] == '+' || s[len(s)-6] == '-') {
		zone := s[len(s)-5:]
		if zone[:2] > "23" || zone[3:] > "59" {
			return fmt.Errorf("invalid UTC offset in %q", s)
		}
	}
	return nil
}

// Decimal response values may have more fractional digits than rust_decimal;
// retain that wire precision, but reject magnitudes outside its 96-bit range.
func validateResponseDecimal(s string) error {
	if !decimalPattern.MatchString(s) {
		return fmt.Errorf("invalid decimal %q", s)
	}
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(s), "e")
	mantissa = strings.TrimLeft(mantissa, "+-")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return nil
	}
	exponent := int64(0)
	if hasExponent {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 64)
		if err != nil {
			if strings.HasPrefix(exponentText, "-") {
				return nil
			}
			return fmt.Errorf("decimal out of range: %q", s)
		}
	}
	// Clamp before subtracting the fraction length to avoid integer overflow.
	if exponent > int64(len(fraction))+29 {
		return fmt.Errorf("decimal out of range: %q", s)
	}
	if exponent < -int64(len(digits))-29 {
		return nil
	}
	integerDigits := int64(len(digits)) + exponent - int64(len(fraction))
	if integerDigits > 29 {
		return fmt.Errorf("decimal out of range: %q", s)
	}
	if integerDigits < 29 {
		return nil
	}
	integer := digits
	if len(integer) < 29 {
		integer += strings.Repeat("0", 29-len(integer))
	}
	if integer[:29] > "79228162514264337593543950335" || (integer[:29] == "79228162514264337593543950335" && len(integer) > 29 && strings.Trim(integer[29:], "0") != "") {
		return fmt.Errorf("decimal out of range: %q", s)
	}
	return nil
}

func validateResponseDecimals(dst any, name string) error {
	v := reflect.ValueOf(dst).Elem()
	number := reflect.TypeOf(Number(""))
	list := reflect.TypeOf(NumberList(nil))
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.Kind() != reflect.Pointer || field.IsNil() {
			continue
		}
		key := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
		if (name == "Market" && (key == "denominationToken" || key == "clobTokenIds")) || (name == "CommentPosition" && key == "tokenId") {
			continue
		}
		switch field.Elem().Type() {
		case number:
			if err := validateResponseDecimal(field.Elem().String()); err != nil {
				return fmt.Errorf("%s.%s: %w", name, key, err)
			}
		case list:
			for j := 0; j < field.Elem().Len(); j++ {
				if err := validateResponseDecimal(field.Elem().Index(j).String()); err != nil {
					return fmt.Errorf("%s.%s[%d]: %w", name, key, j, err)
				}
			}
		}
	}
	return nil
}

// exactResponseFields prevents encoding/json's case-insensitive matching from
// populating fields that serde would treat as unknown.
func exactResponseFields(b []byte, dst any, aliases map[string]string) ([]byte, error) {
	allowed := make(map[string]string)
	t := reflect.TypeOf(dst).Elem()
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		allowed[name] = name
	}
	for alias, name := range aliases {
		allowed[alias] = name
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("expected object")
	}
	fields := make(map[string]json.RawMessage)
	seen := make(map[string]bool)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name := key.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		canonical, ok := allowed[name]
		if !ok {
			continue
		}
		if seen[canonical] {
			return nil, fmt.Errorf("%s: duplicate field %q", t, canonical)
		}
		seen[canonical] = true
		fields[name] = raw
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

// Go otherwise silently decodes null array elements into zero-valued structs or
// strings, whereas these fields are Vec<T>, not Vec<Option<T>>, in Rust.
func validateResponseLists(b []byte, dst any, name string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	t := reflect.TypeOf(dst).Elem()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() != reflect.Pointer || field.Type.Elem().Kind() != reflect.Slice || field.Type.Elem().Name() != "" {
			continue
		}
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return fmt.Errorf("%s.%s: %w", name, key, err)
		}
		for index, element := range elements {
			if bytes.Equal(bytes.TrimSpace(element), []byte("null")) {
				return fmt.Errorf("%s.%s[%d]: null array element", name, key, index)
			}
		}
	}
	return nil
}

func unmarshalResponse(b []byte, name string, dst any) error {
	if err := validateResponse(b, name); err != nil {
		return err
	}
	if err := validateResponseLists(b, dst, name); err != nil {
		return err
	}
	value := reflect.ValueOf(dst)
	decoded := reflect.New(value.Elem().Type())
	clean, err := exactResponseFields(b, decoded.Interface(), nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(clean, decoded.Interface()); err != nil {
		return err
	}
	if err := validateResponseDecimals(decoded.Interface(), name); err != nil {
		return err
	}
	normalizeEmpty(decoded)
	value.Elem().Set(decoded.Elem())
	return nil
}

func (v *ImageOptimization) UnmarshalJSON(b []byte) error {
	type plain ImageOptimization
	return unmarshalResponse(b, "ImageOptimization", (*plain)(v))
}
func (v *Pagination) UnmarshalJSON(b []byte) error {
	type plain Pagination
	return unmarshalResponse(b, "Pagination", (*plain)(v))
}
func (v *PublicProfileUser) UnmarshalJSON(b []byte) error {
	type plain PublicProfileUser
	return unmarshalResponse(b, "PublicProfileUser", (*plain)(v))
}
func (v *SearchTag) UnmarshalJSON(b []byte) error {
	type plain SearchTag
	return unmarshalResponse(b, "SearchTag", (*plain)(v))
}
func (v *Team) UnmarshalJSON(b []byte) error {
	type plain Team
	return unmarshalResponse(b, "Team", (*plain)(v))
}
func (v *SportsMetadata) UnmarshalJSON(b []byte) error {
	type plain SportsMetadata
	return unmarshalResponse(b, "SportsMetadata", (*plain)(v))
}
func (v *SportsMarketTypesResponse) UnmarshalJSON(b []byte) error {
	if err := validateResponse(b, "SportsMarketTypesResponse"); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(fields["marketTypes"], &elements); err != nil {
		return err
	}
	for i, element := range elements {
		if bytes.Equal(bytes.TrimSpace(element), []byte("null")) {
			return fmt.Errorf("SportsMarketTypesResponse.marketTypes[%d]: null string", i)
		}
	}
	type plain SportsMarketTypesResponse
	return unmarshalResponse(b, "SportsMarketTypesResponse", (*plain)(v))
}

func (v SportsMarketTypesResponse) MarshalJSON() ([]byte, error) {
	type plain SportsMarketTypesResponse
	if v.MarketTypes == nil {
		v.MarketTypes = []string{}
	}
	return json.Marshal(plain(v))
}
func (v *Tag) UnmarshalJSON(b []byte) error {
	type plain Tag
	return unmarshalResponse(b, "Tag", (*plain)(v))
}
func (v *RelatedTag) UnmarshalJSON(b []byte) error {
	type plain RelatedTag
	return unmarshalResponse(b, "RelatedTag", (*plain)(v))
}
func (v *Category) UnmarshalJSON(b []byte) error {
	type plain Category
	return unmarshalResponse(b, "Category", (*plain)(v))
}
func (v *EventCreator) UnmarshalJSON(b []byte) error {
	type plain EventCreator
	return unmarshalResponse(b, "EventCreator", (*plain)(v))
}
func (v *Chat) UnmarshalJSON(b []byte) error {
	type plain Chat
	return unmarshalResponse(b, "Chat", (*plain)(v))
}
func (v *Template) UnmarshalJSON(b []byte) error {
	type plain Template
	return unmarshalResponse(b, "Template", (*plain)(v))
}
func (v *Collection) UnmarshalJSON(b []byte) error {
	type plain Collection
	return unmarshalResponse(b, "Collection", (*plain)(v))
}
func (v *ClobReward) UnmarshalJSON(b []byte) error {
	type plain ClobReward
	return unmarshalResponse(b, "ClobReward", (*plain)(v))
}
func (v *Series) UnmarshalJSON(b []byte) error {
	type plain Series
	return unmarshalResponse(b, "Series", (*plain)(v))
}
func (v *CommentPosition) UnmarshalJSON(b []byte) error {
	type plain CommentPosition
	return unmarshalResponse(b, "CommentPosition", (*plain)(v))
}
func (v *CommentProfile) UnmarshalJSON(b []byte) error {
	type plain CommentProfile
	return unmarshalResponse(b, "CommentProfile", (*plain)(v))
}
func (v *Reaction) UnmarshalJSON(b []byte) error {
	type plain Reaction
	return unmarshalResponse(b, "Reaction", (*plain)(v))
}
func (v *Comment) UnmarshalJSON(b []byte) error {
	type plain Comment
	return unmarshalResponse(b, "Comment", (*plain)(v))
}
func (v *PublicProfile) UnmarshalJSON(b []byte) error {
	type plain PublicProfile
	return unmarshalResponse(b, "PublicProfile", (*plain)(v))
}
func (v *Profile) UnmarshalJSON(b []byte) error {
	type plain Profile
	return unmarshalResponse(b, "Profile", (*plain)(v))
}
func (v *SearchResults) UnmarshalJSON(b []byte) error {
	type plain SearchResults
	return unmarshalResponse(b, "SearchResults", (*plain)(v))
}
