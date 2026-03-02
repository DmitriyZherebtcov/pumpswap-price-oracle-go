package pumpswap

import (
	"github.com/gagliardetto/solana-go"
)

// Pump.fun program ID (PumpSwap)
const PumpProgramID = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"

// DeriveBondingCurve returns the bonding curve PDA for the given mint.
// bonding_curve = PDA(["bonding-curve", mint], PUMPSWAP_PROGRAM_ID)
func DeriveBondingCurve(mint solana.PublicKey) (solana.PublicKey, error) {
	programID, err := solana.PublicKeyFromBase58(PumpProgramID)
	if err != nil {
		return solana.PublicKey{}, err
	}
	seeds := [][]byte{
		[]byte("bonding-curve"),
		mint.Bytes(),
	}
	addr, _, err := solana.FindProgramAddress(seeds, programID)
	return addr, err
}

func PriceSOL(solReserve, tokenReserve uint64) float64 {
	if tokenReserve == 0 {
		return 0
	}
	return (float64(solReserve) / 1e9) / (float64(tokenReserve) / 1e6)
}
