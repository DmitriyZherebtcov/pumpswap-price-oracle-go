package pumpswap

import "errors"

// Ошибки, по которым circuit breaker НЕ должен срабатывать (логические/данные).
var (
	ErrPoolNotFound   = errors.New("pumpswap: pool not found")
	ErrInvalidLayout  = errors.New("pumpswap: invalid pool layout")
	ErrZeroLiquidity  = errors.New("pumpswap: zero liquidity")
	ErrNotSolPair     = errors.New("pumpswap: pool is not token/SOL pair")
	ErrSolReserveLow  = errors.New("pumpswap: SOL reserve below minimum (1 SOL)")
	ErrStaleState     = errors.New("pumpswap: pool state stale (slot lag > 2)")
	ErrInvalidPrice   = errors.New("pumpswap: invalid price (sanity check)")
)
