package utils

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPWhitelistMiddleware restricts access to allowed IPs
func IPWhitelistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedIPs := strings.Split(os.Getenv("ALLOWED_IPS"), ",")
		clientIP := net.ParseIP(GetRealIP(r))

		allowed := false
		for _, ip := range allowedIPs {
			if ip == "*" { // For testing
				allowed = true
				break
			}

			_, subnet, err := net.ParseCIDR(ip)
			if err == nil {
				if subnet.Contains(clientIP) {
					allowed = true
					break
				}
				continue
			}

			if net.ParseIP(ip).Equal(clientIP) {
				allowed = true
				break
			}
		}

		if !allowed {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware limits requests per IP
func RateLimitMiddleware(next http.Handler) http.Handler {
	// Map to store rate limiters per IP
	limiters := make(map[string]*rate.Limiter)
	var mu sync.Mutex

	// Create a new limiter: 10 requests per minute
	limiter := rate.NewLimiter(rate.Every(time.Minute/10), 10)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := GetRealIP(r)
		mu.Lock()
		l, exists := limiters[clientIP]
		if !exists {
			l = limiter
			limiters[clientIP] = l
		}
		mu.Unlock()

		if err := l.Wait(r.Context()); err != nil {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetRealIP extracts the real client IP from the request
func GetRealIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}