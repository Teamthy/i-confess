package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/cache"
	"github.com/Teamthy/i-confess/internal/config"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/email"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/oauth"
	"github.com/Teamthy/i-confess/internal/push"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/seed"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voice"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
	"github.com/Teamthy/i-confess/internal/workers"
)

func main() {
	cfg := config.Load()

	// Refuse to start production with development secrets.
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}

	// Store verification (IC-003). Which verifier is installed decides whether a
	// paying customer gets what they paid for, and a stub that reaches
	// production gives premium away - so the process says out loud which one it
	// built, and refuses to start outside development and test without a real
	// one. Both lines exist because "billing is quietly disabled" is not
	// something a running server should be able to hide.
	log.Printf("billing: verifier %s", billing.DescribeVerifier(billing.VerifierFromEnv()))
	if err := billing.RequireVerification(); err != nil {
		log.Fatalf("billing: %v", err)
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
		Provider:                 cfg.StorageProvider,
		LocalRootPath:            cfg.MediaDir,
		S3Bucket:                 cfg.S3Bucket,
		S3Region:                 cfg.S3Region,
		S3AccessKey:              cfg.S3AccessKey,
		S3SecretKey:              cfg.S3SecretKey,
		S3Endpoint:               cfg.S3Endpoint,
		CDNDomain:                cfg.MediaBaseURL,
		CDNProvider:              "cloudfront",
		CloudFrontKeyPairID:      cfg.CloudFrontKeyPairID,
		CloudFrontPrivateKeyPath: cfg.CloudFrontPrivateKeyPath,
		SigningSecret:            cfg.AudioSignSecret,
	})
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	// Seed demo content (development and test only). This is what creates placeholder
	// audio and the demo accounts, and neither of those belongs in production.
	if (cfg.Env == "development" || cfg.Env == "test") && (cfg.Env == "development" || os.Getenv("SEED") == "1") {
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
	// no-op for the text rows. A content bootstrap failure is fatal: serving an
	// empty or partial catalogue is worse than refusing readiness, and makes a
	// launch appear healthy while every content request fails.
	if _, _, err := seed.EnsureContent(context.Background(), conn); err != nil {
		log.Fatalf("content: failed to ensure the canonical library: %v", err)
	}
	if reviewed, err := seed.EnsureCanonicalTheology(context.Background(), conn); err != nil {
		log.Fatalf("content: failed to record canonical theological review: %v", err)
	} else if reviewed > 0 {
		log.Printf("content: recorded %d canonical theological reviews", reviewed)
	}
	if created, err := seed.EnsureCanonicalAudio(context.Background(), conn, objStore); err != nil {
		log.Fatalf("audio: failed to ensure canonical audio: %v", err)
	} else if created > 0 {
		log.Printf("audio: ensured %d canonical bootstrap assets", created)
	}

	h := api.NewHandler(api.Config{JWTSecret: cfg.JWTSecret, TokenTTL: cfg.TokenTTL}, conn)
	localBible := &bible.LocalBibleProvider{DB: conn}
	helloAO, providerErr := bible.NewHelloAOBibleProvider(os.Getenv("HELLOAO_BASE_URL"), nil)
	if providerErr != nil {
		log.Fatalf("bible provider configuration: %v", providerErr)
	}
	h.SetBibleDiscoveryProvider(helloAO)
	// Provider choice is server-side. The local corpus is always the fallback;
	// HelloAO content is exposed only after its database registry row is reviewed.
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BIBLE_PROVIDER"))) {
	case "helloao":
		h.SetBibleProvider(&bible.ReviewedProvider{Local: localBible, Remote: helloAO})
		log.Printf("bible: HelloAO adapter enabled with approved local fallback")
	case "", "local":
		h.SetBibleProvider(localBible)
		log.Printf("bible: local verified corpus provider enabled")
	default:
		log.Fatalf("bible provider configuration: unsupported BIBLE_PROVIDER %q (choose local or helloao)", os.Getenv("BIBLE_PROVIDER"))
	}
	h.SetProduction(cfg.IsProduction())
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

	// Cache invalidation between instances (G-10).
	//
	// The content caches are per-process, so without a bus an admin edit
	// published on one replica stays invisible on the others until ttl+swr
	// expires - up to fifteen minutes on the category and voice caches.
	//
	// This reuses the same Redis the limiter does: pub/sub needs one more
	// connection from a server the process is already configured to talk to,
	// rather than a new dependency. Without REDIS_ADDR the Handler still
	// invalidates its own caches on write, which is what a single-instance
	// deployment needs, and the log says so rather than implying otherwise.
	if cfg.RedisAddr != "" {
		bus := redisCacheBus(cfg)
		if err := h.SetCacheBus(bus); err != nil {
			// Not fatal: publishing still works, so this instance keeps the
			// other replicas fresh while it serves its own cache until TTL.
			log.Printf("redis: cache invalidation subscription failed, this instance will serve stale content until TTL: %v", err)
		} else {
			log.Printf("redis: cross-instance cache invalidation enabled at %s (channel %s)", cfg.RedisAddr, bus.Channel)
		}
		defer h.CloseCacheBus()
	} else {
		log.Printf("redis: REDIS_ADDR unset - cache invalidation is local to this instance only")
	}

	if cfg.ElevenLabsAPIKey != "" {
		h.SetPipeline(voice.NewPipeline(voice.NewElevenLabs(cfg.ElevenLabsAPIKey), objStore))
		log.Printf("voice: elevenlabs synthesis enabled")
	} else {
		log.Printf("voice: ELEVENLABS_API_KEY unset - synthesis endpoints report 503")
	}

	// Audio Platform Phase 1: configure signed playback URL resolution.

	// Create URL generator for signed URLs
	urlGenerator, err := audio.NewURLGenerator(objStore, &audio.URLGeneratorConfig{
		CDNDomain:     cfg.MediaBaseURL,
		StreamTTL:     4 * time.Hour,
		DownloadTTL:   24 * time.Hour,
		SigningSecret: cfg.AudioSignSecret,
	})
	if err != nil {
		log.Fatalf("audio: failed to create URL generator: %v", err)
	}
	h.SetURLGenerator(urlGenerator)
	log.Printf("audio: URL generator enabled (CDN: %s)", cfg.MediaBaseURL)

	audioStore := store.NewAudioStore(conn)
	playbackResolver := audio.NewPlaybackResolver(
		objStore,
		audioStore,
		audioStore,
		&audio.PlaybackResolverConfig{
			CDNDomain:       cfg.MediaBaseURL,
			StreamTTL:       4 * time.Hour,
			DownloadTTL:     24 * time.Hour,
			AllowAllPremium: !cfg.IsProduction(),
		},
	)
	h.SetPlaybackResolver(playbackResolver)
	log.Printf("audio: playback resolver enabled")

	// Background job queue.
	//
	// The durable PostgreSQL implementation rather than the in-process default:
	// work still pending when the process stops survives, and a stop is a deploy
	// rather than an accident. Claiming is a single statement with FOR UPDATE
	// SKIP LOCKED, so several workers over one table never share a job.
	h.SetQueue(store.NewJobQueue(conn))
	queue := h.GetQueue()

	var voiceJobs []string
	// Licensed minister voice platform. GPU workers (voice-engine/) are
	// separate processes; the API only ever talks HTTP to them.
	if orch := buildVoiceOrchestrator(cfg.IsProduction()); orch != nil {
		h.SetVoiceOrchestrator(orch)
		h.SetVoiceThresholds(voiceeval.Thresholds{
			MinScore:      envFloat("VOICE_GATE_MIN_SCORE", 0),
			MinCoverage:   envFloat("VOICE_GATE_MIN_COVERAGE", 0.8),
			MaxRegression: envFloat("VOICE_GATE_MAX_REGRESSION", 0.03),
		})
		h.RegisterVoiceJobs()
		voiceJobs = append(voiceJobs, api.JobVoiceGenerate)
	}
	// Expired grants are flipped to EXPIRED and "delete" post-termination
	// policies applied on a timer. Serving already re-checks rights on every
	// request; the sweep keeps stored status, audit and storage in line (§68).
	h.StartVoiceRightsSweeper(context.Background())
	// Recording intake and fine-tuning run on a (possibly separate) worker
	// pool. Independent of synthesis so intake can start before any model.
	if url := os.Getenv("VOICE_WORKER_URL"); url != "" {
		if cfg.IsProduction() && os.Getenv("VOICE_ENGINE_TOKEN") == "" {
			log.Fatalf("voice: VOICE_ENGINE_TOKEN is required in production")
		}
		h.SetVoiceWorker(voiceengine.NewWorkerClient(url, os.Getenv("VOICE_ENGINE_TOKEN"), 0), api.VoiceIntakeConfig{
			PollInterval: time.Duration(envFloat("VOICE_TRAIN_POLL_SECONDS", 60)) * time.Second,
		})
		h.RegisterVoiceIntakeJobs()
		log.Printf("voice: intake/training worker at %s", url)
	}

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

	var pushSender push.Sender = push.LogSender{}
	if router.Configured() {
		pushSender = router
		h.SetPushSender(router)
	} else {
		log.Printf("push: no provider configured - scheduled reminders will be logged, not delivered")
		h.SetPushSender(push.LogSender{})
	}

	// Reminders go through the durable queue now that there is one, so a
	// provider that is slow or briefly unreachable costs a retry rather than
	// the delivery, and the sweep stays as short as the work it does.
	if h.SetPushQueue() {
		log.Printf("push: scheduled reminders are queued through the durable job queue")
	}

	// Register the job handlers and start the worker pool.
	//
	// Only types with a real collaborator are registered; anything else is left
	// uninstalled so the queue parks such a job with "no handler registered"
	// rather than accepting work it cannot do. Generation runs through the same
	// pipeline the synchronous admin endpoint uses, so the queued path is not a
	// cheaper version of the real thing.
	installed := workers.Register(queue, workers.Services{
		Generate: h,
		Notify:   pushSender,
		Devices:  h,
	})
	worker := jobs.NewWorker(queue, cfg.QueueWorkers)
	worker.Start(context.Background())
	defer worker.Stop()
	log.Printf("queue: %d workers, handlers %v", cfg.QueueWorkers, append(installed, voiceJobs...))

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

	// Expire due trials even when their owners never return to the API. The
	// store locks each trial row and makes the transition/event idempotent, so
	// replicas may overlap safely. Run once at startup to recover missed ticks,
	// then every 15 minutes to keep the churn event close to the expiry time.
	trialSweepCtx, stopTrialSweep := context.WithCancel(context.Background())
	defer stopTrialSweep()
	go func() {
		run := func() {
			n, err := h.RunTrialExpirySweep(trialSweepCtx)
			if err != nil {
				log.Printf("trials: expiry sweep failed after %d transitions: %v", n, err)
			} else if n > 0 {
				log.Printf("trials: expired %d due trials", n)
			}
		}
		run()
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-trialSweepCtx.Done():
				return
			case <-t.C:
				run()
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

	// Periodically purge expired idempotency records and cleanup old data (IC-023).
	idemCtx, stopIdem := context.WithCancel(context.Background())
	defer stopIdem()
	go func() {
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-idemCtx.Done():
				return
			case <-t.C:
				if n, err := h.RunIdempotencySweep(idemCtx); err != nil {
					log.Printf("idempotency: sweep failed: %v", err)
				} else if n > 0 {
					log.Printf("idempotency: purged %d expired keys", n)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
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

// redisCacheBus returns the invalidation transport used across API instances.
func redisCacheBus(cfg config.Config) *cache.RedisBus {
	return cache.NewRedisBus(cfg.RedisAddr, cfg.RedisPassword)
}

// buildVoiceOrchestrator registers one provider per configured worker pool.
// It returns nil when none are configured, leaving generation endpoints at 503.
func buildVoiceOrchestrator(production bool) *voiceengine.Orchestrator {
	token := os.Getenv("VOICE_ENGINE_TOKEN")
	reg := voiceengine.NewRegistry(production)
	n := 0
	for _, e := range []struct {
		env    string
		engine voiceengine.Engine
		mk     func(string, string) *voiceengine.HTTPProvider
	}{
		{"VOICE_ENGINE_COSYVOICE_URL", voiceengine.EngineCosyVoice, voiceengine.NewCosyVoiceProvider},
		{"VOICE_ENGINE_GPTSOVITS_URL", voiceengine.EngineGPTSoVITS, voiceengine.NewGPTSoVITSProvider},
		{"VOICE_ENGINE_VOXCPM_URL", voiceengine.EngineVoxCPM, voiceengine.NewVoxCPMProvider},
		{"VOICE_ENGINE_VOICESTUDIO_URL", voiceengine.EngineVoiceStudio, voiceengine.NewVoiceStudioProvider},
	} {
		url := os.Getenv(e.env)
		if url == "" {
			continue
		}
		if err := reg.Register(e.engine, e.mk(url, token)); err != nil {
			log.Printf("voice: %s ignored: %v", e.env, err)
			continue
		}
		n++
	}
	if n == 0 {
		log.Printf("voice: no VOICE_ENGINE_*_URL set - minister voice generation reports 503")
		return nil
	}
	if production && token == "" {
		log.Fatalf("voice: VOICE_ENGINE_TOKEN is required in production")
	}
	log.Printf("voice: %d voice-engine worker pool(s) registered", n)
	return &voiceengine.Orchestrator{
		Registry: reg,
		Fallback: voiceengine.FallbackPolicy{
			Enabled:      os.Getenv("VOICE_FALLBACK_ENABLED") == "1",
			MinScore:     envFloat("VOICE_FALLBACK_MIN_SCORE", 0),
			MaxScoreDrop: envFloat("VOICE_FALLBACK_MAX_DROP", 0.03),
		},
	}
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}
