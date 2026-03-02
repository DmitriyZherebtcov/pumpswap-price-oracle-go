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

var solMintPK solana.PublicKey

func init() {
	solMintPK, _ = solana.PublicKeyFromBase58(SolMintAddress)
}

// GetPumpSwapPoolState загружает состояние пула: парсит account data, читает резервы из vault token accounts.
// Валидирует: пул — строго token/SOL (один из mint = SOL), SOL_reserve >= 1 SOL.
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
		return nil, fmt.Errorf("%w", ErrInvalidLayout)
	}

	baseMint := solana.PublicKeyFromBytes(data[poolBaseMintOffset : poolBaseMintOffset+32])
	quoteMint := solana.PublicKeyFromBytes(data[poolQuoteMintOffset : poolQuoteMintOffset+32])
	baseVault := solana.PublicKeyFromBytes(data[poolBaseTokenOffset : poolBaseTokenOffset+32])
	quoteVault := solana.PublicKeyFromBytes(data[poolQuoteTokenOffset : poolQuoteTokenOffset+32])

	// Жёсткая валидация: пул должен быть token/SOL (один из mint — SOL).
	if !baseMint.Equals(solMintPK) && !quoteMint.Equals(solMintPK) {
		return nil, ErrNotSolPair
	}

	baseBal, err := getTokenAccountBalance(ctx, client, baseVault)
	if err != nil {
		return nil, fmt.Errorf("base vault balance: %w", err)
	}
	quoteBal, err := getTokenAccountBalance(ctx, client, quoteVault)
	if err != nil {
		return nil, fmt.Errorf("quote vault balance: %w", err)
	}

	// Назначаем Token/Sol по mint: какой vault SOL — тот SolReserve.
	var tokenMint, solMint solana.PublicKey
	var tokenVault, solVault solana.PublicKey
	var tokenReserve, solReserve uint64
	var tokenDecimals, solDecimals uint8

	if baseMint.Equals(solMintPK) {
		solMint, solVault, solReserve, solDecimals = baseMint, baseVault, baseBal.Amount, baseBal.Decimals
		tokenMint, tokenVault, tokenReserve, tokenDecimals = quoteMint, quoteVault, quoteBal.Amount, quoteBal.Decimals
	} else {
		tokenMint, tokenVault, tokenReserve, tokenDecimals = baseMint, baseVault, baseBal.Amount, baseBal.Decimals
		solMint, solVault, solReserve, solDecimals = quoteMint, quoteVault, quoteBal.Amount, quoteBal.Decimals
	}

	if solReserve < MinSolReserveLiquidity {
		return nil, ErrSolReserveLow
	}
	if tokenReserve == 0 {
		return nil, ErrZeroLiquidity
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
		TokenReserve:   tokenReserve,
		SolReserve:     solReserve,
		TokenDecimals:  tokenDecimals,
		SolDecimals:    solDecimals,
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
