package oracle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"pumpswap-price-oracle-go/internal/pumpswap"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// Cache TTL для пула (по ТЗ: 10 минут).
const PoolCacheTTL = 10 * time.Minute

// PriceResult — результат получения цены (формат по ТЗ).
type PriceResult struct {
	Token    string  `json:"token"`
	PriceSOL float64 `json:"price_sol"`
	Pool     string  `json:"pool"`
	Slot     uint64  `json:"slot"`
}

// PriceOracle — оракул цены токена в SOL (только PumpSwap DEX).
type PriceOracle interface {
	GetTokenPriceSOL(ctx context.Context, token solana.PublicKey) (*PriceResult, error)
}

type cachedPool struct {
	Address solana.PublicKey
	Expiry  time.Time
}

type pumpswapOracle struct {
	client    *rpc.Client
	poolCache map[string]cachedPool
	mu        sync.RWMutex
	// Circuit breaker: только при RPC/network ошибках (не при ErrPoolNotFound, layout, liquidity, stale).
	failCount int
	lastFail  time.Time
	cooldown  time.Duration
}

// NewPumpSwapOracle создаёт оракул, использующий только PumpSwap DEX (без bonding curve).
func NewPumpSwapOracle(client *rpc.Client) PriceOracle {
	return &pumpswapOracle{
		client:    client,
		poolCache: make(map[string]cachedPool),
		cooldown:  30 * time.Second,
	}
}

// isLogicalError — ошибки, по которым circuit breaker НЕ срабатывает.
func isLogicalError(err error) bool {
	return err != nil && (errors.Is(err, pumpswap.ErrPoolNotFound) ||
		errors.Is(err, pumpswap.ErrInvalidLayout) ||
		errors.Is(err, pumpswap.ErrZeroLiquidity) ||
		errors.Is(err, pumpswap.ErrStaleState) ||
		errors.Is(err, pumpswap.ErrNotSolPair) ||
		errors.Is(err, pumpswap.ErrSolReserveLow) ||
		errors.Is(err, pumpswap.ErrInvalidPrice))
}

// GetTokenPriceSOL возвращает цену токена в SOL: token → PumpSwap pool (scan) → state → price.
func (o *pumpswapOracle) GetTokenPriceSOL(ctx context.Context, token solana.PublicKey) (*PriceResult, error) {
	o.mu.Lock()
	if o.failCount >= 5 && time.Since(o.lastFail) < o.cooldown {
		o.mu.Unlock()
		return nil, fmt.Errorf("circuit breaker open (cooldown %v)", o.cooldown)
	}
	o.mu.Unlock()

	tokenStr := token.String()

	o.mu.RLock()
	cached := o.poolCache[tokenStr]
	o.mu.RUnlock()

	now := time.Now()
	if cached.Expiry.Before(now) || cached.Address.IsZero() {
		// TTL истёк или кэш пуст — повторный scan пула.
		pool, err := pumpswap.FindPumpSwapSolPoolByScan(ctx, o.client, token)
		if err != nil {
			if !isLogicalError(err) {
				o.recordFail()
			}
			return nil, fmt.Errorf("find pool: %w", err)
		}
		o.mu.Lock()
		o.poolCache[tokenStr] = cachedPool{Address: pool, Expiry: now.Add(PoolCacheTTL)}
		o.failCount = 0
		o.mu.Unlock()
		cached.Address = pool
		cached.Expiry = now.Add(PoolCacheTTL)
	}

	state, err := pumpswap.GetPumpSwapPoolState(ctx, o.client, cached.Address)
	if err != nil {
		o.mu.Lock()
		delete(o.poolCache, tokenStr)
		o.mu.Unlock()
		if !isLogicalError(err) {
			o.recordFail()
		}
		return nil, fmt.Errorf("pool state: %w", err)
	}

	// Валидация актуальности: current_slot - pool_slot <= 2
	currentSlot, err := o.client.GetSlot(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		o.recordFail()
		return nil, fmt.Errorf("get slot: %w", err)
	}
	if currentSlot > 0 && state.Slot+StaleSlotLag < currentSlot {
		o.mu.Lock()
		delete(o.poolCache, tokenStr)
		o.mu.Unlock()
		return nil, fmt.Errorf("%w (slot %d, current %d)", pumpswap.ErrStaleState, state.Slot, currentSlot)
	}

	price, err := pumpswap.PriceInSOL(state)
	if err != nil {
		if !isLogicalError(err) {
			o.recordFail()
		}
		return nil, fmt.Errorf("pricing: %w", err)
	}

	o.mu.Lock()
	o.failCount = 0
	o.mu.Unlock()

	return &PriceResult{
		Token:    tokenStr,
		PriceSOL: price,
		Pool:     state.Pool.String(),
		Slot:     state.Slot,
	}, nil
}

func (o *pumpswapOracle) recordFail() {
	o.mu.Lock()
	o.failCount++
	o.lastFail = time.Now()
	o.mu.Unlock()
}

// StaleSlotLag — максимальное отставание слота (по ТЗ: <= 2 slots).
const StaleSlotLag = 2

// IsStale возвращает true, если state устарел относительно currentSlot.
func IsStale(stateSlot, currentSlot uint64) bool {
	if currentSlot == 0 {
		return false
	}
	return stateSlot+StaleSlotLag < currentSlot
}

// RetryConfig для retry logic.
type RetryConfig struct {
	MaxRetries int
	Delay      time.Duration
}

// DefaultRetryConfig возвращает настройки по умолчанию.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxRetries: 3, Delay: 500 * time.Millisecond}
}
