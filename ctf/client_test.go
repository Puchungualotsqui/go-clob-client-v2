package ctf

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type fakeBackend struct {
	bind.ContractBackend
	bind.DeployBackend
	call                         ethereum.CallMsg
	callResult                   []byte
	callErr, sendErr, receiptErr error
	tx                           *types.Transaction
	receipt                      *types.Receipt
}

func (b *fakeBackend) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	b.call = msg
	return b.callResult, b.callErr
}
func (b *fakeBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	b.tx = tx
	return b.sendErr
}
func (b *fakeBackend) TransactionReceipt(_ context.Context, _ common.Hash) (*types.Receipt, error) {
	return b.receipt, b.receiptErr
}
func (b *fakeBackend) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{1}, nil
}

func word(n int64) []byte                 { return common.LeftPadBytes(big.NewInt(n).Bytes(), 32) }
func addressWord(a common.Address) []byte { return common.LeftPadBytes(a.Bytes(), 32) }
func calldata(signature string, words ...[]byte) []byte {
	data := append([]byte(nil), crypto.Keccak256([]byte(signature))[:4]...)
	for _, w := range words {
		data = append(data, w...)
	}
	return data
}
func checkData(t *testing.T, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("calldata\n got %x\nwant %x", got, want)
	}
}
func txOpts() *bind.TransactOpts {
	return &bind.TransactOpts{From: common.HexToAddress("0x1234"), Nonce: big.NewInt(7), GasPrice: big.NewInt(1), GasLimit: 500000, Signer: func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil }}
}
func TestReads(t *testing.T) {
	b := &fakeBackend{callResult: word(42)}
	c, err := New(b, 137)
	if err != nil {
		t.Fatal(err)
	}
	oracle := common.HexToAddress("0x1234")
	question := common.HexToHash("0x4567")
	parent := common.HexToHash("0x9876")
	condition, err := c.ConditionID(context.Background(), ConditionIDRequest{oracle, question, big.NewInt(2)})
	if err != nil || condition.ConditionID != common.BigToHash(big.NewInt(42)) {
		t.Fatalf("condition: %+v %v", condition, err)
	}
	checkData(t, b.call.Data, calldata("getConditionId(address,bytes32,uint256)", addressWord(oracle), question[:], word(2)))
	if *b.call.To != common.HexToAddress("0x4D97DCd97eC945f40cF65F87097ACe5EA0476045") {
		t.Fatal("wrong CTF address")
	}
	collection, err := c.CollectionID(context.Background(), CollectionIDRequest{parent, question, big.NewInt(1)})
	if err != nil || collection.CollectionID != condition.ConditionID {
		t.Fatalf("collection: %+v %v", collection, err)
	}
	checkData(t, b.call.Data, calldata("getCollectionId(bytes32,bytes32,uint256)", parent[:], question[:], word(1)))
	position, err := c.PositionID(context.Background(), PositionIDRequest{oracle, question})
	if err != nil || position.PositionID.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("position: %+v %v", position, err)
	}
	checkData(t, b.call.Data, calldata("getPositionId(address,bytes32)", addressWord(oracle), question[:]))
	b.callResult = []byte{1}
	if _, err = c.PositionID(context.Background(), PositionIDRequest{oracle, question}); err == nil {
		t.Fatal("expected malformed result error")
	}
	b.callErr = errors.New("rpc failure")
	if _, err = c.PositionID(context.Background(), PositionIDRequest{oracle, question}); !errors.Is(err, b.callErr) {
		t.Fatalf("missing RPC error: %v", err)
	}
}
func TestTransactions(t *testing.T) {
	b := &fakeBackend{receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(123)}}
	c, err := WithNegRisk(b, 80002)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	collateral := common.HexToAddress("0x1234")
	condition := common.HexToHash("0x9876")
	zero := common.Hash{}
	amount := big.NewInt(999)
	cases := []struct {
		name, signature string
		target          common.Address
		words           [][]byte
		run             func() (common.Hash, uint64, error)
	}{
		{"split", "splitPosition(address,bytes32,bytes32,uint256[],uint256)", common.HexToAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"), [][]byte{addressWord(collateral), zero[:], condition[:], word(160), word(999), word(2), word(1), word(2)}, func() (common.Hash, uint64, error) {
			r, e := c.SplitPosition(ctx, txOpts(), SplitPositionForBinaryMarket(collateral, condition, amount))
			return r.TransactionHash, r.BlockNumber, e
		}},
		{"merge", "mergePositions(address,bytes32,bytes32,uint256[],uint256)", common.HexToAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"), [][]byte{addressWord(collateral), zero[:], condition[:], word(160), word(999), word(2), word(1), word(2)}, func() (common.Hash, uint64, error) {
			r, e := c.MergePositions(ctx, txOpts(), MergePositionsForBinaryMarket(collateral, condition, amount))
			return r.TransactionHash, r.BlockNumber, e
		}},
		{"redeem", "redeemPositions(address,bytes32,bytes32,uint256[])", common.HexToAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"), [][]byte{addressWord(collateral), zero[:], condition[:], word(128), word(2), word(1), word(2)}, func() (common.Hash, uint64, error) {
			r, e := c.RedeemPositions(ctx, txOpts(), RedeemPositionsForBinaryMarket(collateral, condition))
			return r.TransactionHash, r.BlockNumber, e
		}},
		{"negative risk", "redeemPositions(bytes32,uint256[])", common.HexToAddress("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"), [][]byte{condition[:], word(64), word(2), word(3), word(4)}, func() (common.Hash, uint64, error) {
			r, e := c.RedeemNegRisk(ctx, txOpts(), RedeemNegRiskRequest{condition, []*big.Int{big.NewInt(3), big.NewInt(4)}})
			return r.TransactionHash, r.BlockNumber, e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, block, err := tc.run()
			if err != nil {
				t.Fatal(err)
			}
			if block != 123 || hash != b.tx.Hash() || *b.tx.To() != tc.target {
				t.Fatalf("wrong result: %s %d %v", hash, block, b.tx.To())
			}
			checkData(t, b.tx.Data(), calldata(tc.signature, tc.words...))
		})
	}
}
func TestErrorsAndConvenience(t *testing.T) {
	b := &fakeBackend{receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1)}}
	if _, err := New(b, 1); err == nil {
		t.Fatal("unsupported chain")
	}
	if _, err := New(nil, 137); err == nil {
		t.Fatal("nil backend")
	}
	c, _ := New(b, 137)
	if c.Provider() != b {
		t.Fatal("provider mismatch")
	}
	if _, err := c.RedeemNegRisk(context.Background(), txOpts(), RedeemNegRiskRequest{}); err == nil {
		t.Fatal("missing adapter")
	}
	for _, n := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 256)} {
		if _, err := c.ConditionID(context.Background(), ConditionIDRequest{OutcomeSlotCount: n}); err == nil {
			t.Fatalf("accepted %v", n)
		}
	}
	req := SplitPositionForBinaryMarket(common.Address{}, common.Hash{}, big.NewInt(1))
	req.Partition[0].SetInt64(9)
	if BinaryPartition()[0].Int64() != 1 {
		t.Fatal("partition shares mutable state")
	}
	req = SplitPositionForBinaryMarket(common.Address{}, common.Hash{}, big.NewInt(1))
	req.Partition[0] = nil
	if _, err := c.SplitPosition(context.Background(), txOpts(), req); err == nil {
		t.Fatal("nil partition element")
	}
	req = SplitPositionForBinaryMarket(common.Address{}, common.Hash{}, big.NewInt(1))
	if _, err := c.SplitPosition(context.Background(), nil, req); err == nil {
		t.Fatal("missing signer")
	}
	opts := txOpts()
	opts.NoSend = true
	if _, err := c.SplitPosition(context.Background(), opts, req); err == nil {
		t.Fatal("NoSend accepted")
	}
	b.sendErr = errors.New("send failed")
	if _, err := c.SplitPosition(context.Background(), txOpts(), req); !errors.Is(err, b.sendErr) {
		t.Fatalf("send error: %v", err)
	}
	b.sendErr = nil
	b.receipt.Status = types.ReceiptStatusFailed
	r, err := c.SplitPosition(context.Background(), txOpts(), req)
	if err == nil || !strings.Contains(err.Error(), "reverted") || r.TransactionHash != b.tx.Hash() {
		t.Fatalf("revert: %+v %v", r, err)
	}
	b.receipt.Status = types.ReceiptStatusSuccessful
	b.receipt.BlockNumber = nil
	r, err = c.SplitPosition(context.Background(), txOpts(), req)
	if err == nil || r.TransactionHash != b.tx.Hash() {
		t.Fatalf("missing block: %+v %v", r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.receipt = nil
	b.receiptErr = ethereum.NotFound
	r, err = c.SplitPosition(ctx, txOpts(), req)
	if !errors.Is(err, context.Canceled) || r.TransactionHash != b.tx.Hash() {
		t.Fatalf("cancelled receipt: %+v %v", r, err)
	}
}
