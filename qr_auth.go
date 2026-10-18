package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/paralin/go-steam/protocol/protobuf/unified"
	qrcode "github.com/skip2/go-qrcode"
)

const steamAuthEndpoint = "https://api.steampowered.com/IAuthenticationService/"

type qrSession struct {
	clientID  uint64
	requestID []byte
	challenge string
	pollEvery time.Duration
}

type marshalerVT interface {
	MarshalVT() ([]byte, error)
}

type unmarshalerVT interface {
	UnmarshalVT([]byte) error
}

type authService struct {
	baseURL string
	http    *http.Client
	wait    func(context.Context, time.Duration) error
}

func newAuthService() *authService {
	return &authService{
		baseURL: steamAuthEndpoint,
		http: &http.Client{
			Timeout: 18 * time.Second,
			// Authentication must not redirect credentials to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (s *authService) call(ctx context.Context, name string, req marshalerVT, result unmarshalerVT) error {
	return s.callMethod(ctx, http.MethodPost, name, req, result)
}

func (s *authService) callMethod(ctx context.Context, method, name string, req marshalerVT, result unmarshalerVT) error {
	data, err := req.MarshalVT()
	if err != nil {
		return fmt.Errorf("кодирование запроса %s: %w", name, err)
	}
	form := url.Values{"input_protobuf_encoded": {base64.StdEncoding.EncodeToString(data)}}
	endpoint := strings.TrimRight(s.baseURL, "/") + "/" + name + "/v1/"
	var bodyReader io.Reader
	if method == http.MethodGet {
		endpoint += "?" + form.Encode()
	} else {
		bodyReader = strings.NewReader(form.Encode())
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return err
	}
	if method != http.MethodGet {
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	httpResp, err := s.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("Steam API %s: %w", name, err)
	}
	defer httpResp.Body.Close()
	if code := httpResp.Header.Get("x-eresult"); code != "" && code != "1" {
		return fmt.Errorf("Steam API %s: EResult %s", name, code)
	}
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("Steam API %s: HTTP %d", name, httpResp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1024*1024+1))
	if err != nil {
		return err
	}
	if len(body) > 1024*1024 {
		return errors.New("ответ Steam API слишком большой")
	}
	if err := result.UnmarshalVT(body); err != nil {
		return fmt.Errorf("некорректный ответ %s: %w", name, err)
	}
	return nil
}

func (s *authService) beginQR(ctx context.Context) (*qrSession, error) {
	platform := unified.EAuthTokenPlatformType_k_EAuthTokenPlatformType_SteamClient
	deviceName := "Deadlock Inviter (QR)"
	websiteID := "Client"
	request := &unified.CAuthentication_BeginAuthSessionViaQR_Request{
		DeviceFriendlyName: &deviceName,
		PlatformType:       &platform,
		WebsiteId:          &websiteID,
		DeviceDetails: &unified.CAuthentication_DeviceDetails{
			DeviceFriendlyName: &deviceName,
			PlatformType:       &platform,
		},
	}
	response := new(unified.CAuthentication_BeginAuthSessionViaQR_Response)
	if err := s.call(ctx, "BeginAuthSessionViaQR", request, response); err != nil {
		return nil, err
	}
	if response.GetClientId() == 0 || len(response.GetRequestId()) == 0 || response.GetChallengeUrl() == "" {
		return nil, errors.New("Steam API не вернул данные QR-сессии")
	}
	if err := validateSteamChallenge(response.GetChallengeUrl()); err != nil {
		return nil, err
	}
	return &qrSession{
		clientID:  response.GetClientId(),
		requestID: response.GetRequestId(),
		challenge: response.GetChallengeUrl(),
		pollEvery: normalizePollInterval(response.GetInterval()),
	}, nil
}

func normalizePollInterval(seconds float32) time.Duration {
	if math.IsNaN(float64(seconds)) || seconds < 1 || seconds > 30 {
		return 5 * time.Second
	}
	return time.Duration(float64(seconds) * float64(time.Second))
}

func (s *authService) waitForPoll(ctx context.Context, interval time.Duration) error {
	if s.wait != nil {
		return s.wait(ctx, interval)
	}
	return sleepContext(ctx, interval)
}

func validateSteamChallenge(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" {
		return errors.New("Steam вернул некорректный HTTPS-адрес QR")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return errors.New("Steam QR должен использовать стандартный HTTPS-порт")
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "s.team", "steamcommunity.com", "store.steampowered.com":
		return nil
	default:
		return errors.New("Steam вернул QR на неизвестном домене (отказано в целях безопасности)")
	}
}

func (s *authService) pollQR(ctx context.Context, session *qrSession) (*unified.CAuthentication_PollAuthSessionStatus_Response, error) {
	request := &unified.CAuthentication_PollAuthSessionStatus_Request{
		ClientId:  &session.clientID,
		RequestId: session.requestID,
	}
	response := new(unified.CAuthentication_PollAuthSessionStatus_Response)
	if err := s.call(ctx, "PollAuthSessionStatus", request, response); err != nil {
		return nil, err
	}
	return response, nil
}

func loginViaSteamQR(ctx context.Context) (*authIdentity, error) {
	service := newAuthService()
	session, err := service.beginQR(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Println("Вход по QR через официальное приложение Steam (Steam Guard).")
	fmt.Println("Сканируй код в приложении и проверь, что запрос называется 'Deadlock Inviter (QR)'.")
	cleanup, err := displayLocalQR(session.challenge)
	if err != nil {
		return nil, err
	}
	defer func() { cleanup() }()

	scanned := false
	for {
		result, err := service.pollQR(ctx, session)
		if err != nil {
			return nil, err
		}
		if token := result.GetRefreshToken(); token != "" {
			if name := result.GetAccountName(); name != "" {
				return &authIdentity{accountName: name, refreshToken: token}, nil
			}
			return nil, errors.New("Steam подтвердил QR, но не вернул имя аккаунта для входа в CM")
		}
		if agreement := result.GetAgreementSessionUrl(); agreement != "" {
			return nil, errors.New("Steam требует принять соглашение перед авторизацией; открой официальный клиент Steam")
		}
		if result.GetHadRemoteInteraction() && !scanned {
			fmt.Println("QR отсканирован. Подтверди вход в приложении Steam.")
			scanned = true
		}
		// Steam can rotate QR challenges before they expire.
		if result.GetNewClientId() != 0 {
			session.clientID = result.GetNewClientId()
		}
		if rotated := result.GetNewChallengeUrl(); rotated != "" && rotated != session.challenge {
			if err := validateSteamChallenge(rotated); err != nil {
				return nil, err
			}
			session.challenge = rotated
			cleanup()
			fmt.Println("Steam обновил QR-код. Отсканируй новый.")
			cleanup, err = displayLocalQR(rotated)
			if err != nil {
				return nil, err
			}
			scanned = false
		}
		if err := service.waitForPoll(ctx, session.pollEvery); err != nil {
			return nil, err
		}
	}
}

func sleepContext(ctx context.Context, pause time.Duration) error {
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Generates a QR image only on this computer. No third-party QR API receives
// the challenge URL. The temporary image is deleted at the end of the session.
func displayLocalQR(challenge string) (func(), error) {
	qr, err := qrcode.New(challenge, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("создание QR: %w", err)
	}
	png, err := qr.PNG(480)
	if err != nil {
		return nil, fmt.Errorf("PNG QR: %w", err)
	}
	file, err := os.CreateTemp("", "deadlock-steam-login-*.png")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := file.Write(png); err != nil {
		_ = file.Close()
		cleanup()
		return nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, err
	}

	// System image viewer shows the locally generated QR (not a QR website).
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	fmt.Printf("QR-код: %s\n", filepath.Clean(path))
	if err := cmd.Start(); err != nil {
		fmt.Printf("Не получилось открыть изображение автоматически (%v). Открой файл вручную.\n", err)
		// A terminal fallback when no image viewer is installed.
		fmt.Println(qr.ToSmallString(false))
	} else {
		// Reap the launcher asynchronously; it doesn't affect QR authentication.
		go func() { _ = cmd.Wait() }()
	}
	return cleanup, nil
}
