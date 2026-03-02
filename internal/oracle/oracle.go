package oracle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pumpswap-price-oracle-go/internal/pumpswap"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// PriceResult — результат получения цены (формат по ТЗ).
type PriceResult struct {
	Token   string  `json:"token"`
	PriceSOL float64 `json:"price_sol"`
	Pool    string  `json:"pool"`
	Slot    uint64  `json:"slot"`
}

// PriceOracle — оракул цены токена в SOL (только PumpSwap DEX).
type PriceOracle interface {
	GetTokenPriceSOL(ctx context.Context, token solana.PublicKey) (*PriceResult, error)
}

type pumpswapOracle struct {
	client    *rpc.Client
	poolCache map[string]solana.PublicKey
	mu        sync.RWMutex
	// Circuit breaker: после failCount подряд ошибок — пауза до cooldown.
	failCount int
	lastFail  time.Time
	cooldown  time.Duration
}

// NewPumpSwapOracle создаёт оракул, использующий только PumpSwap DEX (без bonding curve).
func NewPumpSwapOracle(client *rpc.Client) PriceOracle {
	return &pumpswapOracle{
		client:    client,
		poolCache: make(map[string]solana.PublicKey),
		cooldown:  30 * time.Second,
	}
}

// GetTokenPriceSOL возвращает цену токена в SOL: token → PumpSwap pool → state → price.
func (o *pumpswapOracle) GetTokenPriceSOL(ctx context.Context, token solana.PublicKey) (*PriceResult, error) {
	o.mu.Lock()
	if o.failCount >= 5 && time.Since(o.lastFail) < o.cooldown {
		o.mu.Unlock()
		return nil, fmt.Errorf("circuit breaker open (cooldown %v)", o.cooldown)
	}
	o.mu.Unlock()

	tokenStr := token.String()

	o.mu.RLock()
	pool, ok := o.poolCache[tokenStr]
	o.mu.RUnlock()

	if !ok {
		var err error
		pool, err = pumpswap.FindPumpSwapSolPool(ctx, o.client, token)
		if err != nil {
			o.recordFail()
			return nil, fmt.Errorf("find pool: %w", err)
		}
		o.mu.Lock()
		o.poolCache[tokenStr] = pool
		o.failCount = 0
		o.mu.Unlock()
	}

	state, err := pumpswap.GetPumpSwapPoolState(ctx, o.client, pool)
	if err != nil {
		o.mu.Lock()
		delete(o.poolCache, tokenStr)
		o.mu.Unlock()
		o.recordFail()
		return nil, fmt.Errorf("pool state: %w", err)
	}

	price, err := pumpswap.PriceInSOL(state)
	if err != nil {
		o.recordFail()
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

// StaleSlotLag — максимальное отставание слота (по ТЗ: < 2 slots lag).
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
