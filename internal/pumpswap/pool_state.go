package pumpswap

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// PumpSwap AMM program ID (DEX phase).
const PumpSwapAMMProgramID = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"

// Wrapped SOL mint (quote in PumpSwap SOL pairs).
const SolMintAddress = "So11111111111111111111111111111111111111112"

// PumpSwapPoolState — состояние пула PumpSwap (token/SOL). Резервы читаются из vault token accounts.
type PumpSwapPoolState struct {
	Pool         solana.PublicKey
	TokenMint    solana.PublicKey
	SolMint      solana.PublicKey
	TokenVault   solana.PublicKey
	SolVault     solana.PublicKey
	TokenReserve uint64
	SolReserve   uint64
	TokenDecimals uint8
	SolDecimals   uint8
	Slot         uint64
}

// Pool account layout (Anchor: discriminator 8 + pool_bump 1 + index 2 + creator 32 + base_mint 32 + quote_mint 32 + lp_mint 32 + pool_base_token_account 32 + pool_quote_token_account 32 + lp_supply 8).
const (
	poolDiscriminatorLen = 8
	poolBumpLen          = 1
	poolIndexLen         = 2
	poolCreatorLen       = 32
	poolBaseMintOffset   = poolDiscriminatorLen + poolBumpLen + poolIndexLen + poolCreatorLen // 43
	poolQuoteMintOffset  = poolBaseMintOffset + 32                                             // 75
	poolLpMintOffset     = poolQuoteMintOffset + 32                                            // 107
	poolBaseTokenOffset  = poolLpMintOffset + 32                                               // 139
	poolQuoteTokenOffset = poolBaseTokenOffset + 32                                             // 171
)

// GetPumpSwapPoolState загружает состояние пула: парсит account data, читает резервы из vault token accounts.
func GetPumpSwapPoolState(ctx context.Context, client *rpc.Client, pool solana.PublicKey) (*PumpSwapPoolState, error) {
	resp, err := client.GetAccountInfoWithOpts(ctx, pool, &rpc.GetAccountInfoOpts{
		Encoding:   solana.EncodingBase64,
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, fmt.Errorf("get pool account: %w", err)
	}
	if resp.Value == nil {
		return nil, fmt.Errorf("pool account not found")
	}
	data := resp.Value.Data.GetBinary()
	if len(data) < poolQuoteTokenOffset+32 {
		return nil, fmt.Errorf("invalid pool account size")
	}

	tokenMint := solana.PublicKeyFromBytes(data[poolBaseMintOffset : poolBaseMintOffset+32])
	solMint := solana.PublicKeyFromBytes(data[poolQuoteMintOffset : poolQuoteMintOffset+32])
	tokenVault := solana.PublicKeyFromBytes(data[poolBaseTokenOffset : poolBaseTokenOffset+32])
	solVault := solana.PublicKeyFromBytes(data[poolQuoteTokenOffset : poolQuoteTokenOffset+32])

	tokenReserve, err := getTokenAccountBalance(ctx, client, tokenVault)
	if err != nil {
		return nil, fmt.Errorf("token vault balance: %w", err)
	}
	solReserve, err := getTokenAccountBalance(ctx, client, solVault)
	if err != nil {
		return nil, fmt.Errorf("sol vault balance: %w", err)
	}

	slot := uint64(0)
	if resp.Context.Slot != 0 {
		slot = resp.Context.Slot
	}

	return &PumpSwapPoolState{
		Pool:           pool,
		TokenMint:      tokenMint,
		SolMint:        solMint,
		TokenVault:     tokenVault,
		SolVault:       solVault,
		TokenReserve:   tokenReserve.Amount,
		SolReserve:     solReserve.Amount,
		TokenDecimals:  tokenReserve.Decimals,
		SolDecimals:    solReserve.Decimals,
		Slot:           slot,
	}, nil
}

type tokenBalance struct {
	Amount   uint64
	Decimals uint8
}

func getTokenAccountBalance(ctx context.Context, client *rpc.Client, account solana.PublicKey) (tokenBalance, error) {
	res, err := client.GetTokenAccountBalance(ctx, account, rpc.CommitmentConfirmed)
	if err != nil {
		return tokenBalance{}, err
	}
	if res.Value == nil {
		return tokenBalance{}, fmt.Errorf("token account balance empty")
	}
	amount, err := strconv.ParseUint(res.Value.Amount, 10, 64)
	if err != nil {
		return tokenBalance{}, fmt.Errorf("parse amount %q: %w", res.Value.Amount, err)
	}
	return tokenBalance{Amount: amount, Decimals: res.Value.Decimals}, nil
}
