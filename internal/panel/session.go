package panel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	sessionCookie = "panel_session"
	stateCookie   = "panel_oidc_state"
	sessionTTL    = 12 * time.Hour
	stateTTL      = 10 * time.Minute
)

type sessionPayload struct {
	Subject string `json:"sub"`
	Login   string `json:"login"`
	Email   string `json:"email"`
	Allowed bool   `json:"allowed"`
	Exp     int64  `json:"exp"`
}

func (s sessionPayload) identity() Identity {
	return Identity{Subject: s.Subject, Login: s.Login, Email: s.Email}
}

func newCookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) writeSession(w http.ResponseWriter, id Identity, allowed bool) error {
	payload := sessionPayload{
		Subject: id.Subject,
		Login:   id.Login,
		Email:   id.Email,
		Allowed: allowed,
		Exp:     s.now().Add(sessionTTL).Unix(),
	}
	value, err := sign(s.cfg.SessionSecret, payload)
	if err != nil {
		return err
	}
	http.SetCookie(w, newCookie(sessionCookie, value, int(sessionTTL.Seconds())))
	return nil
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, newCookie(sessionCookie, "", -1))
	http.SetCookie(w, newCookie(stateCookie, "", -1))
}

func (s *Server) readSession(r *http.Request) (sessionPayload, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return sessionPayload{}, false
	}
	var payload sessionPayload
	if err := verify(s.cfg.SessionSecret, c.Value, &payload); err != nil {
		return sessionPayload{}, false
	}
	if payload.Exp > 0 && payload.Exp < s.now().Unix() {
		return sessionPayload{}, false
	}
	return payload, true
}

func (s *Server) writeState(w http.ResponseWriter) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, newCookie(stateCookie, state, int(stateTTL.Seconds())))
	return state, nil
}

func readState(r *http.Request) string {
	c, err := r.Cookie(stateCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func sign(secret []byte, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := hmac.New(sha256.New, secret)
	_, _ = sum.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sum.Sum(nil)), nil
}

func verify(secret []byte, value string, dest any) error {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return fmt.Errorf("session: malformed")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	sum := hmac.New(sha256.New, secret)
	_, _ = sum.Write(body)
	if !hmac.Equal(sum.Sum(nil), sig) {
		return fmt.Errorf("session: bad signature")
	}
	return json.Unmarshal(body, dest)
}
