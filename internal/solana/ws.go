package solana

// WebSocket accountSubscribe для real-time обновления цены (latency <20ms).
// Реализация: использовать github.com/gagliardetto/solana-go/rpc/ws:
//
//	client, _ := ws.Connect(ctx, wsEndpoint)
//	sub, _ := client.AccountSubscribe(pool, rpc.CommitmentConfirmed)
//	for result := range sub.Response() { ... onUpdate(result.Value.Data, result.Context.Slot) }
//
// Пока оракул использует polling (GetPumpSwapPoolState каждые N сек).
// При необходимости раскомментировать и добавить зависимость rpc/ws.
// type AccountSubscription struct{}
// func SubscribeAccount(ctx context.Context, wsEndpoint string, account solana.PublicKey, onUpdate func(data []byte, slot uint64)) (*AccountSubscription, error) { return nil, nil }
