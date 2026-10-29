package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupAdminTestDB(t *testing.T) {
	t.Helper()

	var err error
	sysDB, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sys db: %v", err)
	}
	t.Cleanup(func() {
		sysDB.Close()
	})

	mustExec(t, sysDB, `CREATE TABLE admin_users (username TEXT PRIMARY KEY, password TEXT)`)
}

func TestPasswordHashDoesNotStorePlaintext(t *testing.T) {
	hash, err := hashPassword("12345")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if hash == "12345" {
		t.Fatal("password hash stored plaintext")
	}
	if !verifyPassword(hash, "12345") {
		t.Fatal("hashed password should verify")
	}
	if verifyPassword(hash, "wrong") {
		t.Fatal("wrong password should not verify")
	}
}

func TestAdminLoginSetsHardenedCookieAndUpgradesLegacyPassword(t *testing.T) {
	setupAdminTestDB(t)
	mustExec(t, sysDB, `INSERT INTO admin_users VALUES ('admin', '12345')`)

	form := url.Values{"username": {"admin"}, "password": {"12345"}}
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handleAdminLogin(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Name != "session" || cookies[0].Value == "" || cookies[0].Value == "active" {
		t.Fatalf("unexpected session cookie: %#v", cookies[0])
	}
	if !cookies[0].HttpOnly {
		t.Fatal("session cookie should be HttpOnly")
	}
	if cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want Lax", cookies[0].SameSite)
	}

	var stored string
	if err := sysDB.QueryRow(`SELECT password FROM admin_users WHERE username='admin'`).Scan(&stored); err != nil {
		t.Fatalf("read upgraded password: %v", err)
	}
	if stored == "12345" || !verifyPassword(stored, "12345") {
		t.Fatalf("legacy password was not upgraded: %q", stored)
	}
}

func TestRequireAdminRejectsMissingOrInvalidSession(t *testing.T) {
	sessionToken = "expected-token"
	called := false
	handler := requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(http.MethodGet, "/api/config/get", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if called {
		t.Fatal("handler should not be called without session")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/config/get", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "expected-token"})
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("valid session status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if !called {
		t.Fatal("handler should be called with a valid session")
	}
}
