package pumpswap

func PriceSOL(solReserve, tokenReserve uint64) float64 {
	if tokenReserve == 0 {
		return 0
	}
	return (float64(solReserve) / 1e9) / (float64(tokenReserve) / 1e6)
}
