// Оракул цен PumpSwap DEX: только token/SOL пары, без bonding curve.
// Pipeline: Token mint → Find PumpSwap SOL pool → Fetch pool state → Parse reserves → Price in SOL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"pumpswap-price-oracle-go/internal/config"
	"pumpswap-price-oracle-go/internal/oracle"
	"pumpswap-price-oracle-go/internal/solana"

	solanago "github.com/gagliardetto/solana-go"
)

func main() {
	log.SetFlags(0)
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	jsonOut := flag.Bool("json", false, "output price as JSON (token, price_sol, pool, slot)")
	interval := flag.Duration("interval", 10*time.Second, "polling interval")
	flag.Parse()

	cfg := config.Load(*cfgPath)
	if len(cfg.RPC.Endpoints) == 0 {
		log.Fatal("config: no rpc endpoints")
	}
	if len(cfg.PumpSwap.Mints) == 0 {
		log.Fatal("config: no pumpswap.mints (token mint addresses for https://swap.pump.fun/)")
	}

	var mints []solanago.PublicKey
	for _, a := range cfg.PumpSwap.Mints {
		if a == "" || a == "PUT_MINT_ADDRESS_HERE" {
			continue
		}
		mint, err := solanago.PublicKeyFromBase58(a)
		if err != nil {
			log.Printf("skip invalid mint %q: %v", a, err)
			continue
		}
		mints = append(mints, mint)
	}
	if len(mints) == 0 {
		log.Fatal("no valid mints")
	}

	client := solana.NewRPCClient(cfg.RPC.Endpoints[0])
	rpcIndex := 0
	orc := oracle.NewPumpSwapOracle(client)
	retry := oracle.DefaultRetryConfig()

	log.Printf("PumpSwap DEX Oracle started. Watching %d token(s), RPC: %s", len(mints), cfg.RPC.Endpoints[0])
	log.Println("--- price updates ---")

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		for i := range mints {
			token := mints[i]
			var res *oracle.PriceResult
			var err error
			for attempt := 0; attempt <= retry.MaxRetries; attempt++ {
				res, err = orc.GetTokenPriceSOL(ctx, token)
				if err == nil {
					break
				}
				if attempt < retry.MaxRetries {
					time.Sleep(retry.Delay)
				}
			}
			if err != nil {
				log.Printf("mint %s price = N/A (%v)", token.String(), err)
				if rpcIndex+1 < len(cfg.RPC.Endpoints) {
					rpcIndex++
					client = solana.NewRPCClient(cfg.RPC.Endpoints[rpcIndex])
					orc = oracle.NewPumpSwapOracle(client)
					log.Printf("RPC fallback: %s", cfg.RPC.Endpoints[rpcIndex])
				}
				continue
			}

			if *jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(res)
			} else {
				log.Printf("[%s] mint %s price_sol=%.10f pool=%s slot=%d",
					time.Now().Format("15:04:05"), res.Token, res.PriceSOL, res.Pool, res.Slot)
			}
		}
		cancel()
	}
}
