package http

import (
	"net/http"
	"time"
)

const (
	AccessTokenCookieName  = "access_token"
	RefreshTokenCookieName = "refresh_token"
	LegacySessionCookie    = "session"

	AccessTokenCookiePath  = "/"
	RefreshTokenCookiePath = "/api/v1/auth"
)

type CookieManager struct {
	secure     bool
	sameSite   http.SameSite
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewCookieManager(secure bool, sameSite http.SameSite, accessTTL, refreshTTL time.Duration) *CookieManager {
	return &CookieManager{
		secure:     secure,
		sameSite:   sameSite,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// SetAuthCookies sets both access_token and refresh_token HttpOnly cookies.
func (cm *CookieManager) SetAuthCookies(w http.ResponseWriter, accessToken, refreshToken string) {
	now := time.Now()

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     AccessTokenCookiePath,
		HttpOnly: true,
		Secure:   cm.secure,
		SameSite: cm.sameSite,
		Expires:  now.Add(cm.accessTTL),
		MaxAge:   int(cm.accessTTL.Seconds()),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     RefreshTokenCookiePath,
		HttpOnly: true,
		Secure:   cm.secure,
		SameSite: cm.sameSite,
		Expires:  now.Add(cm.refreshTTL),
		MaxAge:   int(cm.refreshTTL.Seconds()),
	})
}

// ClearAuthCookies expires both access_token and refresh_token cookies with identical Path and attributes.
func (cm *CookieManager) ClearAuthCookies(w http.ResponseWriter) {
	past := time.Unix(1, 0)

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     AccessTokenCookiePath,
		HttpOnly: true,
		Secure:   cm.secure,
		SameSite: cm.sameSite,
		Expires:  past,
		MaxAge:   -1,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    "",
		Path:     RefreshTokenCookiePath,
		HttpOnly: true,
		Secure:   cm.secure,
		SameSite: cm.sameSite,
		Expires:  past,
		MaxAge:   -1,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     LegacySessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cm.secure,
		SameSite: cm.sameSite,
		Expires:  past,
		MaxAge:   -1,
	})
}
