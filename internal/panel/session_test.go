package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieFlagsAndRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	jar, err := newCookieJar([]byte("test-secret"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	sess := Session{Subject: "1", Email: "a@b.com", Allowed: true, Expires: now.Add(time.Hour), IssuedAt: now}
	if err := jar.writeSession(rec, sess); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != sessionCookieName {
		t.Fatalf("name = %s", cookie.Name)
	}
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("flags Secure=%v HttpOnly=%v SameSite=%v", cookie.Secure, cookie.HttpOnly, cookie.SameSite)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	got, err := jar.readSession(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "1" || got.Email != "a@b.com" || !got.Allowed {
		t.Fatalf("session = %#v", got)
	}
}

func TestSessionRejectsTamperAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	jar, err := newCookieJar([]byte("test-secret"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	_ = jar.writeSession(rec, Session{Subject: "1", Expires: now.Add(time.Hour)})
	cookie := rec.Result().Cookies()[0]
	cookie.Value += "x"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	if _, err := jar.readSession(req); err == nil {
		t.Fatal("tampered cookie must fail")
	}

	expired, err := newCookieJar([]byte("test-secret"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	_ = expired.writeSession(rec, Session{Subject: "1", Expires: now.Add(-time.Minute)})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(rec.Result().Cookies()[0])
	// read uses now, and Expires is in the past
	if _, err := expired.readSession(req); err == nil {
		t.Fatal("expired session must fail")
	}
}

func TestNewCookieJarRequiresSecret(t *testing.T) {
	if _, err := newCookieJar(nil, nil); err == nil {
		t.Fatal("empty secret must fail")
	}
}

func TestSessionMalformedToken(t *testing.T) {
	jar, err := newCookieJar([]byte("test-secret"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-token"})
	if _, err := jar.readSession(req); err == nil {
		t.Fatal("malformed session")
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := jar.readState(req); err == nil {
		t.Fatal("missing state")
	}
}

func TestOAuthStateCookie(t *testing.T) {
	jar, err := newCookieJar([]byte("test-secret"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	jar.writeState(rec, "abc")
	c := cookieNamed(rec, oauthStateCookie)
	if c == nil || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie = %#v", c)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	got, err := jar.readState(req)
	if err != nil || got != "abc" {
		t.Fatalf("state = %q err=%v", got, err)
	}
	clear := httptest.NewRecorder()
	jar.clearState(clear)
	jar.clearSession(clear)
	if cookieNamed(clear, sessionCookieName) == nil || cookieNamed(clear, oauthStateCookie) == nil {
		t.Fatal("clear must set cookies")
	}
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
