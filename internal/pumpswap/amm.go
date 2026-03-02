package pumpswap

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// PumpSwap AMM program ID (pool program, not bonding curve)
const PumpSwapAMMProgramID = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"

// Pool account layout (from IDL: discriminator 8 + pool_bump 1 + index 2 + creator 32 + base_mint 32 + quote_mint 32 + lp_mint 32 + pool_base_token_account 32 + pool_quote_token_account 32 + lp_supply 8)
const (
	poolDiscriminatorLen = 8
	poolBumpLen          = 1
	poolIndexLen         = 2
	poolCreatorLen       = 32
	poolBaseMintOffset   = poolDiscriminatorLen + poolBumpLen + poolIndexLen + poolCreatorLen // 43
	poolQuoteMintOffset  = poolBaseMintOffset + 32                                             // 75
	poolLpMintOffset     = poolQuoteMintOffset + 32                                            // 107
	poolBaseTokenOffset = poolLpMintOffset + 32                                               // 139
	poolQuoteTokenOffset = poolBaseTokenOffset + 32                                            // 171
)

// PoolReserves holds pool token account addresses and their balances (for price).
type PoolReserves struct {
	PoolAddress           solana.PublicKey
	BaseTokenAccount      solana.PublicKey
	QuoteTokenAccount     solana.PublicKey
	BaseAmount            uint64
	QuoteAmount           uint64
	BaseDecimals          uint8
	QuoteDecimals         uint8
}

// FindPumpSwapPoolByBaseMint returns the first PumpSwap AMM pool where base_mint equals the given mint (token), quote is typically SOL.
func FindPumpSwapPoolByBaseMint(client *rpc.Client, mint solana.PublicKey) (*PoolReserves, error) {
	ctx := context.Background()
	programID, err := solana.PublicKeyFromBase58(PumpSwapAMMProgramID)
	if err != nil {
		return nil, err
	}

	// Без DataSize: размер аккаунта пула может отличаться; фильтруем только по base_mint.
	opts := &rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
		Filters: []rpc.RPCFilter{
			{
				Memcmp: &rpc.RPCFilterMemcmp{
					Offset: poolBaseMintOffset,
					Bytes:  solana.Base58(mint.String()),
				},
			},
		},
		DataSlice: nil,
	}
	accounts, err := client.GetProgramAccountsWithOpts(ctx, programID, opts)
	if err != nil {
		return nil, fmt.Errorf("GetProgramAccounts: %w", err)
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no pool found for base_mint %s", mint.String())
	}

	// Выбираем пул с максимальной ликвидностью (quote = SOL), чтобы не взять пустой/тестовый пул
	var best *PoolReserves
	for _, acct := range accounts {
		data := acct.Account.Data.GetBinary()
		if len(data) < poolQuoteTokenOffset+32+8 {
			continue
		}
		poolAddr := acct.Pubkey
		baseTokenAccount := solana.PublicKeyFromBytes(data[poolBaseTokenOffset : poolBaseTokenOffset+32])
		quoteTokenAccount := solana.PublicKeyFromBytes(data[poolQuoteTokenOffset : poolQuoteTokenOffset+32])

		baseAmount, quoteAmount, err := getTokenAccountAmounts(client, ctx, baseTokenAccount, quoteTokenAccount)
		if err != nil {
			continue
		}

		// Игнорируем пустые пулы (нулевые резервы → цена 0)
		if quoteAmount == 0 || baseAmount == 0 {
			continue
		}
		cur := &PoolReserves{
			PoolAddress:       poolAddr,
			BaseTokenAccount:  baseTokenAccount,
			QuoteTokenAccount: quoteTokenAccount,
			BaseAmount:        baseAmount,
			QuoteAmount:       quoteAmount,
			BaseDecimals:      6,
			QuoteDecimals:     9,
		}
		if best == nil || quoteAmount > best.QuoteAmount {
			best = cur
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no pool with valid reserves for base_mint %s", mint.String())
	}
	return best, nil
}

func getTokenAccountAmounts(client *rpc.Client, ctx context.Context, baseAccount, quoteAccount solana.PublicKey) (baseAmount, quoteAmount uint64, err error) {
	baseAmount, err = getTokenAccountAmount(client, ctx, baseAccount)
	if err != nil {
		return 0, 0, fmt.Errorf("base token account: %w", err)
	}
	quoteAmount, err = getTokenAccountAmount(client, ctx, quoteAccount)
	if err != nil {
		return 0, 0, fmt.Errorf("quote token account: %w", err)
	}
	return baseAmount, quoteAmount, nil
}

// getTokenAccountAmount использует getTokenAccountBalance RPC для надёжного чтения баланса (без парсинга layout).
func getTokenAccountAmount(client *rpc.Client, ctx context.Context, account solana.PublicKey) (uint64, error) {
	res, err := client.GetTokenAccountBalance(ctx, account, rpc.CommitmentConfirmed)
	if err != nil {
		return 0, err
	}
	if res.Value == nil {
		return 0, fmt.Errorf("token account balance empty")
	}
	amount, err := strconv.ParseUint(res.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse amount %q: %w", res.Value.Amount, err)
	}
	return amount, nil
}

// UpdatePoolReserves fetches current base/quote token account balances for the pool.
func UpdatePoolReserves(client *rpc.Client, ctx context.Context, pool *PoolReserves) (baseAmount, quoteAmount uint64, err error) {
	return getTokenAccountAmounts(client, ctx, pool.BaseTokenAccount, pool.QuoteTokenAccount)
}

// PriceFromPool returns price in SOL per 1 base token (base = token, quote = SOL).
func PriceFromPool(baseAmount, quoteAmount uint64, baseDecimals, quoteDecimals uint8) float64 {
	if baseAmount == 0 {
		return 0
	}
	baseDiv := 1e6
	if baseDecimals == 9 {
		baseDiv = 1e9
	} else if baseDecimals != 6 {
		for i := uint8(0); i < baseDecimals; i++ {
			baseDiv *= 10
		}
	}
	quoteDiv := 1e9
	if quoteDecimals != 9 {
		quoteDiv = 1
		for i := uint8(0); i < quoteDecimals; i++ {
			quoteDiv *= 10
		}
	}
	return (float64(quoteAmount) / float64(quoteDiv)) / (float64(baseAmount) / float64(baseDiv))
}
