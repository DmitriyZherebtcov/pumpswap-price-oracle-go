# pumpswap-price-oracle-go

Оракул цен **только PumpSwap DEX** (без bonding curve). Цена токена строго в паре к SOL.

## Pipeline

```
Token mint → Find PumpSwap SOL pool → Fetch pool state → Parse reserves → Price in SOL
```

## Сборка и запуск

```bash
go build -o oracle ./cmd/
./oracle -config config.yaml
./oracle -config config.yaml -json    # вывод в формате JSON (token, price_sol, pool, slot)
./oracle -interval 5s                # опрос каждые 5 сек
```

## Конфиг (config.yaml)

- `rpc.endpoints` — список RPC (при ошибке переключение на следующий).
- `pumpswap.mints` — адреса mint токенов для отслеживания.

## Структура проекта (после рефакторинга)

```
cmd/
  main.go
internal/
  config/
  oracle/     # PriceOracle, GetTokenPriceSOL
  pumpswap/
    discovery.go   # FindPumpSwapSolPool
    pool_state.go # PumpSwapPoolState, GetPumpSwapPoolState
    pricing.go    # PriceInSOL, проверки
  solana/
    rpc.go
    ws.go        # заглушка для accountSubscribe
```

## Критерии готовности (ТЗ)

- Цена только с PumpSwap DEX, без bonding curve.
- Вход: token mint; выход: price_sol, pool, slot (в т.ч. JSON).
- Резервы из pool state / vault accounts; проверки reserve > 0 и sanity bounds.
- Retry и fallback RPC реализованы.
