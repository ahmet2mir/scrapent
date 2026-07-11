package scrapent

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// Client is an authenticated ENT (entcore) HTTP client.
type Client struct {
	httpClient *http.Client
	domain     string
	log        *log.Logger
}

// NewClient authenticates against the ENT space and returns a client whose
// cookie jar carries the session for subsequent requests.
func NewClient(login, password, domain string, logger *log.Logger) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	// ENT answers a successful login with a 302 to the callBack. Do not follow
	// it: the session cookies land on that redirect response, and following it
	// without carrying them bounces back to the login page.
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	data := url.Values{}
	data.Set("email", login)
	data.Set("password", password)
	data.Set("callBack", fmt.Sprintf("https://%s/timeline/timeline", domain))
	data.Set("details", "")
	data.Set("rememberMe", "true")

	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/auth/login", domain), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", fmt.Sprintf("https://%s", domain))
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:142.0) Gecko/20100101 Firefox/142.0")
	req.Header.Set("Accept", "application/json,text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", fmt.Sprintf("https://%s/auth/login?callBack=https%%3A%%2F%%2F%s%%2F", domain, domain))

	logger.Debug("Authenticating", "domain", domain)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	authURL, err := url.Parse(fmt.Sprintf("https://%s", domain))
	if err != nil {
		return nil, err
	}

	for _, ck := range jar.Cookies(authURL) {
		if ck.Name == "oneSessionId" {
			logger.Debug("Authenticated", "domain", domain)
			return &Client{httpClient: httpClient, domain: domain, log: logger}, nil
		}
	}

	body, _ := io.ReadAll(resp.Body)
	return nil, fmt.Errorf("unable to login, got status %d: %s", resp.StatusCode, string(body))
}
