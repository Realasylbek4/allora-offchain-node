package main

import (
	apiadapter "allora_offchain_node/adapter/api/apiadapter"
	lib "allora_offchain_node/lib"
	"fmt"
)

func NewAlloraAdapter(name string) (lib.AlloraAdapter, error) {
	switch name {
	case "apiAdapter":
		return apiadapter.NewAlloraAdapter(), nil
	// Add other cases for different adapters here
	default:
		return nil, fmt.Errorf("unknown adapter name: %s", name)
	}
}
