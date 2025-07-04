package main

import (
	"axcommutator/app/db"
	"axcommutator/app/handlers"
	"axcommutator/app/utils"
	"axcommutator/app/config"
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func main() {
	validateEnv()
	config.LoadServices()

	logger := initLogger()
	defer logger.Sync()

	db := initDB(logger)
	defer db.Close()

	r := createRouter(logger)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":" + getPort(),
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	startServer(srv, logger)
}

func validateEnv() {
	required := []string{"CSRF_KEY", "DB_PATH"}
	for _, env := range required {
		if os.Getenv(env) == "" {
			log.Fatalf("%s environment variable is required", env)
		}
	}
}

func initLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}
	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	// logger.Info("Logger initialized")
	return logger
}

func initDB(logger *zap.Logger) *sql.DB {
	db, err := db.InitDB(logger)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	return db
}

func createRouter(logger *zap.Logger) *mux.Router {
	r := mux.NewRouter()

	r.Use(
		LoggingMiddleware(logger),
		RecoveryMiddleware(logger),
	)

	api := r.PathPrefix("/api/v1").Subrouter()
	api.Use(
		createCSRFMiddleware(),
		utils.IPWhitelistMiddleware,
		utils.RateLimitMiddleware,
	)

	api.HandleFunc("/order", handlers.HandleProjectOrder).Methods("POST")
	api.HandleFunc("/cookie-consent", handlers.HandleCookieConsent).Methods("POST")
	api.HandleFunc("/health", handlers.HealthCheck).Methods("GET")
	api.HandleFunc("/csrf-token", handlers.GetCSRFToken).Methods("GET")

	return r
}

func createCSRFMiddleware() func(http.Handler) http.Handler {
	return csrf.Protect(
		[]byte(os.Getenv("CSRF_KEY")),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.MaxAge(3600),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// log.Printf("🛡️  CSRF FAILURE:")
			// log.Printf("  ↪ X-CSRF-Token: %s", r.Header.Get("X-CSRF-Token"))
			// log.Printf("  ↪ Cookie:       %s", r.Header.Get("Cookie"))
			// log.Printf("  ↪ Origin:       %s", r.Header.Get("Origin"))
			// log.Printf("  ↪ Referer:      %s", r.Header.Get("Referer"))
			// log.Printf("  ↪ Failure:      %v", r.Context().Value("gorilla.csrf.Error"))
			handlers.CSRFFailureHandler(w, r)
		})),
	)
}

func getPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8600"
}

func startServer(srv *http.Server, logger *zap.Logger) {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// logger.Info("Starting server",
		// 	zap.String("address", srv.Addr),
		// 	zap.String("environment", os.Getenv("ENV")))

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	<-done
	// logger.Info("Server received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown failed", zap.Error(err))
	} else {
		// logger.Info("Server shutdown gracefully")
	}
}

func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// start := time.Now() 
			rw := &responseWriter{w, http.StatusOK}
			next.ServeHTTP(rw, r)
			// logger.Info("Request processed",
			// 	zap.String("method", r.Method),
			// 	zap.String("path", r.URL.Path),
			// 	zap.Int("status", rw.status),
			// 	zap.String("ip", utils.GetRealIP(r)),
			// 	zap.Duration("duration", time.Since(start)))
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func RecoveryMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("Recovered from panic",
						zap.Any("error", err),
						zap.String("path", r.URL.Path),
						zap.String("method", r.Method),
						zap.String("ip", utils.GetRealIP(r)))

					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}