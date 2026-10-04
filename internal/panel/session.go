package panel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookieName = "panel_session"
	oauthStateCookie  = "panel_oauth_state"
	sessionTTL        = 12 * time.Hour
)

// Session is the signed identity stored in the panel cookie.
type Session struct {
	Subject  string    `json:"sub"`
	Email    string    `json:"email"`
	Allowed  bool      `json:"allowed"`
	Expires  time.Time `json:"exp"`
	IssuedAt time.Time `json:"iat"`
}

type cookieJar struct {
	secret []byte
	now    func() time.Time
	secure bool
}

func newCookieJar(secret []byte, now func() time.Time) (*cookieJar, error) {
	if len(secret) == 0 {
		return nil, errors.New("session secret is required")
	}
	if now == nil {
		now = time.Now
	}
	return &cookieJar{secret: secret, now: now, secure: true}, nil
}

func (j *cookieJar) writeSession(w http.ResponseWriter, sess Session) error {
	raw, err := j.sign(sess)
	if err != nil {
		return err
	}
	http.SetCookie(w, j.cookie(sessionCookieName, raw, sess.Expires))
	return nil
}

func (j *cookieJar) readSession(r *http.Request) (*Session, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := j.verify(c.Value, &sess); err != nil {
		return nil, err
	}
	if !sess.Expires.After(j.now()) {
		return nil, errors.New("session expired")
	}
	return &sess, nil
}

func (j *cookieJar) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, j.cookie(sessionCookieName, "", j.now().Add(-time.Hour)))
}

func (j *cookieJar) writeState(w http.ResponseWriter, state string) {
	http.SetCookie(w, j.cookie(oauthStateCookie, state, j.now().Add(15*time.Minute)))
}

func (j *cookieJar) readState(r *http.Request) (string, error) {
	c, err := r.Cookie(oauthStateCookie)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(c.Value) == "" {
		return "", errors.New("oauth state is empty")
	}
	return c.Value, nil
}

func (j *cookieJar) clearState(w http.ResponseWriter) {
	http.SetCookie(w, j.cookie(oauthStateCookie, "", j.now().Add(-time.Hour)))
}

func (j *cookieJar) cookie(name, value string, exp time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
		Secure:   j.secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func (j *cookieJar) sign(v any) (string, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, j.secret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (j *cookieJar) verify(token string, dest any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return errors.New("malformed session")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, j.secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return errors.New("invalid session signature")
	}
	return json.Unmarshal(payload, dest)
}

func randomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
