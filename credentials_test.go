package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	steam "github.com/paralin/go-steam"
	"github.com/paralin/go-steam/protocol/protobuf/unified"
)

type credentialTransport func(*http.Request) (*http.Response, error)

func (f credentialTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type credentialFixture struct {
	service    *authService
	polls      int
	begins     int
	guardCalls int
	readyAfter int
	guard      unified.EAuthSessionGuardType
	agreement  bool
	cancel     context.CancelFunc
}

func newCredentialFixture(t *testing.T) *credentialFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &credentialFixture{readyAfter: 11, guard: unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceConfirmation}
	f.service = newAuthService()
	f.service.wait = func(ctx context.Context, interval time.Duration) error {
		if interval != 5*time.Second {
			t.Errorf("unexpected polling interval: %v", interval)
		}
		if f.cancel != nil {
			f.cancel()
		}
		return ctx.Err()
	}
	f.service.http.Transport = credentialTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.steampowered.com" {
			return nil, errors.New("unexpected authentication endpoint")
		}
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		blob, err := base64.StdEncoding.DecodeString(r.Form.Get("input_protobuf_encoded"))
		if err != nil {
			return nil, err
		}
		var response marshalerVT
		switch {
		case strings.Contains(r.URL.Path, "GetPasswordRSAPublicKey"):
			if r.Method != http.MethodGet {
				t.Error("RSA request must use GET")
			}
			response = &unified.CAuthentication_GetPasswordRSAPublicKey_Response{
				PublickeyMod: authPtr(key.N.Text(16)), PublickeyExp: authPtr(fmt.Sprintf("%x", key.E)), Timestamp: authPtr(uint64(123)),
			}
		case strings.Contains(r.URL.Path, "BeginAuthSessionViaCredentials"):
			f.begins++
			var request unified.CAuthentication_BeginAuthSessionViaCredentials_Request
			if err := request.UnmarshalVT(blob); err != nil {
				t.Fatal(err)
			}
			if r.Method != http.MethodPost || request.GetAccountName() != "fixture_account" || request.GetEncryptionTimestamp() != 123 {
				t.Error("invalid credentials request")
			}
			ciphertext, err := base64.StdEncoding.DecodeString(request.GetEncryptedPassword())
			if err != nil {
				t.Fatal(err)
			}
			plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, key, ciphertext)
			if err != nil || string(plaintext) != "fixture-password-not-real" {
				t.Fatal("password was not correctly RSA encrypted")
			}
			if bytes.Contains(blob, plaintext) {
				t.Fatal("credentials request contains a plaintext password")
			}
			zeroBytes(plaintext)
			response = &unified.CAuthentication_BeginAuthSessionViaCredentials_Response{
				ClientId: authPtr(uint64(1)), Steamid: authPtr(uint64(76561197960389184)), RequestId: []byte{1, 2}, Interval: authPtr(float32(5)),
				AllowedConfirmations: []*unified.CAuthentication_AllowedConfirmation{{ConfirmationType: &f.guard}},
			}
		case strings.Contains(r.URL.Path, "UpdateAuthSessionWithSteamGuardCode"):
			f.guardCalls++
			var request unified.CAuthentication_UpdateAuthSessionWithSteamGuardCode_Request
			if err := request.UnmarshalVT(blob); err != nil {
				t.Fatal(err)
			}
			if request.GetClientId() != 1 || request.GetSteamid() != 76561197960389184 || request.GetCode() != "ABCDE" || request.GetCodeType() != f.guard {
				t.Error("Guard code was not submitted to the original authentication session")
			}
			response = &unified.CAuthentication_UpdateAuthSessionWithSteamGuardCode_Response{}
		case strings.Contains(r.URL.Path, "PollAuthSessionStatus"):
			var request unified.CAuthentication_PollAuthSessionStatus_Request
			if err := request.UnmarshalVT(blob); err != nil {
				t.Fatal(err)
			}
			expectedID := uint64(1)
			if f.polls > 0 {
				expectedID = 2
			}
			if request.GetClientId() != expectedID || !bytes.Equal(request.GetRequestId(), []byte{1, 2}) {
				t.Error("poll did not preserve session and rotated client ID")
			}
			f.polls++
			result := &unified.CAuthentication_PollAuthSessionStatus_Response{NewClientId: authPtr(uint64(2))}
			if f.agreement {
				result.AgreementSessionUrl = authPtr("https://example.invalid/private-fixture-url")
			} else if f.polls >= f.readyAfter {
				result.RefreshToken = authPtr("fixture-token-not-real")
				result.AccountName = authPtr("fixture_account")
			}
			response = result
		default:
			return nil, errors.New("unexpected authentication method")
		}
		body, err := response.MarshalVT()
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Eresult": {"1"}}, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})
	return f
}

func TestCredentialsWaitBeyondTenPolls(t *testing.T) {
	f := newCredentialFixture(t)
	prompts := 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := f.service.loginCredentials(ctx, "fixture_account", "fixture-password-not-real", func(context.Context, []steam.SteamGuardConfirmation) (string, unified.EAuthSessionGuardType, error) {
		prompts++
		return "", f.guard, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.polls != 11 || f.begins != 1 || prompts != 1 || id.accountName != "fixture_account" || id.refreshToken != "fixture-token-not-real" {
		t.Fatal("login did not wait for the original session's eleventh poll")
	}
}

func TestCredentialsGuardCodeUsesSameSession(t *testing.T) {
	for _, kind := range []unified.EAuthSessionGuardType{unified.EAuthSessionGuardType_k_EAuthSessionGuardType_EmailCode, unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceCode} {
		t.Run(kind.String(), func(t *testing.T) {
			f := newCredentialFixture(t)
			f.guard, f.readyAfter = kind, 1
			_, err := f.service.loginCredentials(context.Background(), "fixture_account", "fixture-password-not-real", func(context.Context, []steam.SteamGuardConfirmation) (string, unified.EAuthSessionGuardType, error) {
				return "ABCDE", kind, nil
			})
			if err != nil || f.begins != 1 || f.guardCalls != 1 || f.polls != 1 {
				t.Fatalf("Guard flow failed: %v", err)
			}
		})
	}
}

func TestCredentialsCancellationAndAgreement(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		f := newCredentialFixture(t)
		f.guard = unified.EAuthSessionGuardType_k_EAuthSessionGuardType_None
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.cancel = cancel
		_, err := f.service.loginCredentials(ctx, "fixture_account", "fixture-password-not-real", nil)
		if !errors.Is(err, context.Canceled) || f.polls != 1 {
			t.Fatalf("polling ignored cancellation: %v", err)
		}
	})
	t.Run("agreement", func(t *testing.T) {
		f := newCredentialFixture(t)
		f.guard, f.agreement = unified.EAuthSessionGuardType_k_EAuthSessionGuardType_None, true
		_, err := f.service.loginCredentials(context.Background(), "fixture_account", "fixture-password-not-real", nil)
		if err == nil || strings.Contains(err.Error(), "private-fixture-url") {
			t.Fatal("agreement was ignored or private URL leaked")
		}
	})
}

func TestAuthenticationDoesNotFollowRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	service := newAuthService()
	service.baseURL = source.URL
	_, err := service.loginCredentials(context.Background(), "fixture_account", "fixture-password-not-real", nil)
	if err == nil || targetCalls != 0 {
		t.Fatal("authentication followed a redirect")
	}
}

func TestEncryptSteamPasswordRejectsInvalidKeys(t *testing.T) {
	for _, key := range [][2]string{{"bad-key", "10001"}, {"1", "10001"}, {strings.Repeat("f", 512), "2"}, {strings.Repeat("f", 512), "fffffffffffffffff"}} {
		if _, err := encryptSteamPassword("fixture", key[0], key[1]); err == nil {
			t.Fatal("invalid RSA key accepted")
		}
	}
}

func TestGuardRejectsInvalidConfirmation(t *testing.T) {
	allowed := unified.EAuthSessionGuardType_k_EAuthSessionGuardType_EmailCode
	begin := &unified.CAuthentication_BeginAuthSessionViaCredentials_Response{
		AllowedConfirmations: []*unified.CAuthentication_AllowedConfirmation{{ConfirmationType: &allowed}},
	}
	service := newAuthService()
	service.http.Transport = credentialTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid Guard input must not be submitted")
		return nil, errors.New("unexpected request")
	})
	for _, test := range []struct {
		name string
		code string
		kind unified.EAuthSessionGuardType
	}{
		{"empty code", "", allowed},
		{"unsupported selection", "ABCDE", unified.EAuthSessionGuardType_k_EAuthSessionGuardType_DeviceCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := service.confirmCredentials(context.Background(), begin, func(context.Context, []steam.SteamGuardConfirmation) (string, unified.EAuthSessionGuardType, error) {
				return test.code, test.kind, nil
			})
			if err == nil {
				t.Fatal("invalid Guard input accepted")
			}
		})
	}
	if err := service.confirmCredentials(context.Background(), begin, nil); err == nil {
		t.Fatal("missing Guard confirmation accepted")
	}
}
