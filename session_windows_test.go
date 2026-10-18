//go:build windows

package main

import (
	"bytes"
	"os"
	"testing"
)

func TestDPAPIRoundTrip(t *testing.T) {
	secret := []byte("refresh-token-fixture-not-real")
	encrypted, err := dpapiProtect(secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, secret) {
		t.Fatal("encrypted DPAPI data contains plaintext credential")
	}
	got, err := dpapiUnprotect(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("round-trip mismatch")
	}
	zeroBytes(got)
	if bytes.Equal(got, secret) {
		t.Fatal("zeroBytes did not clear plaintext buffer")
	}
}

func TestSessionCacheWindows(t *testing.T) {
	// Under Windows, os.UserConfigDir uses APPDATA.
	t.Setenv("APPDATA", t.TempDir())
	id := &authIdentity{accountName: "example", refreshToken: "not-a-real-token"}
	if err := saveCachedSession(id); err != nil {
		t.Fatal(err)
	}
	path, err := cachePath()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(id.refreshToken)) {
		t.Fatal("DPAPI cache wrote plaintext token")
	}
	loaded, err := loadCachedSession()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.accountName != id.accountName || loaded.refreshToken != id.refreshToken {
		t.Fatalf("session mismatch: account=%q", loaded.accountName)
	}
	if err := clearCachedSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cache still exists after logout: %v", err)
	}
}
