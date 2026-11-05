package ctf

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const conditionalTokensABI = `[
 {"type":"function","name":"getConditionId","stateMutability":"pure","inputs":[{"type":"address"},{"type":"bytes32"},{"type":"uint256"}],"outputs":[{"type":"bytes32"}]},
 {"type":"function","name":"getCollectionId","stateMutability":"view","inputs":[{"type":"bytes32"},{"type":"bytes32"},{"type":"uint256"}],"outputs":[{"type":"bytes32"}]},
 {"type":"function","name":"getPositionId","stateMutability":"pure","inputs":[{"type":"address"},{"type":"bytes32"}],"outputs":[{"type":"uint256"}]},
 {"type":"function","name":"splitPosition","stateMutability":"nonpayable","inputs":[{"type":"address"},{"type":"bytes32"},{"type":"bytes32"},{"type":"uint256[]"},{"type":"uint256"}],"outputs":[]},
 {"type":"function","name":"mergePositions","stateMutability":"nonpayable","inputs":[{"type":"address"},{"type":"bytes32"},{"type":"bytes32"},{"type":"uint256[]"},{"type":"uint256"}],"outputs":[]},
 {"type":"function","name":"redeemPositions","stateMutability":"nonpayable","inputs":[{"type":"address"},{"type":"bytes32"},{"type":"bytes32"},{"type":"uint256[]"}],"outputs":[]}
]`
const negRiskABI = `[{"type":"function","name":"redeemPositions","stateMutability":"nonpayable","inputs":[{"type":"bytes32"},{"type":"uint256[]"}],"outputs":[]}]`

// Backend is implemented by *ethclient.Client. Transactions require a backend
// capable of sending signed transactions and retrieving receipts.
type Backend interface {
	bind.ContractBackend
	bind.DeployBackend
}

type Client struct {
	backend           Backend
	contract, adapter *bind.BoundContract
}

// New creates a client for Polygon (137) or Polygon Amoy (80002).
func New(backend Backend, chainID uint64) (*Client, error) { return newClient(backend, chainID, false) }

// WithNegRisk creates a client with the negative-risk adapter enabled.
func WithNegRisk(backend Backend, chainID uint64) (*Client, error) {
	return newClient(backend, chainID, true)
}
func newClient(backend Backend, chainID uint64, negRisk bool) (*Client, error) {
	if backend == nil {
		return nil, errors.New("ctf: nil backend")
	}
	var address string
	switch chainID {
	case 137:
		address = "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"
	case 80002:
		address = "0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"
	default:
		return nil, fmt.Errorf("ctf: contract configuration not found for chain ID %d", chainID)
	}
	parsed, err := abi.JSON(strings.NewReader(conditionalTokensABI))
	if err != nil {
		return nil, fmt.Errorf("ctf: parse contract ABI: %w", err)
	}
	c := &Client{backend: backend, contract: bind.NewBoundContract(common.HexToAddress(address), parsed, backend, backend, backend)}
	if negRisk {
		parsed, err = abi.JSON(strings.NewReader(negRiskABI))
		if err != nil {
			return nil, fmt.Errorf("ctf: parse adapter ABI: %w", err)
		}
		c.adapter = bind.NewBoundContract(common.HexToAddress("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"), parsed, backend, backend, backend)
	}
	return c, nil
}

// Provider returns the underlying backend.
func (c *Client) Provider() Backend { return c.backend }

func uint256(name string, n *big.Int) error {
	if n == nil || n.Sign() < 0 || n.BitLen() > 256 {
		return fmt.Errorf("ctf: %s must be a non-nil uint256", name)
	}
	return nil
}
func uint256s(name string, values []*big.Int) error {
	for i, value := range values {
		if err := uint256(fmt.Sprintf("%s[%d]", name, i), value); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) call(ctx context.Context, method string, args ...interface{}) (interface{}, error) {
	var out []interface{}
	if err := c.contract.Call(&bind.CallOpts{Context: ctx}, &out, method, args...); err != nil {
		return nil, fmt.Errorf("ctf: %s call: %w", method, err)
	}
	if len(out) != 1 {
		return nil, fmt.Errorf("ctf: %s returned %d values, expected 1", method, len(out))
	}
	return out[0], nil
}
func (c *Client) ConditionID(ctx context.Context, r ConditionIDRequest) (ConditionIDResponse, error) {
	if err := uint256("outcome slot count", r.OutcomeSlotCount); err != nil {
		return ConditionIDResponse{}, err
	}
	v, err := c.call(ctx, "getConditionId", r.Oracle, r.QuestionID, r.OutcomeSlotCount)
	if err != nil {
		return ConditionIDResponse{}, err
	}
	id, ok := v.([32]byte)
	if !ok {
		return ConditionIDResponse{}, errors.New("ctf: invalid condition ID response")
	}
	return ConditionIDResponse{ConditionID: common.Hash(id)}, nil
}
func (c *Client) CollectionID(ctx context.Context, r CollectionIDRequest) (CollectionIDResponse, error) {
	if err := uint256("index set", r.IndexSet); err != nil {
		return CollectionIDResponse{}, err
	}
	v, err := c.call(ctx, "getCollectionId", r.ParentCollectionID, r.ConditionID, r.IndexSet)
	if err != nil {
		return CollectionIDResponse{}, err
	}
	id, ok := v.([32]byte)
	if !ok {
		return CollectionIDResponse{}, errors.New("ctf: invalid collection ID response")
	}
	return CollectionIDResponse{CollectionID: common.Hash(id)}, nil
}
func (c *Client) PositionID(ctx context.Context, r PositionIDRequest) (PositionIDResponse, error) {
	v, err := c.call(ctx, "getPositionId", r.CollateralToken, r.CollectionID)
	if err != nil {
		return PositionIDResponse{}, err
	}
	id, ok := v.(*big.Int)
	if !ok || id == nil {
		return PositionIDResponse{}, errors.New("ctf: invalid position ID response")
	}
	return PositionIDResponse{PositionID: id}, nil
}

// transact returns the submitted hash even on a receipt error, so callers can
// reconcile an indeterminate outcome without blindly resubmitting.
func (c *Client) transact(ctx context.Context, opts *bind.TransactOpts, contract *bind.BoundContract, method string, args ...interface{}) (common.Hash, uint64, error) {
	if ctx == nil {
		return common.Hash{}, 0, errors.New("ctf: nil context")
	}
	if opts == nil || opts.Signer == nil {
		return common.Hash{}, 0, errors.New("ctf: transaction signer required")
	}
	if opts.NoSend {
		return common.Hash{}, 0, errors.New("ctf: NoSend is incompatible with waiting for a receipt")
	}
	// Never mutate caller-owned options; context applies to both sending and waiting.
	txOpts := *opts
	txOpts.Context = ctx
	tx, err := contract.Transact(&txOpts, method, args...)
	if err != nil {
		return common.Hash{}, 0, fmt.Errorf("ctf: send %s transaction: %w", method, err)
	}
	hash := tx.Hash()
	receipt, err := bind.WaitMined(ctx, c.backend, tx)
	if err != nil {
		return hash, 0, fmt.Errorf("ctf: wait for transaction %s: %w", hash, err)
	}
	if receipt == nil || receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() {
		return hash, 0, fmt.Errorf("ctf: transaction %s has no valid block number", hash)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return hash, 0, fmt.Errorf("ctf: transaction %s reverted", hash)
	}
	return hash, receipt.BlockNumber.Uint64(), nil
}
func (c *Client) SplitPosition(ctx context.Context, opts *bind.TransactOpts, r SplitPositionRequest) (SplitPositionResponse, error) {
	if err := uint256s("partition", r.Partition); err != nil {
		return SplitPositionResponse{}, err
	}
	if err := uint256("amount", r.Amount); err != nil {
		return SplitPositionResponse{}, err
	}
	h, n, err := c.transact(ctx, opts, c.contract, "splitPosition", r.CollateralToken, r.ParentCollectionID, r.ConditionID, r.Partition, r.Amount)
	return SplitPositionResponse{h, n}, err
}
func (c *Client) MergePositions(ctx context.Context, opts *bind.TransactOpts, r MergePositionsRequest) (MergePositionsResponse, error) {
	if err := uint256s("partition", r.Partition); err != nil {
		return MergePositionsResponse{}, err
	}
	if err := uint256("amount", r.Amount); err != nil {
		return MergePositionsResponse{}, err
	}
	h, n, err := c.transact(ctx, opts, c.contract, "mergePositions", r.CollateralToken, r.ParentCollectionID, r.ConditionID, r.Partition, r.Amount)
	return MergePositionsResponse{h, n}, err
}
func (c *Client) RedeemPositions(ctx context.Context, opts *bind.TransactOpts, r RedeemPositionsRequest) (RedeemPositionsResponse, error) {
	if err := uint256s("index sets", r.IndexSets); err != nil {
		return RedeemPositionsResponse{}, err
	}
	h, n, err := c.transact(ctx, opts, c.contract, "redeemPositions", r.CollateralToken, r.ParentCollectionID, r.ConditionID, r.IndexSets)
	return RedeemPositionsResponse{h, n}, err
}
func (c *Client) RedeemNegRisk(ctx context.Context, opts *bind.TransactOpts, r RedeemNegRiskRequest) (RedeemNegRiskResponse, error) {
	if c.adapter == nil {
		return RedeemNegRiskResponse{}, errors.New("ctf: negative-risk adapter unavailable; use WithNegRisk")
	}
	if err := uint256s("amounts", r.Amounts); err != nil {
		return RedeemNegRiskResponse{}, err
	}
	h, n, err := c.transact(ctx, opts, c.adapter, "redeemPositions", r.ConditionID, r.Amounts)
	return RedeemNegRiskResponse{h, n}, err
}
