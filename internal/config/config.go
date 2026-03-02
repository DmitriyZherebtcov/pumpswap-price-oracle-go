package config

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	RPC struct {
		Endpoints []string `yaml:"endpoints"`
	} `yaml:"rpc"`

	WS struct {
		Endpoints []string `yaml:"endpoints"`
	} `yaml:"ws"`

	PumpSwap struct {
		Mints []string `yaml:"mints"` // token mint addresses (PumpSwap DEX only)
	} `yaml:"pumpswap"`

	Stream struct {
		Commitment       string `yaml:"commitment"`
		ReconnectDelayMs int    `yaml:"reconnect_delay_ms"`
	} `yaml:"stream"`
}

func Load(path string) Config {
	data, _ := os.ReadFile(path)
	var cfg Config
	yaml.Unmarshal(data, &cfg)
	return cfg
}
