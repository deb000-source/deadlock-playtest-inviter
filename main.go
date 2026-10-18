// Deadlock Inviter: invite friends through Steam / Deadlock GC without installing the game.
// Unofficial experiment; not affiliated with Valve.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	deadlock "github.com/paralin/go-deadlock"
	devents "github.com/paralin/go-deadlock/events"
	gcproto "github.com/paralin/go-deadlock/protocol"
	steam "github.com/paralin/go-steam"
	"github.com/paralin/go-steam/protocol/steamlang"
	"github.com/sirupsen/logrus"
)

const (
	authTimeout = 5 * time.Minute
	gcTimeout   = 3 * time.Minute
)

var errCMAuthRejected = errors.New("Steam CM отклонил авторизацию")

type inviteResult struct {
	response *gcproto.CMsgClientToGCSubmitPlaytestUserResponse
	err      error
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if err := loadDotEnv(".env"); err != nil {
		return err
	}
	o, err := parseOptions(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if o.showVersion {
		fmt.Printf("Deadlock Inviter %s\n", version)
		return nil
	}
	if o.forgetCache {
		if err := clearCachedSession(); err != nil {
			return err
		}
		fmt.Println("Сохранённая локальная сессия удалена. Steam-токен НЕ отозван на сервере Valve.")
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Printf("Deadlock Inviter %s | приглашение AccountID=%d\n", version, o.friendID)

	selected, err := authenticate(ctx, o)
	if err != nil {
		return fmt.Errorf("вход в Steam: %w", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		err = sendInvite(ctx, selected.identity, o.friendID, o.location, o.useCache && !selected.fromCache)
		if err == nil {
			return nil
		}
		if !selected.fromCache || !errors.Is(err, errCMAuthRejected) || o.authMode == "cache" {
			return err
		}
		// Retry once with fresh credentials ONLY if Steam rejected the cached
		// token. Do not retry failed GC invitations (might duplicate a request).
		fmt.Println("Сохранённый токен не принят Steam. Удаляю кеш и предлагаю новый вход.")
		if deleteErr := clearCachedSession(); deleteErr != nil {
			fmt.Printf("Не удалось удалить устаревший кеш: %v\n", deleteErr)
		}
		selected, err = authenticate(ctx, options{authMode: "auto", username: o.username, useCache: false})
		if err != nil {
			return fmt.Errorf("повторная авторизация: %w", err)
		}
		selected.fromCache = false
	}
	return err
}

func sendInvite(ctx context.Context, id *authIdentity, accountID uint32, location string, persist bool) error {
	if id == nil || id.accountName == "" || id.refreshToken == "" {
		return errors.New("отсутствует действующая Steam-сессия")
	}
	gcCtx, cancel := context.WithTimeout(ctx, gcTimeout)
	defer cancel()

	client := steam.NewClient()
	defer client.Disconnect()
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	dl := deadlock.New(client, logger)
	defer dl.Close()

	fmt.Println("Подключаюсь к Steam CM без установки Deadlock...")
	client.Connect()
	loggedOn := false
	welcomed := false
	results := make(chan inviteResult, 1)
	retryHello := time.NewTicker(7 * time.Second)
	defer retryHello.Stop()

	for {
		select {
		case <-gcCtx.Done():
			return fmt.Errorf("время ожидания Steam / Deadlock GC истекло: %w", gcCtx.Err())
		case <-retryHello.C:
			if loggedOn && !welcomed {
				dl.SayHello()
			}
		case result := <-results:
			if result.err != nil {
				return fmt.Errorf("Deadlock GC: %w", result.err)
			}
			if result.response == nil {
				return errors.New("Deadlock GC вернул пустой ответ")
			}
			code := int32(result.response.GetResponse())
			fmt.Printf("Ответ Deadlock GC: %v (код %d)\n", result.response.GetResponse(), code)
			return describeGCResponse(code)

		case event, ok := <-client.Events():
			if !ok {
				return errors.New("соединение со Steam закрылось")
			}
			switch e := event.(type) {
			case *steam.ConnectedEvent:
				if err := client.Auth.LogOn(gcCtx, &steam.LogOnDetails{
					Username: id.accountName, AccessToken: id.refreshToken,
				}); err != nil {
					return fmt.Errorf("%w: %v", errCMAuthRejected, err)
				}
			case *steam.LoggedOnEvent:
				if e.Result != steamlang.EResult_OK {
					return fmt.Errorf("%w: код %v", errCMAuthRejected, e.Result)
				}
				loggedOn = true
				fmt.Printf("Steam CM авторизован: %s\n", id.accountName)
				if persist && cacheAvailable() {
					if err := saveCachedSession(id); err != nil {
						fmt.Printf("Не удалось сохранить защищённый кеш (приглашение продолжается): %v\n", err)
					} else {
						fmt.Println("Сессия сохранена в Windows DPAPI (текущий пользователь).")
					}
				}
				dl.SetPlaying(true) // Announces AppID 1422450; does not launch Deadlock.exe.
				dl.SayHello()
			case *steam.LogOnFailedEvent:
				return fmt.Errorf("%w: %v; %v", errCMAuthRejected, e.Result, e.Err)
			case *devents.ClientWelcomed:
				if welcomed {
					continue
				}
				welcomed = true
				fmt.Println("Deadlock GC подключён; отправляю приглашение...")
				go func() {
					response, err := dl.SubmitPlaytestUser(gcCtx, location, accountID)
					select {
					case results <- inviteResult{response: response, err: err}:
					case <-gcCtx.Done():
					}
				}()
			case *steam.SteamFailureEvent:
				return fmt.Errorf("сбой Steam: %v", e.Result)
			case steam.FatalErrorEvent:
				return fmt.Errorf("фатальная ошибка Steam: %v", e)
			case *steam.DisconnectedEvent:
				return errors.New("Steam CM отключился до ответа GC")
			case error:
				return fmt.Errorf("ошибка Steam: %w", e)
			}
		}
	}
}

func describeGCResponse(code int32) error {
	switch code {
	case 0:
		fmt.Println("Success: запрос принят. Подтверждение доставки — у приглашённого друга в Steam.")
		return nil
	case 3:
		return errors.New("InvalidFriend: пользователь не подходит или отсутствует в друзьях")
	case 4:
		return errors.New("NotFriendsLongEnough: недостаточный срок дружбы")
	case 5:
		return errors.New("AlreadyHasGame: у друга уже есть доступ")
	case 6:
		return errors.New("LimitedUser: ограничения аккаунта")
	case 7:
		return errors.New("InviteLimitReached: достигнут лимит инвайтов")
	default:
		return fmt.Errorf("GC отклонил приглашение: код %d", code)
	}
}
