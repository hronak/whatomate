package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/assignment"
	"github.com/shridarpatil/whatomate/internal/calling"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/database"
	"github.com/shridarpatil/whatomate/internal/frontend"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/middleware"
	"github.com/shridarpatil/whatomate/internal/queue"
	"github.com/shridarpatil/whatomate/internal/storage"
	"github.com/shridarpatil/whatomate/internal/tts"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"github.com/shridarpatil/whatomate/internal/worker"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
	"golang.org/x/sync/errgroup"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "install":
		runInstall(os.Args[2:])
	case "server":
		runServer(os.Args[2:])
	case "worker":
		runWorker(os.Args[2:])
	case "version":
		fmt.Printf("Whatomate %s (built %s)\n", Version, BuildTime)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Whatomate - WhatsApp Business API Platform

Usage:
  whatomate <command> [options]

Commands:
  install   Initialize the database, then exit
  server    Start the API server (with optional embedded workers)
  worker    Start background workers only (no API server)
  version   Show version information
  help      Show this help message

Install Options:
  -config string    Path to config file (default "config.toml")
  -idempotent       Exit successfully if the database is already installed
  -yes              Don't prompt before migrating a database that has tables
  -seed             Also insert demo contacts, tags and a starter chatbot flow

Server Options:
  -config string       Path to config file (default "config.toml")
  -migrate             Run database migrations on startup
  -workers int         Number of embedded workers (0 to disable) (default 1)
  -frontend-dir string Serve the frontend from this directory instead of the
                       copy embedded in the binary (development)

Worker Options:
  -config string    Path to config file (default "config.toml")
  -workers int      Number of workers to run (default 1)

Examples:
  whatomate install                    # Set up a fresh database, then exit
  whatomate install -seed              # ...and add demo data to look at
  whatomate install -idempotent -yes   # Safe to run unconditionally in scripts
  whatomate server                     # API + 1 embedded worker
  whatomate server -workers 0          # API only (no workers)
  whatomate server -workers 4          # API + 4 embedded workers
  whatomate server -migrate            # Run migrations and start server
  whatomate worker -workers 4          # 4 workers only (no API)

Deployment Scenarios:
  All-in-one:    whatomate server
  Separate:      whatomate server -workers 0  (on API server)
                 whatomate worker -workers 4  (on worker server)`)
}

// ============================================================================
// SERVER COMMAND
// ============================================================================

func runServer(args []string) {
	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := serverFlags.String("config", "config.toml", "Path to config file")
	migrate := serverFlags.Bool("migrate", false, "Run database migrations")
	numWorkers := serverFlags.Int("workers", 1, "Number of workers to run (0 to disable embedded workers)")
	frontendDir := serverFlags.String("frontend-dir", "", "Serve the frontend from this directory instead of the embedded copy")
	_ = serverFlags.Parse(args)

	// Initialize logger
	lo := logf.New(logf.Opts{
		EnableColor:     true,
		Level:           logf.DebugLevel,
		EnableCaller:    true,
		TimestampFormat: "2006-01-02 15:04:05",
		DefaultFields:   []any{"app", "whatomate"},
	})

	lo.Info("Starting Whatomate server...", "version", Version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		lo.Fatal("Failed to load config", "error", err)
	}

	// An explicit flag beats whatever the config file said.
	if *frontendDir != "" {
		cfg.App.FrontendDir = *frontendDir
	}

	// Validate JWT secret
	if cfg.App.Environment == "production" && len(cfg.JWT.Secret) < 32 {
		lo.Fatal("JWT secret must be at least 32 characters in production")
	}
	if cfg.JWT.Secret == "" {
		lo.Warn("JWT secret is empty, using a random secret (tokens will not persist across restarts)")
	}

	// Warn if debug mode is on in production
	if cfg.App.Environment == "production" && cfg.App.Debug {
		lo.Warn("Debug mode is enabled in production! This may expose sensitive information.")
	}

	// Require explicit CORS origins in production
	if cfg.App.Environment == "production" && cfg.Server.AllowedOrigins == "" {
		lo.Fatal("server.allowed_origins must be set in production (e.g. \"https://app.example.com\")")
	}

	// Set log level based on environment
	if cfg.App.Environment == "production" {
		lo = logf.New(logf.Opts{
			Level:           logf.InfoLevel,
			TimestampFormat: "2006-01-02 15:04:05",
			DefaultFields:   []any{"app", "whatomate"},
		})
	}

	// Connect to PostgreSQL
	db, err := database.NewPostgres(&cfg.Database, cfg.App.Debug)
	if err != nil {
		lo.Fatal("Failed to connect to database", "error", err)
	}
	lo.Info("Connected to PostgreSQL")

	// Run migrations if requested. Same work the `install` command does, minus
	// the seeding and the prompts.
	if *migrate {
		if err := migrateSchema(db, cfg, lo, os.Stdout); err != nil {
			lo.Fatal("Migration failed", "error", err)
		}
	}

	// Connect to Redis
	rdb, err := database.NewRedis(&cfg.Redis)
	if err != nil {
		lo.Fatal("Failed to connect to Redis", "error", err)
	}
	lo.Info("Connected to Redis")

	// Initialize job queue
	jobQueue := queue.NewRedisQueue(rdb, lo)
	lo.Info("Job queue initialized")

	// Initialize Fastglue
	g := fastglue.NewGlue()

	// Initialize WhatsApp client
	waClient := whatsapp.New(whatsapp.WithLogger(lo), whatsapp.WithBaseURL(cfg.WhatsApp.BaseURL))

	// Initialize WebSocket hub
	wsHub := websocket.NewHub(lo)
	go wsHub.Run()
	lo.Info("WebSocket hub started")

	// Initialize app with dependencies
	// Shared HTTP client with connection pooling for external API calls
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext:         handlers.SSRFSafeDialer(),
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	app := &handlers.App{
		Config:     cfg,
		DB:         db,
		Redis:      rdb,
		Log:        lo,
		WhatsApp:   waClient,
		WSHub:      wsHub,
		Queue:      jobQueue,
		HTTPClient: httpClient,
	}

	// Initialize S3 client for call recordings (optional)
	var s3Client *storage.S3Client
	if cfg.Calling.RecordingEnabled && cfg.Storage.S3Bucket != "" {
		var err error
		s3Client, err = storage.NewS3Client(&cfg.Storage)
		if err != nil {
			lo.Warn("Failed to initialize S3 client for recordings, recording disabled", "error", err)
		} else {
			lo.Info("S3 client initialized for call recordings", "bucket", cfg.Storage.S3Bucket)
		}
	}

	// Initialize shared assignment engine (used by both chat and call transfers)
	assigner := assignment.New(db, rdb, lo)
	app.Assigner = assigner

	// Initialize CallManager (per-org calling_enabled DB setting controls access)
	app.CallManager = calling.NewManager(&cfg.Calling, s3Client, db, rdb, waClient, wsHub, assigner, lo)
	app.S3Client = s3Client
	lo.Info("Call manager initialized")

	// Initialize TTS based on configured provider
	switch cfg.TTS.Provider {
	case "openai":
		app.TTS = &tts.OpenAITTS{
			APIKey:   cfg.TTS.OpenAIKey,
			Voice:    cfg.TTS.OpenAIVoice,
			AudioDir: cfg.Calling.AudioDir,
		}
		lo.Info("TTS initialized", "provider", "openai", "voice", cfg.TTS.OpenAIVoice)
	case "elevenlabs":
		app.TTS = &tts.ElevenLabsTTS{
			APIKey:   cfg.TTS.ElevenLabsKey,
			VoiceID:  cfg.TTS.ElevenLabsVoiceID,
			AudioDir: cfg.Calling.AudioDir,
		}
		lo.Info("TTS initialized", "provider", "elevenlabs", "voice", cfg.TTS.ElevenLabsVoiceID)
	case "google":
		app.TTS = &tts.GoogleTTS{
			CredentialsJSON: []byte(cfg.TTS.GoogleCredentialsJSON),
			VoiceName:       cfg.TTS.GoogleVoiceName,
			AudioDir:        cfg.Calling.AudioDir,
		}
		lo.Info("TTS initialized", "provider", "google", "voice", cfg.TTS.GoogleVoiceName)
	default:
		if cfg.TTS.Provider != "" {
			lo.Warn("Unknown TTS provider configured", "provider", cfg.TTS.Provider)
		}
	}

	// Start campaign stats subscriber for real-time WebSocket updates from worker
	if err := app.StartCampaignStatsSubscriber(); err != nil {
		lo.Error("Failed to start campaign stats subscriber", "error", err)
	}

	// Parse allowed origins for CORS
	allowedOrigins := middleware.ParseAllowedOrigins(cfg.Server.AllowedOrigins)

	// Setup middleware (CORS is handled by corsWrapper at fasthttp level)
	g.Before(middleware.SecurityHeaders())
	g.Before(middleware.RequestLogger(lo))
	g.Before(middleware.Recovery(lo))
	g.Before(middleware.CSRFProtection())

	// Setup routes
	setupRoutes(g, app, lo, cfg.Server.BasePath, rdb, cfg)

	// Create server with CORS wrapper
	server := &fasthttp.Server{
		Handler:            corsWrapper(g.Handler(), allowedOrigins),
		ReadTimeout:        time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout:       time.Duration(cfg.Server.WriteTimeout) * time.Second,
		MaxRequestBodySize: 15 * 1024 * 1024,
		// Headers, not body — see ServerConfig.ReadBufferSize. fasthttp's 4KB
		// default drops the connection outright once cookies and forwarded
		// headers add up, which surfaces as an unexplained failed request.
		ReadBufferSize: cfg.Server.ReadBufferSize,
		Name:           "Whatomate",
	}

	// Start server in goroutine. A listen failure signals the shutdown path
	// rather than calling lo.Fatal: os.Exit from here would skip every cleanup
	// step below, dropping in-flight requests, live calls and queued work.
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	serverFailed := make(chan error, 1)
	go func() {
		lo.Info("Server listening", "address", addr)
		if err := server.ListenAndServe(addr); err != nil {
			serverFailed <- err
		}
	}()

	// Start frontend dev server if in development mode
	var viteCmd *exec.Cmd
	var viteExited chan error
	if cfg.App.Environment == "development" {
		cmd := exec.Command("npm", "run", "dev")
		cmd.Dir = "frontend"
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		if err := cmd.Start(); err != nil {
			lo.Error("Failed to start frontend dev server", "error", err)
		} else {
			viteCmd = cmd
			viteExited = make(chan error, 1)
			frontendPort := os.Getenv("FRONTEND_PORT")
			if frontendPort == "" {
				frontendPort = "3000"
			}
			lo.Info("Frontend dev server started (Vite)")
			lo.Info(fmt.Sprintf("App      http://localhost:%s   <-- open this", frontendPort))

			go func() {
				viteExited <- cmd.Wait()
			}()
		}
	}

	// Start SLA processor (runs every minute)
	slaProcessor := handlers.NewSLAProcessor(app, time.Minute)
	slaCtx, slaCancel := context.WithCancel(context.Background())
	go slaProcessor.Start(slaCtx)
	lo.Info("SLA processor started")

	// Start embedded workers under an errgroup so shutdown can actually await
	// them. Previously they were cancelled but never waited for, so an
	// in-flight job was abandoned mid-send.
	var workers []*worker.Worker
	workerCtx, workerCancel := context.WithCancel(context.Background())
	workerGroup, workerGroupCtx := errgroup.WithContext(workerCtx)
	if *numWorkers > 0 {
		for i := range *numWorkers {
			w, err := worker.New(cfg, db, rdb, lo)
			if err != nil {
				lo.Fatal("Failed to create worker", "error", err, "worker_num", i+1)
			}
			workers = append(workers, w)

			workerNum := i + 1
			workerGroup.Go(func() error {
				lo.Info("Worker started", "worker_num", workerNum)
				if err := w.Run(workerGroupCtx); err != nil && !errors.Is(err, context.Canceled) {
					lo.Error("Worker error", "error", err, "worker_num", workerNum)
					return err
				}
				return nil
			})
		}
		lo.Info("Embedded workers started", "count", *numWorkers)
	} else {
		lo.Info("Embedded workers disabled, run workers separately")
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		lo.Info("Shutting down...")
	case err := <-serverFailed:
		lo.Error("Server failed, shutting down", "error", err)
	case err := <-viteExited:
		lo.Error("Vite frontend dev server exited unexpectedly, shutting down", "error", err)
	}

	// Order matters. Each step must stop producing work for the step after it,
	// so nothing is left half-done:
	//
	//  1. stop accepting requests and drain the in-flight ones — they may still
	//     enqueue jobs and spawn background tasks;
	//  2. stop the SLA processor, the other producer of transfer work;
	//  3. cancel and *await* the workers, which consume that queue;
	//  4. wait for App's background tasks (webhook dispatch, async sends,
	//     audit writes) that steps 1-3 may have spawned;
	//  5. close the Redis subscriber, whose only job was feeding the hub;
	//  6. end live calls cleanly so recordings finalize and upload;
	//  7. stop the hub last — everything above may broadcast to it.

	if viteCmd != nil && viteCmd.Process != nil {
		lo.Info("Stopping Vite frontend dev server...")
		_ = syscall.Kill(-viteCmd.Process.Pid, syscall.SIGTERM)
		lo.Info("Vite frontend dev server stopped")
	}

	lo.Info("Stopping server...")
	if err := server.Shutdown(); err != nil {
		lo.Error("Server shutdown error", "error", err)
	}
	lo.Info("Server stopped")

	lo.Info("Stopping SLA processor...")
	slaCancel()
	slaProcessor.Stop()
	lo.Info("SLA processor stopped")

	lo.Info("Stopping workers...", "count", len(workers))
	workerCancel()
	if err := workerGroup.Wait(); err != nil {
		lo.Error("Worker group exited with error", "error", err)
	}
	for _, w := range workers {
		if err := w.Close(); err != nil {
			lo.Error("Worker close error", "error", err)
		}
	}
	lo.Info("Workers stopped")

	lo.Info("Waiting for background tasks...")
	app.WaitForBackgroundTasks()
	lo.Info("Background tasks complete")

	lo.Info("Stopping campaign stats subscriber...")
	app.StopCampaignStatsSubscriber()
	lo.Info("Campaign stats subscriber stopped")

	if app.CallManager != nil {
		lo.Info("Ending active calls...")
		callCtx, callCancel := context.WithTimeout(context.Background(), shutdownCallTimeout)
		app.CallManager.Shutdown(callCtx)
		callCancel()
		lo.Info("Active calls ended")
	}

	lo.Info("Stopping WebSocket hub...")
	wsHub.Stop()
	lo.Info("WebSocket hub stopped")

	lo.Info("Shutdown complete")
}

// shutdownCallTimeout bounds how long shutdown waits for live calls to end
// cleanly (peer connections closed, recordings finalized and uploaded).
const shutdownCallTimeout = 30 * time.Second

// ============================================================================
// WORKER COMMAND
// ============================================================================

func runWorker(args []string) {
	workerFlags := flag.NewFlagSet("worker", flag.ExitOnError)
	configPath := workerFlags.String("config", "config.toml", "Path to config file")
	workerCount := workerFlags.Int("workers", 1, "Number of workers to run")
	_ = workerFlags.Parse(args)

	// Initialize logger
	lo := logf.New(logf.Opts{
		EnableColor:     true,
		Level:           logf.DebugLevel,
		EnableCaller:    true,
		TimestampFormat: "2006-01-02 15:04:05",
		DefaultFields:   []any{"app", "whatomate-worker"},
	})

	lo.Info("Starting Whatomate worker...", "version", Version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		lo.Fatal("Failed to load config", "error", err)
	}

	// Set log level based on environment
	if cfg.App.Environment == "production" {
		lo = logf.New(logf.Opts{
			Level:           logf.InfoLevel,
			TimestampFormat: "2006-01-02 15:04:05",
			DefaultFields:   []any{"app", "whatomate-worker"},
		})
	}

	// Connect to PostgreSQL
	db, err := database.NewPostgres(&cfg.Database, cfg.App.Debug)
	if err != nil {
		lo.Fatal("Failed to connect to database", "error", err)
	}
	lo.Info("Connected to PostgreSQL")

	// Connect to Redis
	rdb, err := database.NewRedis(&cfg.Redis)
	if err != nil {
		lo.Fatal("Failed to connect to Redis", "error", err)
	}
	lo.Info("Connected to Redis")

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Create and run workers under an errgroup, so shutdown awaits the
	// in-flight job instead of abandoning it mid-send.
	workers := make([]*worker.Worker, *workerCount)
	group, groupCtx := errgroup.WithContext(ctx)
	errCh := make(chan error, *workerCount)

	for i := range *workerCount {
		w, err := worker.New(cfg, db, rdb, lo)
		if err != nil {
			lo.Fatal("Failed to create worker", "error", err, "worker_num", i+1)
		}
		workers[i] = w

		workerNum := i + 1
		group.Go(func() error {
			lo.Info("Worker started", "worker_num", workerNum)
			err := w.Run(groupCtx)
			errCh <- err
			return err
		})
	}

	lo.Info("Workers started", "count", *workerCount)

	// Wait for shutdown signal or error
	select {
	case sig := <-quit:
		lo.Info("Received shutdown signal", "signal", sig)
		cancel()
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			lo.Error("Worker error", "error", err)
		}
		cancel()
	}

	// Cleanup
	lo.Info("Shutting down workers...")
	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		lo.Error("Worker group exited with error", "error", err)
	}
	for _, w := range workers {
		if w != nil {
			if err := w.Close(); err != nil {
				lo.Error("Error closing worker", "error", err)
			}
		}
	}
	lo.Info("Workers stopped")
}

// ============================================================================
// ROUTES
// ============================================================================

func setupRoutes(g *fastglue.Fastglue, app *handlers.App, lo logf.Logger, basePath string, rdb *redis.Client, cfg *config.Config) {
	// Per-endpoint rate limiters, referenced by name from the route table.
	limiters := map[string]rateLimiter{}
	if cfg.RateLimit.Enabled {
		window := time.Duration(cfg.RateLimit.WindowSeconds) * time.Second
		lo.Info("Rate limiting enabled on auth endpoints",
			"login_max", cfg.RateLimit.LoginMaxAttempts,
			"register_max", cfg.RateLimit.RegisterMaxAttempts,
			"refresh_max", cfg.RateLimit.RefreshMaxAttempts,
			"sso_max", cfg.RateLimit.SSOMaxAttempts,
			"window_seconds", cfg.RateLimit.WindowSeconds)

		bucket := func(keyPrefix string, maxAttempts int) rateLimiter {
			return func(h fastglue.FastRequestHandler) fastglue.FastRequestHandler {
				return withRateLimit(h, middleware.RateLimitOpts{
					Redis: rdb, Log: lo, Max: maxAttempts, Window: window,
					KeyPrefix: keyPrefix, TrustProxy: cfg.RateLimit.TrustProxy,
				})
			}
		}
		limiters["login"] = bucket("login", cfg.RateLimit.LoginMaxAttempts)
		limiters["register"] = bucket("register", cfg.RateLimit.RegisterMaxAttempts)
		limiters["refresh"] = bucket("refresh", cfg.RateLimit.RefreshMaxAttempts)
		limiters["sso_init"] = bucket("sso_init", cfg.RateLimit.SSOMaxAttempts)
		limiters["sso_callback"] = bucket("sso_callback", cfg.RateLimit.SSOMaxAttempts)
	}

	// Auth: applied to everything under /api except the table's public routes.
	public := publicPaths()
	g.Before(func(r *fastglue.Request) *fastglue.Request {
		// Skip auth for OPTIONS preflight requests (handled by CORS middleware)
		if string(r.RequestCtx.Method()) == fasthttp.MethodOptions {
			return r
		}
		path := string(r.RequestCtx.Path())
		if isPublicPath(path, public) {
			return r
		}
		// Apply auth for all other /api routes (supports both JWT and API key)
		if strings.HasPrefix(path, "/api") {
			return middleware.AuthWithDB(app.Config.JWT.Secret, app.DB)(r)
		}
		return r
	})

	// Global rate limit on all /api/ routes, keyed by user ID (or IP if unauthenticated).
	// Runs after auth so the user identity is available.
	if cfg.RateLimit.Enabled {
		apiRL := middleware.UserAwareRateLimit(middleware.RateLimitOpts{
			Redis:      rdb,
			Log:        lo,
			Max:        cmp.Or(cfg.RateLimit.APIMaxRequests, 200),
			Window:     time.Duration(cmp.Or(cfg.RateLimit.APIWindowSeconds, 60)) * time.Second,
			KeyPrefix:  "api_global",
			TrustProxy: cfg.RateLimit.TrustProxy,
		})
		g.Before(func(r *fastglue.Request) *fastglue.Request {
			if strings.HasPrefix(string(r.RequestCtx.Path()), "/api") {
				return apiRL(r)
			}
			return r
		})
	}

	if !cfg.App.EnforceRoutePermissions {
		lo.Warn("Route permissions are in SHADOW MODE: denials are logged, not enforced. " +
			"Set app.enforce_route_permissions=true once the logs are clean.")
	}
	registerRoutes(g, app, lo, cfg.App.EnforceRoutePermissions, limiters)

	// Serve the frontend (SPA). A directory on disk wins over the embedded copy
	// so development can point at frontend/dist and always see the current
	// build; shipped binaries leave frontend_dir empty and use the embedded one.
	var frontendHandler fasthttp.RequestHandler
	switch {
	case cfg.App.FrontendDir != "":
		if !frontend.DirHasIndex(cfg.App.FrontendDir) {
			lo.Warn("app.frontend_dir has no index.html — run 'make frontend-build'",
				"dir", cfg.App.FrontendDir)
		}
		lo.Info("Serving frontend from disk", "dir", cfg.App.FrontendDir, "base_path", basePath)
		frontendHandler = frontend.DirHandler(basePath, cfg.App.FrontendDir)
	case frontend.IsEmbedded() && cfg.App.Environment != "development":
		lo.Info("Serving embedded frontend", "base_path", basePath)
		frontendHandler = frontend.Handler(basePath)
	default:
		lo.Info("Frontend not embedded, API-only mode")
	}

	if frontendHandler != nil {
		// Catch-all for frontend routes
		g.GET("/{path:*}", func(r *fastglue.Request) error {
			frontendHandler(r.RequestCtx)
			return nil
		})
		g.GET("/", func(r *fastglue.Request) error {
			frontendHandler(r.RequestCtx)
			return nil
		})
	}
}

// withRateLimit wraps a handler with the rate limit middleware.
func withRateLimit(handler fastglue.FastRequestHandler, opts middleware.RateLimitOpts) fastglue.FastRequestHandler {
	rl := middleware.RateLimit(opts)
	return func(r *fastglue.Request) error {
		if rl(r) == nil {
			return nil // Rate limited — response already sent.
		}
		return handler(r)
	}
}

// corsWrapper wraps a handler with CORS support at the fasthttp level.
// This ensures CORS headers are set even for auto-handled OPTIONS requests.
func corsWrapper(next fasthttp.RequestHandler, allowedOrigins map[string]bool) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		origin := string(ctx.Request.Header.Peek("Origin"))

		if origin != "" && middleware.IsOriginAllowed(origin, allowedOrigins) {
			ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
			ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
		} else if len(allowedOrigins) == 0 && origin != "" {
			// Development: no whitelist configured
			ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
		}

		ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Organization-ID, X-CSRF-Token")
		ctx.Response.Header.Set("Access-Control-Max-Age", "86400")

		// Handle preflight OPTIONS requests
		if string(ctx.Method()) == "OPTIONS" {
			ctx.SetStatusCode(fasthttp.StatusNoContent)
			return
		}

		next(ctx)
	}
}
