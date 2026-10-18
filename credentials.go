package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"math/big"
	"net/http"
	"strings"

	steam "github.com/paralin/go-steam"
	"github.com/paralin/go-steam/protocol/protobuf/unified"
)

type guardPrompt func(context.Context, []steam.SteamGuardConfirmation) (string, unified.EAuthSessionGuardType, error)

func authPtr[T any](value T) *T { return &value }

// Own the polling loop so a slow mobile approval does not hit go-steam's
// fixed ten-poll limit. Every request belongs to the same authentication session.
func (s *authService) loginCredentials(ctx context.Context, username, password string, prompt guardPrompt) (*authIdentity, error) {
	if username == "" || password == "" {
		return nil, errors.New("логин и пароль Steam обязательны")
	}
	key := new(unified.CAuthentication_GetPasswordRSAPublicKey_Response)
	if err := s.callMethod(ctx, http.MethodGet, "GetPasswordRSAPublicKey", &unified.CAuthentication_GetPasswordRSAPublicKey_Request{AccountName: &username}, key); err != nil {
		return nil, err
	}
	encrypted, err := encryptSteamPassword(password, key.GetPublickeyMod(), key.GetPublickeyExp())
	password = ""
	if err != nil {
		return nil, err
	}
	deviceName := "Deadlock Inviter (Credentials)"
	platform := unified.EAuthTokenPlatformType_k_EAuthTokenPlatformType_SteamClient
	persistence := unified.ESessionPersistence_k_ESessionPersistence_Persistent
	begin := new(unified.CAuthentication_BeginAuthSessionViaCredentials_Response)
	if err := s.call(ctx, "BeginAuthSessionViaCredentials", &unified.CAuthentication_BeginAuthSessionViaCredentials_Request{
		AccountName: &username, EncryptedPassword: &encrypted,
		EncryptionTimestamp: authPtr(key.GetTimestamp()), RememberLogin: authPtr(true),
		DeviceFriendlyName: &deviceName, PlatformType: &platform, Persistence: &persistence,
		WebsiteId:     authPtr("Client"),
		DeviceDetails: &unified.CAuthentication_DeviceDetails{DeviceFriendlyName: &deviceName, PlatformType: &platform},
	}, begin); err != nil {
		return nil, err
	}
	if begin.GetExtendedErrorMessage() != "" {
		// Server-provided messages may contain sensitive authentication details.
		return nil, errors.New("Steam не разрешил начать авторизацию")
	}
	if begin.GetClientId() == 0 || begin.GetSteamid() == 0 || len(begin.GetRequestId()) == 0 {
		return nil, errors.New("Steam не вернул данные сессии авторизации")
	}
	if err := s.confirmCredentials(ctx, begin, prompt); err != nil {
		return nil, err
	}
	session := &qrSession{clientID: begin.GetClientId(), requestID: begin.GetRequestId()}
	interval := normalizePollInterval(begin.GetInterval())
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := s.pollQR(ctx, session)
		if err != nil {
			return nil, err
		}
		if result.GetAgreementSessionUrl() != "" {
			return nil, errors.New("Steam требует принять соглашение в официальном клиенте")
		}
		token := result.GetRefreshToken()
		if token == "" {
			token = result.GetAccessToken()
		}
		if token != "" {
			name := result.GetAccountName()
			if name == "" {
				name = username
			} else if !strings.EqualFold(name, username) {
				return nil, errors.New("Steam вернул сессию другого аккаунта")
			}
			return &authIdentity{accountName: name, refreshToken: token}, nil
		}
		if result.GetNewClientId() != 0 {
			session.clientID = result.GetNewClientId()
		}
		if err := s.waitForPoll(ctx, interval); err != nil {
			return nil, err
		}
	}
}

func (s *authService) confirmCredentials(ctx context.Context, begin *unified.CAuthentication_BeginAuthSessionViaCredentials_Response, prompt guardPrompt) error {
	confirmations := make([]steam.SteamGuardConfirmation, 0, len(begin.GetAllowedConfirmations()))
	for _, confirmation := range begin.GetAllowedConfirmations() {
		kind := confirmation.GetConfirmationType()
		if kind == unified.EAuthSessionGuardType_k_EAuthSessionGuardType_None {
			return nil
		}
		confirmations = append(confirmations, steam.SteamGuardConfirmation{Type: kind})
	}
	if len(confirmations) == 0 {
		return nil
	}
	if prompt == nil {
		return errors.New("Steam требует подтверждение Steam Guard")
	}
	code, kind, err := prompt(ctx, confirmations)
	if err != nil {
		return err
	}
	allowed := false
	for _, confirmation := range confirmations {
		if confirmation.Type == kind {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("выбран недоступный способ Steam Guard")
	}
	switch kind {
	case unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceConfirmation, unified.EAuthSessionGuardType_k_EAuthSessionGuardType_EmailConfirmation:
		return nil
	case unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceCode, unified.EAuthSessionGuardType_k_EAuthSessionGuardType_EmailCode:
		if strings.TrimSpace(code) == "" {
			return errors.New("пустой код Steam Guard")
		}
		response := new(unified.CAuthentication_UpdateAuthSessionWithSteamGuardCode_Response)
		if err := s.call(ctx, "UpdateAuthSessionWithSteamGuardCode", &unified.CAuthentication_UpdateAuthSessionWithSteamGuardCode_Request{
			ClientId: authPtr(begin.GetClientId()), Steamid: authPtr(begin.GetSteamid()), Code: &code, CodeType: &kind,
		}, response); err != nil {
			return err
		}
		if response.GetAgreementSessionUrl() != "" {
			return errors.New("Steam требует принять соглашение в официальном клиенте")
		}
		return nil
	default:
		return errors.New("Steam вернул неподдерживаемый тип Steam Guard")
	}
}

func encryptSteamPassword(password, modulusHex, exponentHex string) (string, error) {
	modulus, ok := new(big.Int).SetString(modulusHex, 16)
	if !ok || modulus.BitLen() < 1024 || modulus.BitLen() > 8192 {
		return "", errors.New("некорректный RSA-модуль Steam")
	}
	exponent, ok := new(big.Int).SetString(exponentHex, 16)
	if !ok || !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 1<<31-1 || exponent.Bit(0) == 0 {
		return "", errors.New("некорректная RSA-экспонента Steam")
	}
	plaintext := []byte(password)
	defer zeroBytes(plaintext)
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}, plaintext)
	if err != nil {
		return "", errors.New("не удалось зашифровать пароль Steam")
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
