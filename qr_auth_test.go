package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/paralin/go-steam/protocol/protobuf/unified"
)

func TestParseFriendAccountID(t *testing.T) {
	const fullID uint64 = 76561197960389184
	const accountID uint32 = uint32(fullID - steamID64Base)
	for _, input := range []string{strconv.FormatUint(fullID, 10), strconv.FormatUint(uint64(accountID), 10)} {
		got, err := parseFriendAccountID(input)
		if err != nil || got != accountID {
			t.Fatalf("input=%q: got %d, %v, want %d", input, got, err, accountID)
		}
	}
	for _, input := range []string{"", "0", "friend", "76561197960265728", "18446744073709551615"} {
		if _, err := parseFriendAccountID(input); err == nil {
			t.Fatalf("expected validation error for %q", input)
		}
	}
}

func TestSteamChallengeHostValidation(t *testing.T) {
	for _, good := range []string{"https://s.team/q/1/123/abc", "https://steamcommunity.com/mobilelogin"} {
		if err := validateSteamChallenge(good); err != nil {
			t.Errorf("expected allowed URL %q: %v", good, err)
		}
	}
	for _, bad := range []string{"http://s.team/q/1/abc", "https://s.team.evil.example/q/1", "https://other.example/login", "https://user@s.team/q/1", "https://s.team:123/q/1"} {
		if err := validateSteamChallenge(bad); err == nil {
			t.Errorf("expected rejected URL %q", bad)
		}
	}
}

func TestProtobufQRAuthenticationRequests(t *testing.T) {
	username := "test_account"
	challenge := "https://s.team/q/1/123/abcd"
	clientID := uint64(123)
	requestID := []byte{1, 2, 3, 4}
	pollCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		blob, err := base64.StdEncoding.DecodeString(r.Form.Get("input_protobuf_encoded"))
		if err != nil {
			t.Errorf("base64 request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var encoded []byte
		switch {
		case strings.Contains(r.URL.Path, "BeginAuthSessionViaQR"):
			var req unified.CAuthentication_BeginAuthSessionViaQR_Request
			if err := req.UnmarshalVT(blob); err != nil {
				t.Errorf("begin protobuf: %v", err)
			}
			if req.GetPlatformType() != unified.EAuthTokenPlatformType_k_EAuthTokenPlatformType_SteamClient || req.GetWebsiteId() != "Client" {
				t.Errorf("unexpected QR platform or website: %+v", &req)
			}
			waitSeconds := float32(2)
			resp := &unified.CAuthentication_BeginAuthSessionViaQR_Response{
				ClientId: &clientID, RequestId: requestID, ChallengeUrl: &challenge, Interval: &waitSeconds,
			}
			encoded, err = resp.MarshalVT()
		case strings.Contains(r.URL.Path, "PollAuthSessionStatus"):
			pollCount++
			var req unified.CAuthentication_PollAuthSessionStatus_Request
			if err := req.UnmarshalVT(blob); err != nil {
				t.Errorf("poll protobuf: %v", err)
			}
			if req.GetClientId() != clientID || string(req.GetRequestId()) != string(requestID) {
				t.Errorf("unexpected poll session info: %+v", &req)
			}
			token := "not-a-real-token"
			resp := &unified.CAuthentication_PollAuthSessionStatus_Response{
				AccountName: &username, RefreshToken: &token,
			}
			encoded, err = resp.MarshalVT()
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			t.Errorf("encoding mock response: %v", err)
			return
		}
		w.Header().Set("x-eresult", "1")
		_, _ = w.Write(encoded)
	}))
	defer server.Close()

	service := &authService{baseURL: server.URL + "/IAuthenticationService/", http: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := service.beginQR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if session.clientID != clientID || session.challenge != challenge || session.pollEvery != 2*time.Second {
		t.Fatalf("bad QR session: %+v", session)
	}
	response, err := service.pollQR(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if response.GetAccountName() != username || response.GetRefreshToken() != "not-a-real-token" || pollCount != 1 {
		t.Fatalf("bad QR poll response: name=%q, polls=%d", response.GetAccountName(), pollCount)
	}
}

func TestNoUnexpectedURLDependencies(t *testing.T) {
	parsed, err := url.Parse(steamAuthEndpoint)
	if err != nil || parsed.Hostname() != "api.steampowered.com" || parsed.Scheme != "https" {
		t.Fatal("Steam API endpoint is not the official HTTPS API")
	}
}
