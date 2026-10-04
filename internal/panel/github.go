package panel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"
	githubEmailsURL    = "https://api.github.com/user/emails"
	githubScopes       = "read:user user:email"
)

// GitHubConfig is the authorization-code client for GitHub.
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthorizeURL string
	TokenURL     string
	UserURL      string
	EmailsURL    string
	HTTPClient   *http.Client
}

func (c GitHubConfig) authorizeURL() string {
	if c.AuthorizeURL != "" {
		return c.AuthorizeURL
	}
	return githubAuthorizeURL
}

func (c GitHubConfig) tokenURL() string {
	if c.TokenURL != "" {
		return c.TokenURL
	}
	return githubTokenURL
}

func (c GitHubConfig) userURL() string {
	if c.UserURL != "" {
		return c.UserURL
	}
	return githubUserURL
}

func (c GitHubConfig) emailsURL() string {
	if c.EmailsURL != "" {
		return c.EmailsURL
	}
	return githubEmailsURL
}

func (c GitHubConfig) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c GitHubConfig) authCodeURL(state string) string {
	q := url.Values{
		"client_id":    {c.ClientID},
		"redirect_uri": {c.RedirectURI},
		"scope":        {githubScopes},
		"state":        {state},
	}
	return c.authorizeURL() + "?" + q.Encode()
}

type githubIdentity struct {
	Subject string
	Email   string
}

type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

type githubUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (c GitHubConfig) exchange(code string) (githubIdentity, error) {
	var none githubIdentity
	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.RedirectURI},
	}
	req, err := http.NewRequest(http.MethodPost, c.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return none, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http().Do(req)
	if err != nil {
		return none, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return none, err
	}
	if res.StatusCode >= 300 {
		return none, fmt.Errorf("github token: status %d", res.StatusCode)
	}
	var tok githubTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return none, err
	}
	if tok.Error != "" || tok.AccessToken == "" {
		if tok.ErrorDesc != "" {
			return none, fmt.Errorf("github token: %s", tok.ErrorDesc)
		}
		return none, fmt.Errorf("github token: %s", tok.Error)
	}
	user, err := c.getUser(tok.AccessToken)
	if err != nil {
		return none, err
	}
	email := strings.TrimSpace(user.Email)
	if email == "" {
		email, err = c.primaryEmail(tok.AccessToken)
		if err != nil {
			return none, err
		}
	}
	if user.ID == 0 {
		return none, fmt.Errorf("github user: missing id")
	}
	return githubIdentity{
		Subject: strconv.FormatInt(user.ID, 10),
		Email:   email,
	}, nil
}

func (c GitHubConfig) getUser(token string) (githubUser, error) {
	var user githubUser
	if err := c.getJSON(c.userURL(), token, &user); err != nil {
		return user, err
	}
	return user, nil
}

func (c GitHubConfig) primaryEmail(token string) (string, error) {
	var emails []githubEmail
	if err := c.getJSON(c.emailsURL(), token, &emails); err != nil {
		return "", err
	}
	var fallback string
	for _, item := range emails {
		if item.Email == "" || !item.Verified {
			continue
		}
		if item.Primary {
			return item.Email, nil
		}
		if fallback == "" {
			fallback = item.Email
		}
	}
	return fallback, nil
}

func (c GitHubConfig) getJSON(rawURL, token string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("github api %s: status %d", rawURL, res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(dest)
}
