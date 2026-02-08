package main

func getMapKeys(m map[string]*PendingTransfer) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
