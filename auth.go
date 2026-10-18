package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	steam "github.com/paralin/go-steam"
	"github.com/paralin/go-steam/protocol/protobuf/unified"
	"golang.org/x/term"
)

type authIdentity struct {
	accountName  string
	refreshToken string // bearer credential; never print, log, or write as plaintext
}

type authResult struct {
	identity  *authIdentity
	fromCache bool
}

var consoleInput = bufio.NewReader(os.Stdin)

func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	value, err := consoleInput.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && value != "") {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// readHidden never echoes passwords/codes to terminal, and refuses non-TTY input.
// For automation opt into STEAM_PASSWORD in .env (at your own risk).
func readHidden(prompt string) (string, error) {
	fmt.Print(prompt)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("нет интерактивного терминала для скрытого ввода")
	}
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	defer zeroBytes(secret)
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

func selectAuthMode() (string, error) {
	fmt.Println("Способ авторизации: [1] QR Steam Guard (рекомендуется), [2] логин + пароль")
	value, err := readLine("Выбери [1]: ")
	if err != nil {
		return "", err
	}
	switch value {
	case "", "1":
		return "qr", nil
	case "2":
		return "credentials", nil
	default:
		return "", fmt.Errorf("некорректный номер способа входа: %q", value)
	}
}

func authenticate(ctx context.Context, o options) (*authResult, error) {
	if o.authMode == "cache" && !o.useCache {
		return nil, errors.New("--auth cache несовместим с --cache-session=false")
	}
	if o.useCache && (o.authMode == "auto" || o.authMode == "cache") {
		id, err := loadCachedSession()
		if err == nil && o.username != "" && id.accountName != o.username {
			err = fmt.Errorf("кеш принадлежит аккаунту %q, запрошен %q", id.accountName, o.username)
		}
		if err == nil {
			fmt.Printf("Использую локальную защищённую сессию Steam: %s\n", id.accountName)
			return &authResult{identity: id, fromCache: true}, nil
		}
		if o.authMode == "cache" {
			return nil, fmt.Errorf("нет доступной кешированной сессии: %w", err)
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errCacheUnsupported) {
			fmt.Printf("Сохранённая сессия недоступна: %v\n", err)
		}
	}
	if o.authMode == "cache" {
		return nil, errors.New("сессия не найдена; войди через QR или Credentials")
	}

	mode := o.authMode
	if mode == "auto" {
		var err error
		mode, err = selectAuthMode()
		if err != nil {
			return nil, err
		}
	}
	var id *authIdentity
	var err error
	switch mode {
	case "qr":
		qrCtx, cancel := context.WithTimeout(ctx, authTimeout)
		defer cancel()
		id, err = loginViaSteamQR(qrCtx)
	case "credentials":
		authCtx, cancel := context.WithTimeout(ctx, authTimeout)
		defer cancel()
		id, err = loginViaSteamCredentials(authCtx, o.username)
	default:
		return nil, fmt.Errorf("метод входа %q не реализован", mode)
	}
	if err != nil {
		return nil, err
	}
	return &authResult{identity: id}, nil
}

func loginViaSteamCredentials(ctx context.Context, username string) (*authIdentity, error) {
	if username == "" {
		value, err := readLine("Логин Steam: ")
		if err != nil {
			return nil, err
		}
		username = value
	}
	if username == "" {
		return nil, errors.New("логин Steam не указан")
	}

	password, exists := os.LookupEnv("STEAM_PASSWORD")
	if !exists {
		var err error
		password, err = readHidden("Пароль Steam (ввод скрыт): ")
		if err != nil {
			return nil, err
		}
	} else {
		fmt.Println("Пароль прочитан из STEAM_PASSWORD (не рекомендуется хранить его в .env).")
	}
	if password == "" {
		return nil, errors.New("пароль Steam пуст")
	}

	fmt.Println("Получаю токен Steam; при необходимости потребуется Steam Guard...")
	id, err := newAuthService().loginCredentials(ctx, username, password, promptSteamGuard)
	password = ""
	if err != nil {
		return nil, fmt.Errorf("Steam отклонил авторизацию: %w", err)
	}
	return id, nil
}

// Called for the *same* authentication session; asking for a code
// outside this callback could invalidate an email code by creating a new one.
func promptSteamGuard(ctx context.Context, confirmations []steam.SteamGuardConfirmation) (string, unified.EAuthSessionGuardType, error) {
	fmt.Println("Steam Guard: доступные варианты подтверждения:")
	types := make([]steam.SteamGuardConfirmation, 0, len(confirmations))
	for _, c := range confirmations {
		name := c.Type.String()
		var label string
		switch {
		case strings.Contains(name, "DeviceCode"):
			label = "Код из мобильного Steam Guard"
		case strings.Contains(name, "EmailCode"):
			label = "Код из письма Steam"
		case strings.Contains(name, "DeviceConfirmation"):
			label = "Подтверждение входа в приложении Steam"
		case strings.Contains(name, "EmailConfirmation"):
			label = "Подтверждение входа по email"
		default:
			continue
		}
		types = append(types, c)
		fmt.Printf("  [%d] %s\n", len(types), label)
	}
	if len(types) == 0 {
		return "", 0, errors.New("Steam вернул неподдерживаемый тип Steam Guard")
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	choice := 1
	if len(types) > 1 {
		line, err := readLine("Способ подтверждения [1]: ")
		if err != nil {
			return "", 0, err
		}
		if line != "" {
			choice, err = strconv.Atoi(line)
			if err != nil || choice < 1 || choice > len(types) {
				return "", 0, errors.New("неверный номер способа Steam Guard")
			}
		}
	}
	selected := types[choice-1]
	typeName := selected.Type.String()
	if strings.Contains(typeName, "DeviceCode") || strings.Contains(typeName, "EmailCode") {
		code, err := readHidden("Код Steam Guard (ввод скрыт): ")
		if err != nil {
			return "", 0, err
		}
		code = strings.TrimSpace(code)
		if code == "" {
			return "", 0, errors.New("пустой код Steam Guard")
		}
		return code, selected.Type, nil
	}
	fmt.Println("Подтверди вход в приложении Steam / почте. Программа проверит результат автоматически.")
	return "", selected.Type, nil
}
