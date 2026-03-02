package pumpswap

import (
	"fmt"
	"math"
)

// MinReserveForPrice — минимальный резерв для расчёта цены (защита от деления и нестабильных пулов).
const MinReserveForPrice = 1

// PriceInSOL возвращает цену 1 токена в SOL.
// Формула: (sol_reserve / 10^sol_decimals) / (token_reserve / 10^token_decimals).
// Проверки: резервы > 0, результат в разумных границах.
func PriceInSOL(state *PumpSwapPoolState) (float64, error) {
	if state == nil {
		return 0, fmt.Errorf("pool state is nil")
	}
	if state.TokenReserve < MinReserveForPrice {
		return 0, fmt.Errorf("token reserve too small: %d", state.TokenReserve)
	}
	if state.SolReserve < MinReserveForPrice {
		return 0, fmt.Errorf("sol reserve too small: %d", state.SolReserve)
	}

	solDiv := math.Pow(10, float64(state.SolDecimals))
	tokenDiv := math.Pow(10, float64(state.TokenDecimals))
	price := (float64(state.SolReserve) / solDiv) / (float64(state.TokenReserve) / tokenDiv)

	// Sanity: цена не должна быть отрицательной, нулевой, NaN или астрономической
	if price <= 0 || math.IsInf(price, 0) || math.IsNaN(price) {
		return 0, fmt.Errorf("invalid price: %f", price)
	}
	if price > 1e15 || price < 1e-15 {
		return 0, fmt.Errorf("price out of sanity bounds: %f", price)
	}
	return price, nil
}
