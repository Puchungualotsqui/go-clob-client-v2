package bridge

import (
	"encoding/json"
	"fmt"
)

// DepositTransactionStatus is the bridge's transaction state.
type DepositTransactionStatus string

const (
	DepositDetected   DepositTransactionStatus = "DEPOSIT_DETECTED"
	Processing        DepositTransactionStatus = "PROCESSING"
	OriginTxConfirmed DepositTransactionStatus = "ORIGIN_TX_CONFIRMED"
	Submitted         DepositTransactionStatus = "SUBMITTED"
	Completed         DepositTransactionStatus = "COMPLETED"
	Failed            DepositTransactionStatus = "FAILED"
)

func (s *DepositTransactionStatus) UnmarshalJSON(b []byte) error {
	var value string
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	switch DepositTransactionStatus(value) {
	case DepositDetected, Processing, OriginTxConfirmed, Submitted, Completed, Failed:
		*s = DepositTransactionStatus(value)
		return nil
	default:
		return fmt.Errorf("unknown deposit transaction status %q", value)
	}
}
