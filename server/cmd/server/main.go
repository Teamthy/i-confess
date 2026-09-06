package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/config"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/email"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/oauth"
	"github.com/Teamthy/i-confess/internal/push"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/seed"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/voice"
)

func main() {
	cfg := config.Load()

	// Refuse to start production with development secrets.
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer conn.Close()

	// Object storage. The provider comes from configuration, not from the code:
	// hard-coding it here meant production ran on a local filesystem while the
	// cloud providers sat unimplemented, and nothing on that path complained.
	//
	// Development uses a filesystem provider that enforces the same signature
	// and expiry rules as the production CDN, so signed delivery is exercised
	// in every environment (PRD S11). config.Validate has already refused
	// STORAGE_PROVIDER=local outside development.
	objStore, err := storage.New(&storage.StorageConfig{
		Provider:      cfg.StorageProvider,
		LocalRootPath: cfg.MediaDir,
		S3Bucket:      cfg.S3Bucket,
		S3Region:      cfg.S3Region,
		S3AccessKey:   cfg.S3AccessKey,
		S3SecretKey:   cfg.S3SecretKey,
		S3Endpoint:    cfg.S3Endpoint,
		CDNDomain:     cfg.MediaBaseURL,
		SigningSecret: cfg.AudioSignSecret,
	})
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	// Seed demo content (development only). This is what creates placeholder
	// audio and the demo accounts, and neither of those belongs in production.
	if cfg.Env == "development" || os.Getenv("SEED") == "1" {
		if err := seed.Seed(conn, objStore); err != nil {
			log.Printf("seed: %v", err)
		}
	}

	// Ensure the canonical content library exists in EVERY environment.
	//
	// This used to be covered by Seed alone, which does not run in production,
	// so a deployed server came up with an empty catalogue. Categories and
	// confessions are the product's inventory, not demo data.
	//
	// It is idempotent: after Seed has populated a dev database this is a
	// no-op. It creates no audio - audio comes from the generation pipeline
	// behind the rights gate, never from a bootstrap.
	//
	// Failure is logged loudly but not fatal: auth, profile and admin still
	// work with an empty catalogue, and a crash loop is the worse outcome. It
	// does mean a boot can succeed with no content, which is why the log line
	// is an error rather than an info.
	if _, _, err := seed.EnsureContent(context.Background(), conn); err != nil {
		log.Printf("content: FAILED to ensure the canonical library: %v", err)
	}

	h := api.NewHandler(api.Config{JWTSecret: cfg.JWTSecret, TokenTTL: cfg.TokenTTL}, conn)
	h.BuildEngine()
	h.SetSigner(objStore)

	// The local origin stands in for the CDN. Production leaves this unset and
	// serves audio from the edge.
	if !cfg.IsProduction() {
		if local, ok := objStore.(*storage.LocalStorage); ok {
			h.SetMediaHandler(local.Handler("/media"))
		}
	}

	// Social sign-in. A provider is enabled only when its client id is set:
	// an empty audience would accept tokens minted for any other application,
	// so absent configuration must disable the provider rather than open it.
	verifiers := map[string]oauth.Verifier{}
	if cfg.GoogleClientID != "" {
		verifiers[oauth.ProviderGoogle] = oauth.NewGoogle(cfg.GoogleClientID)
		log.Printf("auth: google sign-in enabled")
	}
	if cfg.AppleClientID != "" {
		verifiers[oauth.ProviderApple] = oauth.NewApple(cfg.AppleClientID)
		log.Printf("auth: apple sign-in enabled")
	}
	if len(verifiers) == 0 {
		log.Printf("auth: no social providers configured")
	}
	h.SetVerifiers(verifiers)

	// Transactional email. Delivery is asynchronous so a slow or failing
	// provider can never turn a registration into a 500 (PRD S57, S58).
	var sender email.Sender = email.LogSender{}
	if cfg.EmailProvider == "postmark" {
		sender = email.NewPostmark(cfg.PostmarkToken, cfg.EmailFrom)
		log.Printf("email: postmark sender enabled")
	} else {
		log.Printf("email: using log sender - messages are printed, not delivered")
	}
	mailQueue := email.NewQueue(sender, 512)
	mailQueue.Start(context.Background(), 2)
	defer mailQueue.Stop()

	h.SetMailer(mailQueue, email.Config{
		AppName:        cfg.AppName,
		BaseURL:        cfg.PublicBaseURL,
		FromAddress:    cfg.EmailFrom,
		SupportAddress: cfg.EmailSupport,
	})

	// Rate limiting. Without a shared store each replica enforces its own
	// budget, so N replicas allow N times the intended rate (PRD S20).
	if cfg.RedisAddr != "" {
		rdb := redisStore(cfg)
		defer rdb.Close()
		h.SetLimiter(ratelimit.NewDistributed(rdb))
		if err := rdb.Ping(); err != nil {
			// Not fatal: the limiter degrades to per-instance rather than
			// refusing all logins because Redis is briefly unavailable.
			log.Printf("redis: unreachable at startup, rate limiting is degraded: %v", err)
		} else {
			log.Printf("redis: shared rate limiting enabled at %s", cfg.RedisAddr)
		}
	} else {
		log.Printf("redis: REDIS_ADDR unset - rate limits are per-instance only")
	}

	if cfg.ElevenLabsAPIKey != "" {
		h.SetPipeline(voice.NewPipeline(voice.NewElevenLabs(cfg.ElevenLabsAPIKey), objStore))
		log.Printf("voice: elevenlabs synthesis enabled")
	} else {
		log.Printf("voice: ELEVENLABS_API_KEY unset - synthesis endpoints report 503")
	}

	// Start background job workers.
	queue := h.GetQueue()
	jobs.RegisterHandlers(queue)
	worker := jobs.NewWorker(queue, cfg.QueueWorkers)
	worker.Start(context.Background())
	defer worker.Stop()

	// Push notifications. Without a configured provider, scheduled reminders
	// are logged rather than delivered - the schedule still fires, so the
	// behaviour is visible in development (PRD S47).
	router := &push.Router{}
	if cfg.APNsKeyPath != "" && cfg.APNsKeyID != "" && cfg.APNsTeamID != "" {
		if pem, rerr := os.ReadFile(cfg.APNsKeyPath); rerr != nil {
			log.Printf("push: cannot read APNs key: %v", rerr)
		} else if apns, aerr := push.NewAPNs(string(pem), cfg.APNsTeamID, cfg.APNsKeyID,
			cfg.APNsTopic, cfg.APNsProduction); aerr != nil {
			log.Printf("push: APNs disabled: %v", aerr)
		} else {
			router.APNs = apns
			log.Printf("push: APNs enabled (topic=%s production=%t)", cfg.APNsTopic, cfg.APNsProduction)
		}
	}
	if cfg.FCMServiceAccountPath != "" {
		if key, kerr := os.ReadFile(cfg.FCMServiceAccountPath); kerr != nil {
			log.Printf("push: cannot read FCM service account: %v", kerr)
		} else if fcm, ferr := push.NewFCMFromServiceAccount(key); ferr != nil {
			log.Printf("push: FCM disabled: %v", ferr)
		} else {
			router.FCM = fcm
			log.Printf("push: FCM enabled (project=%s)", fcm.ProjectID)
		}
	}

	if router.Configured() {
		h.SetPushSender(router)
	} else {
		log.Printf("push: no provider configured - scheduled reminders will be logged, not delivered")
		h.SetPushSender(push.LogSender{})
	}

	// Fire scheduled session reminders. A one-minute tick keeps delivery
	// within a minute of the user's chosen time; the occurrence key makes a
	// double tick harmless.
	schedCtx, stopSched := context.WithCancel(context.Background())
	defer stopSched()
	go func() {
		t := time.NewTicker(1 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-schedCtx.Done():
				return
			case <-t.C:
				rep, err := h.RunScheduleSweep(schedCtx)
				if err != nil {
					log.Printf("scheduler: sweep failed: %v", err)
				} else if rep.Sent > 0 || rep.Failed > 0 {
					log.Printf("scheduler: sent=%d failed=%d skipped=%d of %d schedules",
						rep.Sent, rep.Failed, rep.Skipped, rep.Evaluated)
				}
			}
		}
	}()

	// Erase accounts whose grace period has elapsed (PRD S50). Running it on a
	// timer inside the API process is right for one instance; at multiple
	// replicas this needs a lock so two sweepers do not race, which the
	// per-account transaction already makes safe but wasteful.
	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go func() {
		t := time.NewTicker(1 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-sweepCtx.Done():
				return
			case <-t.C:
				if n, err := h.RunDeletionSweep(sweepCtx); err != nil {
					log.Printf("deletion: sweep failed: %v", err)
				} else if n > 0 {
					log.Printf("deletion: erased %d expired accounts", n)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown handling.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("shutdown signal received, gracefully stopping...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
		worker.Stop()
	}()

	log.Printf("i-confess backend listening on :%s (env=%s, workers=%d)", cfg.Port, cfg.Env, cfg.QueueWorkers)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// redisStore builds the shared counter backend for rate limiting.
func redisStore(cfg config.Config) *ratelimit.RedisStore {
	return ratelimit.NewRedisStore(cfg.RedisAddr, cfg.RedisPassword)
}
