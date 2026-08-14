package azureipamclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type fakeCredential struct {
	calls  int
	scopes []string
}

func (c *fakeCredential) GetToken(_ context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.calls++
	c.scopes = options.Scopes
	return azcore.AccessToken{Token: "token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestClientUsesAndCachesCredentialToken(t *testing.T) {
	credential := &fakeCredential{}
	client, err := NewClient("https://example.test", credential, "api://ipam/.default", false)
	if err != nil {
		t.Fatal(err)
	}
	client.HTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("unexpected Authorization header %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})

	for range 2 {
		req, requestErr := http.NewRequest(http.MethodGet, "https://example.test/api", nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		if _, requestErr = client.doRequest(req); requestErr != nil {
			t.Fatal(requestErr)
		}
	}

	if credential.calls != 1 {
		t.Fatalf("expected one token acquisition, got %d", credential.calls)
	}
	if len(credential.scopes) != 1 || credential.scopes[0] != "api://ipam/.default" {
		t.Fatalf("unexpected scopes %v", credential.scopes)
	}
}

func TestClientRefreshesCredentialTokenNearExpiry(t *testing.T) {
	credential := &fakeCredential{}
	client, err := NewClient("https://example.test", credential, "api://ipam/.default", false)
	if err != nil {
		t.Fatal(err)
	}
	client.accessTokenValue = "expiring-token"
	client.expiresOn = time.Now().Add(4 * time.Minute)

	token, err := client.accessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "token" || credential.calls != 1 {
		t.Fatalf("expected refreshed token, got %q after %d acquisitions", token, credential.calls)
	}
}
