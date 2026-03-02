package pumpswap

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// FindPumpSwapSolPool находит пул PumpSwap для пары tokenMint/SOL (quote = wrapped SOL).
// Возвращает пул с максимальной ликвидностью SOL среди найденных.
func FindPumpSwapSolPool(ctx context.Context, client *rpc.Client, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	programID, err := solana.PublicKeyFromBase58(PumpSwapAMMProgramID)
	if err != nil {
		return solana.PublicKey{}, err
	}

	opts := &rpc.GetProgramAccountsOpts{
		Commitment: rpc.CommitmentConfirmed,
		Encoding:   solana.EncodingBase64,
		Filters: []rpc.RPCFilter{
			{
				Memcmp: &rpc.RPCFilterMemcmp{
					Offset: poolBaseMintOffset,
					Bytes:  solana.Base58(tokenMint.String()),
				},
			},
		},
	}
	accounts, err := client.GetProgramAccountsWithOpts(ctx, programID, opts)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if len(accounts) == 0 {
		return solana.PublicKey{}, fmt.Errorf("no PumpSwap SOL pool for mint %s", tokenMint.String())
	}

	var bestPool solana.PublicKey
	var bestSolReserve uint64

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
		if state.SolReserve == 0 || state.TokenReserve == 0 {
			continue
		}
		if state.SolReserve > bestSolReserve {
			bestSolReserve = state.SolReserve
			bestPool = state.Pool
		}
	}

	if bestPool.IsZero() {
		return solana.PublicKey{}, fmt.Errorf("no PumpSwap SOL pool with valid reserves for mint %s", tokenMint.String())
	}
	return bestPool, nil
}
