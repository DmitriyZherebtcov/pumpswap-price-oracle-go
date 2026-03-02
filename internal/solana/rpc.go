package solana

import (
	"github.com/gagliardetto/solana-go/rpc"
)

// NewRPCClient создаёт RPC-клиент для заданного endpoint.
func NewRPCClient(endpoint string) *rpc.Client {
	return rpc.New(endpoint)
}
