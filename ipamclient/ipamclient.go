package azureipamclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// Client -
type Client struct {
	HostURL          string
	HTTPClient       *http.Client
	credential       azcore.TokenCredential
	scope            string
	tokenMu          sync.Mutex
	writeMu          sync.Mutex
	accessTokenValue string
	expiresOn        time.Time
}

// NewClient constructs a client that obtains renewable access
// tokens from an Azure credential. The scope is normally api://<app-id>/.default.
func NewClient(host string, credential azcore.TokenCredential, scope string, skipCertificateVerification bool) (*Client, error) {
	var tr http.RoundTripper
	if skipCertificateVerification {
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Skip tls certificate verification if requested
		}
	} else {
		tr = http.DefaultTransport // Use http.DefaultTransport, needed to allow acceptance tests with [jarcoal/httpmock](https://github.com/jarcoal/httpmock)
	}
	c := Client{
		HostURL:    host,
		HTTPClient: &http.Client{Timeout: 10 * time.Second, Transport: tr},
		credential: credential,
		scope:      scope,
	}
	return &c, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.accessTokenValue != "" && time.Until(c.expiresOn) > 5*time.Minute {
		return c.accessTokenValue, nil
	}

	token, err := c.credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{c.scope}})
	if err != nil {
		return "", fmt.Errorf("acquiring Azure IPAM access token: %w", err)
	}
	c.accessTokenValue = token.Token
	c.expiresOn = token.ExpiresOn
	return c.accessTokenValue, nil
}

// doRequest -
func (c *Client) doRequest(req *http.Request) (body []byte, err error) {
	token, err := c.accessToken(req.Context())
	if err != nil {
		return nil, err
	}
	// perform request
	req.Header.Add("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := res.Body.Close(); err == nil {
			err = closeErr
		}
	}()

	// read response body
	body, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// write error not StatusOK
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusAccepted && res.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
	}

	return body, nil
}
