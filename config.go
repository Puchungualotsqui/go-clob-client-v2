package clob

import "time"

const (
	Polygon             uint64 = 137
	Amoy                uint64 = 80002
	PrivateKeyVar              = "POLYMARKET_PRIVATE_KEY"
	DefaultCLOBHost            = "https://clob-v2.polymarket.com"
	DefaultGeoblockHost        = "https://polymarket.com"
	USDCDecimals               = 6
	LotSizeScale               = 2
	TerminalCursor             = "LTE="
)

type ContractConfig struct {
	Exchange, Collateral, ConditionalTokens Address
	NegRiskAdapter, ExchangeV2, ExchangeV3  *Address
}
type WalletContractConfig struct {
	ProxyFactory *Address
	SafeFactory  Address
}

func configAddress(s string) Address {
	a, err := ParseAddress(s)
	if err != nil {
		panic(err) // Configuration addresses are compile-time constants.
	}
	return a
}
func addressPtr(s string) *Address { a := configAddress(s); return &a }

// ContractConfigFor returns a fresh configuration, or false for an unsupported chain.
func ContractConfigFor(chainID uint64, negRisk bool) (ContractConfig, bool) {
	if chainID != Polygon && chainID != Amoy {
		return ContractConfig{}, false
	}
	c := ContractConfig{Exchange: configAddress("0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"), Collateral: configAddress("0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"), ConditionalTokens: configAddress("0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"), ExchangeV2: addressPtr("0xE111180000d2663C0091e4f400237545B87B996B"), ExchangeV3: addressPtr("0xe3333700cA9d93003F00f0F71f8515005F6c00Aa")}
	if chainID == Amoy {
		c.Exchange = configAddress("0xdFE02Eb6733538f8Ea35D585af8DE5958AD99E40")
		c.ConditionalTokens = configAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB")
		c.ExchangeV3 = addressPtr("0x9fE6e61422AdB6F610d8597F9684b16912D50C3D")
	}
	if negRisk {
		c.Exchange = configAddress("0xC5d563A36AE78145C45a50134d48A1215220f80a")
		c.NegRiskAdapter = addressPtr("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296")
		c.ExchangeV2 = addressPtr("0xe2222d279d744050d28e00520010520000310F59")
	}
	return c, true
}
func WalletContractConfigFor(chainID uint64) (WalletContractConfig, bool) {
	if chainID != Polygon && chainID != Amoy {
		return WalletContractConfig{}, false
	}
	c := WalletContractConfig{SafeFactory: configAddress("0xaacFeEa03eb1561C4e67d661e40682Bd20E3541b")}
	if chainID == Polygon {
		c.ProxyFactory = addressPtr("0xaB45c5A4B0c941a2F231C04C3f49182e1A254052")
	}
	return c, true
}

type Config struct {
	UseServerTime     bool
	GeoblockHost      *string
	BuilderCode       *B256
	HeartbeatInterval time.Duration
}

func DefaultConfig() Config { return Config{HeartbeatInterval: 5 * time.Second} }

type ReconnectConfig struct {
	MaxAttempts                *uint32
	InitialBackoff, MaxBackoff time.Duration
	BackoffMultiplier          float64
}
type WebSocketConfig struct {
	HeartbeatInterval, HeartbeatTimeout time.Duration
	Reconnect                           ReconnectConfig
}

func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{5 * time.Second, 15 * time.Second, ReconnectConfig{nil, time.Second, 60 * time.Second, 2}}
}
