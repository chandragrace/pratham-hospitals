package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "pratham_admin_session"
const sessionSecret = "pratham-secure-session-hmac-secret-2026"

func CheckAdminSession(r *http.Request) (bool, string) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return false, ""
	}

	parts := strings.Split(cookie.Value, ":")
	if len(parts) != 2 {
		return false, ""
	}

	username := parts[0]
	signature := parts[1]

	// Verify HMAC signature
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write([]byte(username))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return true, username
	}
	return false, ""
}

func SetAdminSession(w http.ResponseWriter, username string) {
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write([]byte(username))
	sig := hex.EncodeToString(mac.Sum(nil))

	val := fmt.Sprintf("%s:%s", username, sig)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
}

func ClearAdminSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// RequireAdmin middleware
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isAdmin, _ := CheckAdminSession(r)
		if !isAdmin {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

type AuthHandler struct {
	db   *sql.DB
	tmpl map[string]*template.Template
}

func NewAuthHandler(db *sql.DB, tmpl map[string]*template.Template) *AuthHandler {
	return &AuthHandler{db: db, tmpl: tmpl}
}

func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		isAdmin, _ := CheckAdminSession(r)
		if isAdmin {
			http.Redirect(w, r, "/admin/dashboard", http.StatusFound)
			return
		}

		data := struct {
			Title     string
			ActiveNav string
			IsAdmin   bool
			AdminUser string
			Error     string
		}{
			Title:     "Admin Portal Sign In",
			ActiveNav: "admin",
			IsAdmin:   false,
		}
		t := h.tmpl["admin_login"]
		_ = t.ExecuteTemplate(w, "layout.html", data)
		return
	}

	if r.Method == http.MethodPost {
		username := strings.TrimSpace(r.FormValue("username"))
		password := strings.TrimSpace(r.FormValue("password"))

		var passwordHash string
		var role string
		var active bool

		err := h.db.QueryRow(`
			SELECT password_hash, role, active 
			FROM admin_users 
			WHERE username = $1
		`, username).Scan(&passwordHash, &role, &active)

		if err != nil || !active {
			data := struct {
				Title     string
				ActiveNav string
				IsAdmin   bool
				AdminUser string
				Error     string
			}{
				Title:     "Admin Portal Sign In",
				ActiveNav: "admin",
				Error:     "Invalid credentials or inactive account",
			}
			t := h.tmpl["admin_login"]
			_ = t.ExecuteTemplate(w, "layout.html", data)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
			data := struct {
				Title     string
				ActiveNav string
				IsAdmin   bool
				AdminUser string
				Error     string
			}{
				Title:     "Admin Portal Sign In",
				ActiveNav: "admin",
				Error:     "Invalid password. Please try again.",
			}
			t := h.tmpl["admin_login"]
			_ = t.ExecuteTemplate(w, "layout.html", data)
			return
		}

		// Update last login
		_, _ = h.db.Exec("UPDATE admin_users SET last_login = NOW() WHERE username = $1", username)

		SetAdminSession(w, username)
		http.Redirect(w, r, "/admin/dashboard", http.StatusFound)
	}
}

func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	ClearAdminSession(w)
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}
