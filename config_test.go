package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitCacheFlagOverridesMalformedEnvironment(t *testing.T) {
	t.Setenv("CACHE_SESSION", "invalid")
	t.Setenv("AUTH_METHOD", "auto")
	for _, enabled := range []bool{false, true} {
		arg := "--cache-session=" + map[bool]string{false: "false", true: "true"}[enabled]
		opt, err := parseOptions([]string{arg, "123456"}, io.Discard)
		if err != nil || opt.useCache != enabled {
			t.Fatalf("explicit cache flag did not override environment: %v", err)
		}
	}
	if _, err := parseOptions([]string{"123456"}, io.Discard); err == nil {
		t.Fatal("invalid effective environment must be rejected")
	}
}

func TestUtilityOptionsIgnoreInvalidAuthEnvironment(t *testing.T) {
	t.Setenv("CACHE_SESSION", "invalid")
	t.Setenv("AUTH_METHOD", "invalid")
	for _, arg := range []string{"--version", "--logout", "--help"} {
		_, err := parseOptions([]string{arg}, io.Discard)
		if err != nil && !(arg == "--help" && errors.Is(err, flag.ErrHelp)) {
			t.Errorf("%s blocked by auth environment: %v", arg, err)
		}
	}
}

func TestParseOptions(t *testing.T) {
	for _, envName := range []string{"AUTH_METHOD", "CACHE_SESSION", "STEAM_USERNAME", "DEADLOCK_LOCATION"} {
		t.Setenv(envName, "")
	}
	const steamID = "76561197960389184"
	var errBuf bytes.Buffer
	opt, err := parseOptions([]string{"--auth", "credentials", "--username", "sample", steamID}, &errBuf)
	if err != nil {
		t.Fatal(err)
	}
	if opt.authMode != "credentials" || opt.username != "sample" || !opt.useCache || opt.friendID != uint32(76561197960389184-steamID64Base) {
		t.Fatalf("unexpected options: %#v", opt)
	}
	_, err = parseOptions([]string{"--auth", "banana", steamID}, &errBuf)
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	_, err = parseOptions([]string{"76561197960265728"}, &errBuf)
	if err == nil {
		t.Fatal("expected invalid account ID")
	}
	opt, err = parseOptions([]string{"--logout"}, &errBuf)
	if err != nil || !opt.forgetCache {
		t.Fatalf("logout failed: %#v %v", opt, err)
	}
}

func TestDotEnvPriorityAndQuoting(t *testing.T) {
	const key = "TEST_DEADLOCK_INVITER_ENV"
	t.Setenv(key, "from-os")
	const secondary = "TEST_DEADLOCK_INVITER_ANOTHER"
	// Do not allow an env variable set by a previous unit test to affect this test.
	_ = os.Unsetenv(secondary)
	t.Cleanup(func() { _ = os.Unsetenv(secondary) })
	path := filepath.Join(t.TempDir(), ".env")
	content := strings.Join([]string{
		"# test config",
		"TEST_DEADLOCK_INVITER_ENV=from-file",
		"export TEST_DEADLOCK_INVITER_ANOTHER=\"quoted value\"",
		"", "",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "from-os" {
		t.Errorf("os env must win, got %q", got)
	}
	if got := os.Getenv(secondary); got != "quoted value" {
		t.Errorf("env parsing failed, got %q", got)
	}
}

func TestDotEnvRejectsSyntaxErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	for _, bad := range []string{"not key=value", "export TEST_KEY=\"unterminated", "INVALID-KEY=x"} {
		if err := os.WriteFile(path, []byte(bad+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := loadDotEnv(path); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestNoPlaintextCacheFallback(t *testing.T) {
	if cacheAvailable() {
		// Windows-specific round trip is tested in session_windows_test.go.
		return
	}
	if err := saveCachedSession(&authIdentity{accountName: "user", refreshToken: "token"}); err != errCacheUnsupported {
		t.Errorf("unsafe platform must not save cache: %v", err)
	}
}

func TestGCStatus(t *testing.T) {
	if err := describeGCResponse(0); err != nil {
		t.Fatalf("expected success: %v", err)
	}
	for _, code := range []int32{3, 4, 5, 6, 7, 12345} {
		if err := describeGCResponse(code); err == nil {
			t.Errorf("expected error for code %d", code)
		}
	}
}
