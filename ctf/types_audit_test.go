package ctf

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestBinaryMarketAmountsAreIndependent(t *testing.T) {
	amount := big.NewInt(7)
	split := SplitPositionForBinaryMarket(common.Address{}, common.Hash{}, amount)
	merge := MergePositionsForBinaryMarket(common.Address{}, common.Hash{}, amount)
	amount.SetInt64(9)
	if split.Amount.Int64() != 7 || merge.Amount.Int64() != 7 {
		t.Fatalf("amounts share caller state: %v %v", split.Amount, merge.Amount)
	}
	split.Amount.SetInt64(3)
	if merge.Amount.Int64() != 7 {
		t.Fatal("requests share amount state")
	}
	if SplitPositionForBinaryMarket(common.Address{}, common.Hash{}, nil).Amount != nil || MergePositionsForBinaryMarket(common.Address{}, common.Hash{}, nil).Amount != nil {
		t.Fatal("nil amount should remain invalid")
	}
}

func TestBinaryMarketLargeAmountAndPartitionIndependence(t *testing.T) {
	amount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	want := new(big.Int).Set(amount)
	split := SplitPositionForBinaryMarket(common.Address{}, common.Hash{}, amount)
	merge := MergePositionsForBinaryMarket(common.Address{}, common.Hash{}, amount)
	redeem := RedeemPositionsForBinaryMarket(common.Address{}, common.Hash{})
	amount.SetInt64(0)
	split.Partition[0].SetInt64(9)
	if split.Amount.Cmp(want) != 0 || merge.Amount.Cmp(want) != 0 || merge.Partition[0].Int64() != 1 || redeem.IndexSets[0].Int64() != 1 || split.Partition[1].Int64() != 2 {
		t.Fatalf("binary market values not independent: split=%+v merge=%+v redeem=%+v", split, merge, redeem)
	}
}

func TestAllCTFUint256InputsValidated(t *testing.T) {
	c, err := WithNegRisk(&fakeBackend{}, 137)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, value := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 256)} {
		cases := []struct {
			field string
			run   func() error
		}{
			{"outcome slot count", func() error { _, e := c.ConditionID(ctx, ConditionIDRequest{OutcomeSlotCount: value}); return e }},
			{"index set", func() error { _, e := c.CollectionID(ctx, CollectionIDRequest{IndexSet: value}); return e }},
			{"partition[0]", func() error {
				_, e := c.SplitPosition(ctx, nil, SplitPositionRequest{Partition: []*big.Int{value}, Amount: big.NewInt(1)})
				return e
			}},
			{"amount", func() error { _, e := c.SplitPosition(ctx, nil, SplitPositionRequest{Amount: value}); return e }},
			{"partition[0]", func() error {
				_, e := c.MergePositions(ctx, nil, MergePositionsRequest{Partition: []*big.Int{value}, Amount: big.NewInt(1)})
				return e
			}},
			{"amount", func() error { _, e := c.MergePositions(ctx, nil, MergePositionsRequest{Amount: value}); return e }},
			{"index sets[0]", func() error {
				_, e := c.RedeemPositions(ctx, nil, RedeemPositionsRequest{IndexSets: []*big.Int{value}})
				return e
			}},
			{"amounts[0]", func() error {
				_, e := c.RedeemNegRisk(ctx, nil, RedeemNegRiskRequest{Amounts: []*big.Int{value}})
				return e
			}},
		}
		for _, tc := range cases {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Errorf("%s %v: %v", tc.field, value, err)
			}
		}
	}
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for _, value := range []*big.Int{big.NewInt(0), max} {
		if err := uint256("value", value); err != nil {
			t.Errorf("valid %v: %v", value, err)
		}
	}
}
