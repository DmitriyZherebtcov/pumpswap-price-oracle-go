package main

import (
	"fmt"
	"os"

	"pumpswap-price-oracle-go/internal/pumpswap"

	"github.com/gagliardetto/solana-go"
)

func main() {
	mintStr := "HcMEYrBKdULwo8WArhz8M8huSgqGh11g3zRin96QAkwQ"
	if len(os.Args) > 1 {
		mintStr = os.Args[1]
	}
	mint, err := solana.PublicKeyFromBase58(mintStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid mint: %v\n", err)
		os.Exit(1)
	}
	bc, err := pumpswap.DeriveBondingCurve(mint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "derive: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(bc.String())
}
