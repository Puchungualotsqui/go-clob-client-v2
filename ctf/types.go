// Package ctf provides a client for the Conditional Token Framework contracts.
package ctf

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// All integer inputs are unsigned 256-bit values. A zero parent collection ID
// identifies a top-level position.
type ConditionIDRequest struct {
	Oracle           common.Address
	QuestionID       common.Hash
	OutcomeSlotCount *big.Int
}
type CollectionIDRequest struct {
	ParentCollectionID, ConditionID common.Hash
	IndexSet                        *big.Int
}
type PositionIDRequest struct {
	CollateralToken common.Address
	CollectionID    common.Hash
}
type SplitPositionRequest struct {
	CollateralToken                 common.Address
	ParentCollectionID, ConditionID common.Hash
	Partition                       []*big.Int
	Amount                          *big.Int
}
type MergePositionsRequest struct {
	CollateralToken                 common.Address
	ParentCollectionID, ConditionID common.Hash
	Partition                       []*big.Int
	Amount                          *big.Int
}
type RedeemPositionsRequest struct {
	CollateralToken                 common.Address
	ParentCollectionID, ConditionID common.Hash
	IndexSets                       []*big.Int
}
type RedeemNegRiskRequest struct {
	ConditionID common.Hash
	Amounts     []*big.Int
}

type ConditionIDResponse struct{ ConditionID common.Hash }
type CollectionIDResponse struct{ CollectionID common.Hash }
type PositionIDResponse struct{ PositionID *big.Int }
type SplitPositionResponse struct {
	TransactionHash common.Hash
	BlockNumber     uint64
}
type MergePositionsResponse struct {
	TransactionHash common.Hash
	BlockNumber     uint64
}
type RedeemPositionsResponse struct {
	TransactionHash common.Hash
	BlockNumber     uint64
}
type RedeemNegRiskResponse struct {
	TransactionHash common.Hash
	BlockNumber     uint64
}

// BinaryPartition returns independent values for the YES and NO index sets.
func BinaryPartition() []*big.Int { return []*big.Int{big.NewInt(1), big.NewInt(2)} }

func copyAmount(amount *big.Int) *big.Int {
	if amount == nil {
		return nil
	}
	return new(big.Int).Set(amount)
}

// SplitPositionForBinaryMarket copies amount so later changes to it do not affect the request.
func SplitPositionForBinaryMarket(collateral common.Address, condition common.Hash, amount *big.Int) SplitPositionRequest {
	return SplitPositionRequest{CollateralToken: collateral, ConditionID: condition, Partition: BinaryPartition(), Amount: copyAmount(amount)}
}

// MergePositionsForBinaryMarket copies amount so later changes to it do not affect the request.
func MergePositionsForBinaryMarket(collateral common.Address, condition common.Hash, amount *big.Int) MergePositionsRequest {
	return MergePositionsRequest{CollateralToken: collateral, ConditionID: condition, Partition: BinaryPartition(), Amount: copyAmount(amount)}
}
func RedeemPositionsForBinaryMarket(collateral common.Address, condition common.Hash) RedeemPositionsRequest {
	return RedeemPositionsRequest{CollateralToken: collateral, ConditionID: condition, IndexSets: BinaryPartition()}
}
