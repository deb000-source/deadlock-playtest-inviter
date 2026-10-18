package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Session cache deliberately has no plain-text fallback. Only Windows DPAPI
// supports persistence in this project. --cache-session=false disables it.
var errCacheUnsupported = errors.New("защищённый кеш доступен только в Windows (DPAPI)")

const sessionCacheVersion = 1

type sessionRecord struct {
	Version int       `json:"version"`
	Account string    `json:"account"`
	Token   string    `json:"token"`
	SavedAt time.Time `json:"saved_at"`
}

func cachePath() (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configRoot, "deadlock-inviter", "session.dpapi"), nil
}

func loadCachedSession() (*authIdentity, error) {
	if !cacheAvailable() {
		return nil, errCacheUnsupported
	}
	path, err := cachePath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(content) > 1024*1024 {
		return nil, errors.New("кеш сессии слишком большой")
	}
	plaintext, err := dpapiUnprotect(content)
	if err != nil {
		return nil, fmt.Errorf("кеш зашифрован для другого Windows-пользователя или повреждён: %w", err)
	}
	defer zeroBytes(plaintext)
	var rec sessionRecord
	if err := json.Unmarshal(plaintext, &rec); err != nil {
		return nil, errors.New("кеш сессии повреждён")
	}
	if rec.Version != sessionCacheVersion || rec.Account == "" || rec.Token == "" {
		return nil, errors.New("кеш сессии пустой или несовместим с текущей версией")
	}
	return &authIdentity{accountName: rec.Account, refreshToken: rec.Token}, nil
}

func saveCachedSession(id *authIdentity) error {
	if !cacheAvailable() {
		return errCacheUnsupported
	}
	if id == nil || id.accountName == "" || id.refreshToken == "" {
		return errors.New("нельзя сохранить неполную Steam-сессию")
	}
	rec := sessionRecord{Version: sessionCacheVersion, Account: id.accountName, Token: id.refreshToken, SavedAt: time.Now()}
	plaintext, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	defer zeroBytes(plaintext)
	blob, err := dpapiProtect(plaintext)
	if err != nil {
		return fmt.Errorf("DPAPI: %w", err)
	}
	defer zeroBytes(blob)
	path, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(blob); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		// Some Windows configurations do not replace an existing target.
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return err
		}
		return os.Rename(tempPath, path)
	}
	return nil
}

func clearCachedSession() error {
	if !cacheAvailable() {
		return errCacheUnsupported
	}
	path, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
