package gamma

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Request validation happens before transport because the public request types
// retain string representations of Rust's strongly typed values.
type namedString struct {
	name  string
	value *string
}

type namedNumber struct {
	name  string
	value *Number
}

func validateRequestFilters(dates []namedString, numbers []namedNumber) error {
	for _, field := range dates {
		if field.value == nil {
			continue
		}
		if err := validateRequestTimestamp(*field.value); err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
	}
	for _, field := range numbers {
		if field.value == nil {
			continue
		}
		if err := validateRequestDecimal(string(*field.value)); err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
	}
	return nil
}

var requestTimestampPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-][0-9]{2}:[0-9]{2})$`)

func validateRequestTimestamp(s string) error {
	if !requestTimestampPattern.MatchString(s) {
		return fmt.Errorf("invalid RFC3339 timestamp %q", s)
	}
	if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
		return fmt.Errorf("invalid RFC3339 timestamp %q: %w", s, err)
	}
	if s[len(s)-1] != 'Z' {
		zone := s[len(s)-5:]
		hour, _ := strconv.Atoi(zone[:2])
		minute, _ := strconv.Atoi(zone[3:])
		if hour > 23 || minute > 59 {
			return fmt.Errorf("invalid RFC3339 timezone in %q", s)
		}
	}
	return nil
}

var maxDecimalCoefficient = new(big.Int).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

// rust_decimal stores a 96-bit coefficient and a scale of at most 28. Check
// exact representability without rounding or expanding enormous exponents.
func validateRequestDecimal(s string) error {
	if !decimalPattern.MatchString(s) {
		return fmt.Errorf("invalid decimal %q", s)
	}
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(s), "e")
	exponent := int64(0)
	if hasExponent {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 32)
		if err != nil {
			return fmt.Errorf("decimal exponent out of range in %q", s)
		}
	}
	mantissa = strings.TrimPrefix(strings.TrimPrefix(mantissa, "+"), "-")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return nil
	}
	exponent -= int64(len(fraction))
	trimmed := strings.TrimRight(digits, "0")
	exponent += int64(len(digits) - len(trimmed))
	digits = trimmed
	if exponent < -28 || exponent > 29 || int64(len(digits))+max(exponent, 0) > 29 {
		return fmt.Errorf("decimal is not exactly representable as rust_decimal: %q", s)
	}
	if exponent > 0 {
		digits += strings.Repeat("0", int(exponent))
	}
	coefficient, ok := new(big.Int).SetString(digits, 10)
	if !ok || coefficient.Cmp(maxDecimalCoefficient) > 0 {
		return fmt.Errorf("decimal is not exactly representable as rust_decimal: %q", s)
	}
	return nil
}

func validateRequestHex(value, field string, size int) error {
	if value == "" {
		return fmt.Errorf("%s: empty value", field)
	}
	if err := validateHex(value, size); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

func (r EventsRequest) validateRequest() error {
	return validateRequestFilters(
		[]namedString{{"start_date_min", r.StartDateMin}, {"start_date_max", r.StartDateMax}, {"end_date_min", r.EndDateMin}, {"end_date_max", r.EndDateMax}},
		[]namedNumber{{"liquidity_min", r.LiquidityMin}, {"liquidity_max", r.LiquidityMax}, {"volume_min", r.VolumeMin}, {"volume_max", r.VolumeMax}},
	)
}

func (r MarketsRequest) validateRequest() error {
	if err := validateRequestFilters(
		[]namedString{{"start_date_min", r.StartDateMin}, {"start_date_max", r.StartDateMax}, {"end_date_min", r.EndDateMin}, {"end_date_max", r.EndDateMax}},
		[]namedNumber{{"liquidity_num_min", r.LiquidityNumMin}, {"liquidity_num_max", r.LiquidityNumMax}, {"volume_num_min", r.VolumeNumMin}, {"volume_num_max", r.VolumeNumMax}, {"rewards_min_size", r.RewardsMinSize}},
	); err != nil {
		return err
	}
	for i, id := range r.CLOBTokenIDs {
		if err := validateUint256(string(id)); err != nil {
			return fmt.Errorf("clob_token_ids[%d]: %w", i, err)
		}
	}
	for _, list := range []struct {
		name   string
		size   int
		values []string
	}{{"condition_ids", 32, r.ConditionIDs}, {"question_ids", 32, r.QuestionIDs}, {"market_maker_address", 20, r.MarketMakerAddress}} {
		for i, value := range list.values {
			if err := validateRequestHex(value, fmt.Sprintf("%s[%d]", list.name, i), list.size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r PublicProfileRequest) validateRequest() error {
	return validateRequestHex(r.Address, "address", 20)
}

func (r CommentsByUserAddressRequest) validateRequest() error {
	return validateRequestHex(r.UserAddress, "user_address", 20)
}
