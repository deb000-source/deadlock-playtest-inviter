//go:build !windows

package main

func cacheAvailable() bool { return false }

func dpapiProtect([]byte) ([]byte, error) {
	return nil, errCacheUnsupported
}

func dpapiUnprotect([]byte) ([]byte, error) {
	return nil, errCacheUnsupported
}
