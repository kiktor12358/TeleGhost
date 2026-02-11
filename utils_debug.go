package main

import "teleghost/internal/appcore"

func getMapKeys(m map[string]*appcore.PendingTransfer) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
