package data

func (v *Health) UnmarshalJSON(b []byte) error {
	type plain Health
	return decodeResponse(b, (*plain)(v))
}

func (v *ApiError) UnmarshalJSON(b []byte) error {
	type plain ApiError
	return decodeResponse(b, (*plain)(v))
}

func (v *ClosedPosition) UnmarshalJSON(b []byte) error {
	type plain ClosedPosition
	return decodeResponse(b, (*plain)(v))
}

func (v *MetaHolder) UnmarshalJSON(b []byte) error {
	type plain MetaHolder
	return decodeResponse(b, (*plain)(v))
}

func (v *Traded) UnmarshalJSON(b []byte) error {
	type plain Traded
	return decodeResponse(b, (*plain)(v))
}

func (v *Value) UnmarshalJSON(b []byte) error {
	type plain Value
	return decodeResponse(b, (*plain)(v))
}

func (v *OpenInterest) UnmarshalJSON(b []byte) error {
	type plain OpenInterest
	return decodeResponse(b, (*plain)(v))
}

func (v *MarketVolume) UnmarshalJSON(b []byte) error {
	type plain MarketVolume
	return decodeResponse(b, (*plain)(v))
}
