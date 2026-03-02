package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"pumpswap-price-oracle-go/internal/config"
	"pumpswap-price-oracle-go/internal/pumpswap"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// Оракул смотрит только PumpSwap (https://swap.pump.fun/) по адресу контракта (mint) токена.

func main() {
	log.SetFlags(0)
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg := config.Load(*cfgPath)
	if len(cfg.RPC.Endpoints) == 0 {
		log.Fatal("config: no rpc endpoints")
	}
	if len(cfg.PumpSwap.Mints) == 0 {
		log.Fatal("config: no pumpswap.mints (token contract addresses for https://swap.pump.fun/)")
	}

	type target struct {
		mint solana.PublicKey
		pool *pumpswap.PoolReserves
	}
	var targets []target
	for _, a := range cfg.PumpSwap.Mints {
		if a == "" || a == "PUT_MINT_ADDRESS_HERE" {
			continue
		}
		mint, err := solana.PublicKeyFromBase58(a)
		if err != nil {
			log.Printf("skip invalid mint %q: %v", a, err)
			continue
		}
		targets = append(targets, target{mint: mint})
	}
	if len(targets) == 0 {
		log.Fatal("no valid mints")
	}

	ctx := context.Background()
	rpcEndpoints := cfg.RPC.Endpoints
	if len(rpcEndpoints) == 0 {
		log.Fatal("config: no rpc endpoints")
	}
	client := rpc.New(rpcEndpoints[0])
	rpcIndex := 0

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	log.Printf("Oracle started. PumpSwap only (https://swap.pump.fun/). Watching %d token(s), RPC: %s", len(targets), rpcEndpoints[0])
	log.Println("--- price changes ---")

	for range ticker.C {
		ts := time.Now().Format("15:04:05")
		for i := range targets {
			t := &targets[i]
			mintAddr := t.mint.String()

			if t.pool == nil {
				pool, err := pumpswap.FindPumpSwapPoolByBaseMint(client, t.mint)
				if err != nil {
					log.Printf("[%s] mint %s price = N/A (pool not found: %v)", ts, mintAddr, err)
					if rpcIndex+1 < len(rpcEndpoints) {
						rpcIndex++
						client = rpc.New(rpcEndpoints[rpcIndex])
						log.Printf("RPC fallback: %s", rpcEndpoints[rpcIndex])
					}
					continue
				}
				t.pool = pool
				log.Printf("[%s] mint %s pool=%s base_ata=%s quote_ata=%s", ts, mintAddr, pool.PoolAddress.String(), pool.BaseTokenAccount.String(), pool.QuoteTokenAccount.String())
			}

			baseAmount, quoteAmount, err := pumpswap.UpdatePoolReserves(client, ctx, t.pool)
			if err != nil {
				log.Printf("[%s] mint %s price = N/A (reserves error: %v)", ts, mintAddr, err)
				t.pool = nil // сброс кэша пула — на следующем тике попробуем снова (или с другим RPC)
				if rpcIndex+1 < len(rpcEndpoints) {
					rpcIndex++
					client = rpc.New(rpcEndpoints[rpcIndex])
					log.Printf("RPC fallback: %s", rpcEndpoints[rpcIndex])
				}
				continue
			}
			price := pumpswap.PriceFromPool(baseAmount, quoteAmount, t.pool.BaseDecimals, t.pool.QuoteDecimals)
			reserves := fmt.Sprintf("base_raw=%d quote_raw=%d", baseAmount, quoteAmount)
			log.Printf("[%s] mint %s price = %.10f SOL/token (pumpswap, %s)", ts, mintAddr, price, reserves)
		}
	}
}
