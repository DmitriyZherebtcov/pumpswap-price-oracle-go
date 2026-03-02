package pumpswap

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// MinSolReserveLiquidity — минимум 1 SOL в резервах пула (anti-dust). Raw units для 9 decimals = 1e9.
const MinSolReserveLiquidity = 1_000_000_000

var solMintPubkey solana.PublicKey

func init() {
	solMintPubkey, _ = solana.PublicKeyFromBase58(SolMintAddress)
}

// FindPumpSwapSolPoolByScan находит пул PumpSwap строго token/SOL через getProgramAccounts и memcmp (без PDA).
// Фильтры: (base_mint=token, quote_mint=SOL) или (base_mint=SOL, quote_mint=token).
// Выбирается пул с максимальной ликвидностью SOL, SOL_reserve >= 1 SOL.
func FindPumpSwapSolPoolByScan(ctx context.Context, client *rpc.Client, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	programID, err := solana.PublicKeyFromBase58(PumpSwapAMMProgramID)
	if err != nil {
		return solana.PublicKey{}, err
	}

	// 1) Пул: base=token, quote=SOL
	pool, solReserve, err := scanPoolsWithFilters(ctx, client, programID, tokenMint, solMintPubkey, tokenMint)
	if err != nil {
		pool = solana.PublicKey{}
		solReserve = 0
	}

	// 2) Пул: base=SOL, quote=token
	pool2, solReserve2, err2 := scanPoolsWithFilters(ctx, client, programID, solMintPubkey, tokenMint, tokenMint)
	if err2 != nil {
		pool2 = solana.PublicKey{}
		solReserve2 = 0
	}

	// Выбираем пул с максимальной ликвидностью SOL
	if solReserve >= solReserve2 && !pool.IsZero() {
		return pool, nil
	}
	if !pool2.IsZero() {
		return pool2, nil
	}

	// 3) Fallback: без memcmp, локальная фильтрация
	pool3, err3 := scanPoolsFallback(ctx, client, programID, tokenMint)
	if err3 == nil && !pool3.IsZero() {
		return pool3, nil
	}

	return solana.PublicKey{}, fmt.Errorf("%w: mint %s", ErrPoolNotFound, tokenMint.String())
}

// scanPoolsWithFilters: getProgramAccounts с memcmp(base_mint=baseMint, quote_mint=quoteMint), декод, фильтр liquidity >= 1 SOL, макс SOL.
func scanPoolsWithFilters(ctx context.Context, client *rpc.Client, programID, baseMint, quoteMint, tokenMint solana.PublicKey) (bestPool solana.PublicKey, bestSolReserve uint64, err error) {
	opts := &rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
		Filters: []rpc.RPCFilter{
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: poolBaseMintOffset, Bytes: solana.Base58(baseMint.String())}},
			{Memcmp: &rpc.RPCFilterMemcmp{Offset: poolQuoteMintOffset, Bytes: solana.Base58(quoteMint.String())}},
		},
	}
	accounts, err := client.GetProgramAccountsWithOpts(ctx, programID, opts)
	if err != nil {
		return solana.PublicKey{}, 0, err
	}

	for _, acct := range accounts {
		data := acct.Account.Data.GetBinary()
		if len(data) < poolQuoteTokenOffset+32 {
			continue
		}
		poolAddr := acct.Pubkey
		state, err := GetPumpSwapPoolState(ctx, client, poolAddr)
		if err != nil {
			continue
		}
		if !isValidSolPair(state, tokenMint) {
			continue
		}
		if state.SolReserve < MinSolReserveLiquidity {
			continue
		}
		if state.SolReserve > bestSolReserve {
			bestSolReserve = state.SolReserve
			bestPool = state.Pool
		}
	}
	return bestPool, bestSolReserve, nil
}

// scanPoolsFallback: getProgramAccounts без фильтров, локальная фильтрация token/SOL и liquidity >= 1 SOL.
func scanPoolsFallback(ctx context.Context, client *rpc.Client, programID, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	opts := &rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
	}
	accounts, err := client.GetProgramAccountsWithOpts(ctx, programID, opts)
	if err != nil {
		return solana.PublicKey{}, err
	}

	var bestPool solana.PublicKey
	var bestSolReserve uint64

	for _, acct := range accounts {
		data := acct.Account.Data.GetBinary()
		if len(data) < poolQuoteTokenOffset+32 {
			continue
		}
		baseMint := solana.PublicKeyFromBytes(data[poolBaseMintOffset : poolBaseMintOffset+32])
		quoteMint := solana.PublicKeyFromBytes(data[poolQuoteMintOffset : poolQuoteMintOffset+32])
		// Строго token/SOL: (base=token && quote=SOL) || (base=SOL && quote=token)
		if !((baseMint.Equals(tokenMint) && quoteMint.Equals(solMintPubkey)) ||
			(baseMint.Equals(solMintPubkey) && quoteMint.Equals(tokenMint))) {
			continue
		}
		state, err := GetPumpSwapPoolState(ctx, client, acct.Pubkey)
		if err != nil {
			continue
		}
		if !isValidSolPair(state, tokenMint) {
			continue
		}
		if state.SolReserve < MinSolReserveLiquidity {
			continue
		}
		if state.SolReserve > bestSolReserve {
			bestSolReserve = state.SolReserve
			bestPool = state.Pool
		}
	}
	return bestPool, nil
}

// isValidSolPair проверяет: один из mint пула — SOL, другой — переданный tokenMint.
func isValidSolPair(state *PumpSwapPoolState, tokenMint solana.PublicKey) bool {
	if state == nil {
		return false
	}
	tok := state.TokenMint
	sol := state.SolMint
	return (tok.Equals(tokenMint) && sol.Equals(solMintPubkey)) ||
		(tok.Equals(solMintPubkey) && sol.Equals(tokenMint))
}
