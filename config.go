package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const steamID64Base uint64 = 76561197960265728

const appVersion = "0.2.0"

// version may be overridden at build time with -ldflags.
var version = appVersion

type options struct {
	friendID    uint32
	authMode    string
	username    string
	location    string
	useCache    bool
	forgetCache bool
	showVersion bool
}

func parseFriendAccountID(raw string) (uint32, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errors.New("SteamID64 / AccountID должен быть десятичным числом")
	}
	if id >= steamID64Base {
		id -= steamID64Base
	}
	if id == 0 || id > uint64(^uint32(0)) {
		return 0, fmt.Errorf("некорректный SteamID64 / AccountID: %s", raw)
	}
	return uint32(id), nil
}

// loadDotEnv reads a deliberately small subset of dotenv syntax. Existing OS
// environment wins. The file is optional, and is NEVER committed to Git.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("открытие .env: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validEnvKey(key) {
			return fmt.Errorf(".env:%d: ожидается KEY=VALUE", lineNo)
		}
		val = strings.TrimSpace(val)
		if len(val) > 0 && (val[0] == '\'' || val[0] == '"') {
			quote := val[0]
			if len(val) < 2 || val[len(val)-1] != quote {
				return fmt.Errorf(".env:%d: незакрытая кавычка", lineNo)
			}
			val = val[1 : len(val)-1]
			// Double-quoted values allow escaped slash and quote. Do not
			// interpret \n as a newline in a Steam password.
			if quote == '"' {
				val = strings.ReplaceAll(val, `\"`, `"`)
				val = strings.ReplaceAll(val, `\\`, `\`)
			}
		} else if pos := strings.Index(val, " #"); pos >= 0 {
			val = strings.TrimSpace(val[:pos])
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, val); err != nil {
				return fmt.Errorf(".env:%d: некорректная переменная", lineNo)
			}
		}
	}
	return scanner.Err()
}

func validEnvKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func parseOptions(args []string, output io.Writer) (options, error) {
	o := options{}
	mode := strings.ToLower(strings.TrimSpace(getEnv("AUTH_METHOD", "auto")))
	if mode == "" {
		mode = "auto"
	}

	cacheDefault := true
	var cacheEnvErr error
	if raw := strings.TrimSpace(os.Getenv("CACHE_SESSION")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			cacheEnvErr = errors.New("CACHE_SESSION должен быть true или false")
		} else {
			cacheDefault = parsed
		}
	}

	fs := flag.NewFlagSet("deadlock-inviter", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&o.authMode, "auth", mode, "auto|qr|credentials|cache")
	fs.StringVar(&o.username, "username", getEnv("STEAM_USERNAME", ""), "Steam account login name (not a password)")
	fs.StringVar(&o.location, "location", getEnv("DEADLOCK_LOCATION", ""), "optional undocumented GC location field")
	fs.BoolVar(&o.useCache, "cache-session", cacheDefault, "use encrypted Windows session cache")
	fs.BoolVar(&o.forgetCache, "logout", false, "delete cached session and exit")
	fs.BoolVar(&o.showVersion, "version", false, "print program version and exit")
	fs.Usage = func() {
		fmt.Fprintln(output, "Deadlock Inviter — Steam QR / credentials / Windows DPAPI")
		fmt.Fprintln(output, "Usage: deadlock-inviter [flags] <SteamID64-or-AccountID>")
		fmt.Fprintln(output, "       deadlock-inviter --logout")
		fmt.Fprintln(output, "Flags must precede SteamID64:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if o.showVersion || o.forgetCache {
		if fs.NArg() != 0 {
			return o, errors.New("--version и --logout запускаются без SteamID друга")
		}
		return o, nil
	}
	cacheExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "cache-session" {
			cacheExplicit = true
		}
	})
	if cacheEnvErr != nil && !cacheExplicit {
		return o, cacheEnvErr
	}
	o.authMode = strings.ToLower(strings.TrimSpace(o.authMode))
	switch o.authMode {
	case "auto", "qr", "credentials", "cache":
	default:
		return o, fmt.Errorf("неизвестный метод входа %q (auto|qr|credentials|cache)", o.authMode)
	}
	if fs.NArg() != 1 {
		return o, errors.New("укажи SteamID64 друга (см. --help)")
	}
	var err error
	o.friendID, err = parseFriendAccountID(fs.Arg(0))
	return o, err
}
