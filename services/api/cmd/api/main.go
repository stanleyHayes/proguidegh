// Command api runs the ProGuideGH HTTP API server. Phase 0 shipped health,
// readiness and an OpenAPI skeleton; Phase 1 added identity, RBAC and
// profiles; Phase 2 the certification pipeline and catalog; Phase 3 adds
// search, availability and bookings (spec §8.2, §10, §13.2–13.4); Phase 4
// payments, ledger and receipts; Phase 5 dispatch, realtime and tour
// operations (spec §10.3, §11, §13.4–13.5).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"proguidegh/api/internal/admin"
	"proguidegh/api/internal/auth"
	"proguidegh/api/internal/availability"
	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/catalog"
	"proguidegh/api/internal/certification"
	"proguidegh/api/internal/dispatch"
	"proguidegh/api/internal/guides"
	"proguidegh/api/internal/incidents"
	"proguidegh/api/internal/ledger"
	"proguidegh/api/internal/payments"
	"proguidegh/api/internal/payouts"
	"proguidegh/api/internal/platform/audit"
	pauth "proguidegh/api/internal/platform/auth"
	"proguidegh/api/internal/platform/config"
	"proguidegh/api/internal/platform/db"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/observability"
	"proguidegh/api/internal/platform/ratelimit"
	"proguidegh/api/internal/platform/rbac"
	"proguidegh/api/internal/platform/redis"
	"proguidegh/api/internal/platform/storage"
	"proguidegh/api/internal/privacy"
	"proguidegh/api/internal/realtime"
	"proguidegh/api/internal/receipts"
	"proguidegh/api/internal/reporting"
	"proguidegh/api/internal/reviews"
	"proguidegh/api/internal/safety"
	"proguidegh/api/internal/tourists"
	"proguidegh/api/internal/tours"
	"proguidegh/api/internal/tracking"
	"proguidegh/api/internal/training"
)

func main() {
	dumpOpenAPI := flag.Bool("dump-openapi", false, "print the OpenAPI 3.1 document to stdout and exit")
	flag.Parse()

	if *dumpOpenAPI {
		fmt.Print(openAPIDocument)
		return
	}

	cfg := config.Load()
	log := observability.NewLogger(cfg.AppEnv)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := redis.Connect(ctx, cfg.RedisURL)
	if err != nil {
		log.Error("connect redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close() //nolint:errcheck

	app, err := buildHandler(cfg, pool, rdb, log)
	if err != nil {
		log.Error("build http handler", "error", err)
		os.Exit(1)
	}
	defer app.hub.Close()

	// Offer sweeper: lazy expiry on reads is the primary mechanism; this
	// ticker is the background guarantee that terminal EXPIRED states and
	// guide/ops notifications land even when nobody reads (spec §10.3). The
	// worker can take over by calling the same ExpireOffers entry point.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := app.sweep(ctx)
				if err != nil && ctx.Err() == nil {
					log.Error("offer sweeper", "error", err)
				} else if n > 0 {
					log.Info("dispatch offers expired", "count", n)
				}
			}
		}
	}()

	// Weekly payout batch (spec §8.4, P7-03): the ticker wakes hourly; the
	// closure itself decides whether a batch is due (Monday, none scheduled
	// yet today) and runs it. Per-(guide, date) uniqueness makes any rerun
	// idempotent (P7-07).
	go func() {
		run := func() {
			created, err := app.payoutBatch(ctx)
			if err != nil && ctx.Err() == nil {
				log.Error("payout batch scheduler", "error", err)
			} else if created > 0 {
				log.Info("weekly payout batch queued", "created", created)
			}
		}
		run() // catch up after a weekend deploy/restart
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           app.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Info("api listening", "port", cfg.APIPort, "env", cfg.AppEnv)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("http server", "error", err)
		os.Exit(1)
	}
}

// apiApp bundles the HTTP handler with the runtime services main must
// supervise: the realtime hub (graceful shutdown), one dispatch expiry
// sweep pass (ticker) and one weekly payout-batch run (scheduler).
type apiApp struct {
	handler     http.Handler
	hub         *realtime.Hub
	sweep       func(ctx context.Context) (int, error)
	payoutBatch func(ctx context.Context) (int, error)
}

// buildHandler wires the full route tree: platform middleware → authn/authz
// → domain handlers. Kept separate from main so integration tests can build
// the same stack over httptest.
func buildHandler(cfg config.Config, pool *pgxpool.Pool, rdb *goredis.Client, log *slog.Logger) (*apiApp, error) {
	if cfg.JWTOrSessionSecret == "" {
		return nil, errors.New("JWT_OR_SESSION_SECRET must be set")
	}

	// Platform services.
	issuer, err := pauth.NewTokenIssuer(cfg.JWTOrSessionSecret)
	if err != nil {
		return nil, err
	}
	rbacStore := rbac.NewStore(pool, rdb)
	rbacMw := rbac.NewMiddleware(issuer, rbacStore)
	auditor := audit.NewRecorder(pool)
	limiter := ratelimit.NewLimiter(rdb)

	var objStore storage.Store
	var fileHandler http.Handler
	switch cfg.StorageProvider {
	case "local":
		localStore, err := storage.NewLocal(cfg.UploadsDir, cfg.JWTOrSessionSecret)
		if err != nil {
			return nil, err
		}
		objStore = localStore
		fileHandler = localStore.Handler()
	case "r2":
		objStore, err = storage.NewR2(storage.R2Config{
			Endpoint:        cfg.R2Endpoint,
			AccessKeyID:     cfg.R2AccessKeyID,
			SecretAccessKey: cfg.R2SecretAccessKey,
			BucketPrivate:   cfg.R2BucketPrivate,
		})
		if err != nil {
			return nil, err
		}
		// R2 serves files via provider-presigned URLs; no local file handler.
	default:
		return nil, fmt.Errorf("unknown STORAGE_PROVIDER %q", cfg.StorageProvider)
	}

	// Domain slices.
	secureCookies := !cfg.IsLocal()

	authSvc := auth.NewService(auth.NewRepository(pool), issuer, rbacStore, auditor, rdb, cfg.AppEnv, cfg.JWTOrSessionSecret)
	authH := auth.NewHandler(authSvc, secureCookies)

	touristsH := tourists.NewHandler(tourists.NewRepository(pool))
	privacyH := privacy.NewHandler(privacy.NewRepository(pool), auditor, objStore)

	certRepo := certification.NewRepository(pool)
	certSvc := certification.NewService(certRepo)
	certH := certification.NewHandler(certRepo, certSvc,
		func(userID string) { rbacStore.Invalidate(context.Background(), userID) })

	guidesRepo := guides.NewRepository(pool)
	catalogRepo := catalog.NewRepository(pool)
	availRepo := availability.NewRepository(pool)
	presence := availability.NewPresence(rdb)

	guidesH := guides.NewHandler(guidesRepo, guides.NewSearchRepository(pool), availRepo, presence,
		catalogRepo, objStore, certSvc,
		func(userID string) { rbacStore.Invalidate(context.Background(), userID) })

	catalogH := catalog.NewHandler(catalogRepo)

	bookingsRepo := bookings.NewRepository(pool)
	bookingsH := bookings.NewHandler(bookings.NewService(bookingsRepo,
		catalogRepo, guidesRepo, certSvc, availRepo))

	// Phase 4: payments, ledger & receipts (spec §4.5, §8.3, §9, §17).
	ledgerSvc := ledger.NewService(pool)
	receiptsSvc := receipts.NewService(receipts.NewRepository(pool), objStore)
	paymentsSvc := payments.NewService(pool, payments.NewRepository(pool),
		bookingsRepo, ledgerSvc, receiptsSvc, payments.NewProvider(cfg))
	paymentsH := payments.NewHandler(paymentsSvc, bookingsRepo, auditor)
	receiptsH := receipts.NewHandler(receiptsSvc, bookingsRepo)

	adminH := admin.NewHandler(admin.NewRepository(pool), auditor,
		func(userID string) { rbacStore.Invalidate(context.Background(), userID) })

	// Phase 5: dispatch, realtime & tour operations (spec §10.3, §11,
	// §13.4–13.5). The hub is pure fan-out; Postgres stays the source of
	// truth and every pushed message has a REST catch-up path (§31.27).
	hub := realtime.NewHub()
	rtServer := realtime.NewServer(hub, issuer, rbacStore)

	dispatchSvc := dispatch.NewService(pool, dispatch.NewRepository(pool),
		bookingsRepo, presence, rdb, hub)
	dispatchH := dispatch.NewHandler(dispatchSvc, auditor)

	trackingSvc := tracking.NewService(pool, bookingsRepo, rdb, hub)
	trackingH := tracking.NewHandler(trackingSvc, bookingsRepo)

	toursSvc := tours.NewService(pool, bookingsRepo, ledgerSvc, trackingSvc, hub)
	toursH := tours.NewHandler(toursSvc, auditor)

	// Phase 6: safety, reviews & quality (spec §4.4, §12, §13.5–13.6).
	safetyRepo := safety.NewRepository(pool)
	safetySvc := safety.NewService(safetyRepo, bookingsRepo, hub, limiter, auditor)
	safetyH := safety.NewHandler(safetySvc)
	reviewsH := reviews.NewHandler(reviews.NewService(reviews.NewRepository(pool), bookingsRepo))
	incidentsH := incidents.NewHandler(incidents.NewService(
		incidents.NewRepository(pool), safetyRepo, hub, auditor))

	// Phase 7: wallet, payouts & finance (spec §8, P7-01…P7-07). The account
	// key encrypts payout destination refs at rest; when PAYOUT_ACCOUNT_KEY
	// is unset it derives from the session secret (local dev).
	payoutsSvc := payouts.NewService(payouts.NewRepository(pool), pool, ledgerSvc, auditor,
		payouts.Key(cfg.PayoutAccountKey, cfg.JWTOrSessionSecret))
	payoutsH := payouts.NewHandler(payoutsSvc)

	// Phase 8: light LMS, executive reporting and the audit viewer (spec
	// §4.3, §13.6, P8-01…P8-04).
	trainingH := training.NewHandler(training.NewService(training.NewRepository(pool), auditor))
	reportingH := reporting.NewHandler(reporting.NewRepository(pool), auditor)

	// Marketplace bookings (no guide chosen at creation) enter dispatch when
	// payment confirms them; direct bookings skip it (guide already set).
	paymentsSvc.OnConfirmed = func(ctx context.Context, bookingID string) {
		b, err := bookingsRepo.GetByID(ctx, bookingID)
		if err != nil || b.GuideID != nil {
			return
		}
		if _, err := dispatchSvc.Dispatch(ctx, bookingID, "payment.confirmed"); err != nil {
			slog.Error("dispatch after payment confirmation", "booking_id", bookingID, "error", err)
		}
	}

	// WS authorization/snapshot hooks (realtime stays persistence-free).
	rtServer.BookingVisible = func(ctx context.Context, bookingID string, id rbac.Identity) bool {
		b, err := bookingsRepo.GetByID(ctx, bookingID)
		if err != nil {
			return false
		}
		return b.TouristID == id.UserID ||
			(b.GuideID != nil && *b.GuideID == id.UserID) ||
			id.Has("bookings.read")
	}
	rtServer.GuideSnapshot = dispatchSvc.SnapshotMessages
	rtServer.BookingSnapshot = func(ctx context.Context, bookingID string) []realtime.Message {
		b, err := bookingsRepo.GetByID(ctx, bookingID)
		if err != nil {
			return nil
		}
		msgs := []realtime.Message{realtime.NewMessage("booking.updated", b)}
		if p, err := trackingSvc.CurrentForBooking(ctx, bookingID); err == nil {
			msgs = append(msgs, realtime.NewMessage("location.update", p))
		}
		return msgs
	}

	mux := http.NewServeMux()

	// Probes (Phase 0).
	mux.HandleFunc("GET /healthz", healthzHandler())
	mux.HandleFunc("GET /readyz", readyzHandler(pool, rdb))

	// Auth (spec §13.1). Login/OTP/reset are rate-limited (spec §15.2).
	mux.Handle("POST /api/v1/auth/register", http.HandlerFunc(authH.Register))
	mux.Handle("POST /api/v1/auth/login", ratelimit.Middleware(limiter, auth.LoginLimit)(http.HandlerFunc(authH.Login)))
	mux.Handle("POST /api/v1/auth/login/mfa", ratelimit.Middleware(limiter, auth.LoginLimit)(http.HandlerFunc(authH.LoginMFA)))
	mux.Handle("POST /api/v1/auth/otp/request", ratelimit.Middleware(limiter, auth.OTPLimit)(http.HandlerFunc(authH.RequestOTP)))
	mux.Handle("POST /api/v1/auth/otp/verify", ratelimit.Middleware(limiter, auth.OTPLimit)(http.HandlerFunc(authH.VerifyOTP)))
	mux.Handle("POST /api/v1/auth/refresh", http.HandlerFunc(authH.Refresh))
	mux.Handle("POST /api/v1/auth/logout", http.HandlerFunc(authH.Logout))
	mux.Handle("POST /api/v1/auth/password/forgot", ratelimit.Middleware(limiter, auth.ResetLimit)(http.HandlerFunc(authH.ForgotPassword)))
	mux.Handle("POST /api/v1/auth/password/reset", ratelimit.Middleware(limiter, auth.ResetLimit)(http.HandlerFunc(authH.ResetPassword)))

	// Self-service (auth required).
	mux.Handle("GET /api/v1/me/tourist-profile", rbacMw.RequireAuth(http.HandlerFunc(touristsH.Get)))
	mux.Handle("PATCH /api/v1/me/tourist-profile", rbacMw.RequireAuth(http.HandlerFunc(touristsH.Patch)))
	mux.Handle("POST /api/v1/me/mfa/enroll", rbacMw.RequireAuth(http.HandlerFunc(authH.EnrollMFA)))
	mux.Handle("POST /api/v1/me/mfa/verify", rbacMw.RequireAuth(http.HandlerFunc(authH.VerifyMFA)))

	// Data-subject rights (Apple 5.1.1(v), Google Play data deletion,
	// Ghana Data Protection Act 2012 s.32/s.33). All self-scoped.
	// Policies are public: the sign-up screen must show them before an
	// account exists, and both stores require a reachable privacy policy.
	mux.Handle("GET /api/v1/legal/policies", http.HandlerFunc(privacyH.Policies))
	// Public marketing content for the website. Reads one allow-listed
	// system_settings key; admin-web writes it through the audited
	// PUT /admin/settings/{key} path.
	mux.Handle("GET /api/v1/content/marketing", http.HandlerFunc(privacyH.MarketingContentHandler))
	mux.Handle("POST /api/v1/me/consent", rbacMw.RequireAuth(http.HandlerFunc(privacyH.Consent)))
	mux.Handle("GET /api/v1/me/export", rbacMw.RequireAuth(http.HandlerFunc(privacyH.Export)))
	mux.Handle("GET /api/v1/me/deletion", rbacMw.RequireAuth(http.HandlerFunc(privacyH.DeletionPreview)))
	mux.Handle("DELETE /api/v1/me", rbacMw.RequireAuth(http.HandlerFunc(privacyH.Delete)))

	// Guides (spec §13.4).
	mux.Handle("POST /api/v1/guides/apply", rbacMw.RequireAuth(http.HandlerFunc(guidesH.Apply)))
	mux.Handle("POST /api/v1/guides/documents", rbacMw.RequireAuth(http.HandlerFunc(guidesH.RegisterDocument)))
	mux.Handle("GET /api/v1/me/guide", rbacMw.RequireAuth(http.HandlerFunc(guidesH.Me)))
	mux.Handle("GET /api/v1/me/guide/certification", rbacMw.RequireAuth(http.HandlerFunc(guidesH.MeCertification)))
	mux.Handle("PATCH /api/v1/me/guide/profile", rbacMw.RequireAuth(http.HandlerFunc(guidesH.PatchProfile)))
	mux.HandleFunc("GET /api/v1/guides/{id}", guidesH.PublicDetail) // public, §10.2-gated
	mux.HandleFunc("GET /api/v1/guides/search", guidesH.Search)     // public, §10.1 filters over §10.2-eligible guides

	// Guide availability (spec §10.1, §13.4) — auth required, own record.
	mux.Handle("POST /api/v1/me/guide/availability", rbacMw.RequireAuth(http.HandlerFunc(guidesH.SetAvailability)))
	mux.Handle("PUT /api/v1/me/guide/availability/schedule", rbacMw.RequireAuth(http.HandlerFunc(guidesH.PutSchedule)))
	mux.Handle("POST /api/v1/me/guide/availability/time-off", rbacMw.RequireAuth(http.HandlerFunc(guidesH.AddTimeOff)))
	mux.Handle("DELETE /api/v1/me/guide/availability/time-off/{id}", rbacMw.RequireAuth(http.HandlerFunc(guidesH.DeleteTimeOff)))

	// Bookings (spec §13.3). Quote is public and server-authoritative (§14);
	// creation requires auth + Idempotency-Key.
	mux.HandleFunc("POST /api/v1/bookings/quote", bookingsH.Quote)
	mux.Handle("POST /api/v1/bookings", rbacMw.RequireAuth(http.HandlerFunc(bookingsH.Create)))
	mux.Handle("GET /api/v1/bookings/{id}", rbacMw.RequireAuth(http.HandlerFunc(bookingsH.Get)))
	mux.Handle("GET /api/v1/me/bookings", rbacMw.RequireAuth(http.HandlerFunc(bookingsH.ListMine)))
	mux.Handle("GET /api/v1/me/guide/bookings", rbacMw.RequireAuth(http.HandlerFunc(bookingsH.ListGuide)))

	// Guide wallet & payout account (spec §8.1, Phase 7) — self-scoped;
	// destination refs are only ever returned masked.
	mux.Handle("GET /api/v1/me/guide/wallet", rbacMw.RequireAuth(http.HandlerFunc(payoutsH.Wallet)))
	mux.Handle("GET /api/v1/me/guide/statement", rbacMw.RequireAuth(http.HandlerFunc(payoutsH.Statement)))
	mux.Handle("GET /api/v1/me/guide/payout-account", rbacMw.RequireAuth(http.HandlerFunc(payoutsH.GetPayoutAccount)))
	mux.Handle("PUT /api/v1/me/guide/payout-account", rbacMw.RequireAuth(http.HandlerFunc(payoutsH.PutPayoutAccount)))

	// Payments & receipts (spec §13.3, Phase 4). Payment initiation and
	// refunds are idempotent (§14); the webhook is public but provider-
	// signature-authenticated; refunds require payments.refund and are
	// audited. Receipt downloads are short-lived signed URLs (§17).
	mux.Handle("POST /api/v1/bookings/{id}/payment-intent",
		rbacMw.RequireAuth(http.HandlerFunc(paymentsH.CreateIntent)))
	mux.HandleFunc("POST /api/v1/webhooks/payments/{provider}", paymentsH.Webhook)
	mux.Handle("GET /api/v1/bookings/{id}/receipt",
		rbacMw.RequireAuth(http.HandlerFunc(receiptsH.Get)))
	mux.Handle("GET /api/v1/payments/{id}",
		rbacMw.RequireAuth(http.HandlerFunc(paymentsH.Get)))
	mux.Handle("POST /api/v1/payments/{id}/refund",
		rbacMw.RequireAuth(rbac.RequirePermission("payments.refund")(http.HandlerFunc(paymentsH.Refund))))

	// Public catalog (spec §13.2).
	mux.HandleFunc("GET /api/v1/regions", catalogH.Regions)
	mux.HandleFunc("GET /api/v1/specialties", catalogH.Specialties)
	mux.HandleFunc("GET /api/v1/tour-packages", catalogH.TourPackages)

	// Admin (spec §13.6) — explicit permission checks per route.
	mux.Handle("GET /api/v1/admin/users",
		rbacMw.RequireAuth(rbac.RequirePermission("users.read")(http.HandlerFunc(adminH.ListUsers))))
	mux.Handle("PATCH /api/v1/admin/users/{id}/roles",
		rbacMw.RequireAuth(rbac.RequirePermission("users.manage")(http.HandlerFunc(adminH.SetUserRoles))))
	mux.Handle("GET /api/v1/admin/guides",
		rbacMw.RequireAuth(rbac.RequirePermission("guides.read")(http.HandlerFunc(adminH.ListGuides))))
	mux.Handle("GET /api/v1/admin/bookings",
		rbacMw.RequireAuth(rbac.RequirePermission("bookings.read")(http.HandlerFunc(adminH.ListBookings))))
	mux.Handle("GET /api/v1/admin/certification/queue",
		rbacMw.RequireAuth(rbac.RequirePermission("certification.read")(http.HandlerFunc(certH.Queue))))
	mux.Handle("GET /api/v1/admin/certification/{caseId}",
		rbacMw.RequireAuth(rbac.RequirePermission("certification.read")(http.HandlerFunc(certH.CaseDetail))))
	mux.Handle("POST /api/v1/admin/certification/{caseId}/transition",
		rbacMw.RequireAuth(rbac.RequirePermission("certification.review")(http.HandlerFunc(certH.Transition))))

	// Dispatch & tour operations (spec §10.3, §11, §13.4, Phase 5). Offers and
	// tour edges are self-scoped to the caller (guide id == user id); the
	// admin variants require dispatch.manage and are audited.
	mux.Handle("GET /api/v1/me/guide/offers",
		rbacMw.RequireAuth(http.HandlerFunc(dispatchH.ListMine)))
	mux.Handle("POST /api/v1/offers/{id}/accept",
		rbacMw.RequireAuth(http.HandlerFunc(dispatchH.Accept)))
	mux.Handle("POST /api/v1/offers/{id}/decline",
		rbacMw.RequireAuth(http.HandlerFunc(dispatchH.Decline)))
	mux.Handle("POST /api/v1/bookings/{id}/location",
		rbacMw.RequireAuth(http.HandlerFunc(trackingH.Post)))
	mux.Handle("GET /api/v1/bookings/{id}/location",
		rbacMw.RequireAuth(http.HandlerFunc(trackingH.Get)))
	mux.Handle("POST /api/v1/bookings/{id}/en-route",
		rbacMw.RequireAuth(http.HandlerFunc(toursH.EnRoute)))
	mux.Handle("POST /api/v1/bookings/{id}/arrived",
		rbacMw.RequireAuth(http.HandlerFunc(toursH.Arrived)))
	mux.Handle("POST /api/v1/bookings/{id}/start",
		rbacMw.RequireAuth(http.HandlerFunc(toursH.Start)))
	mux.Handle("POST /api/v1/bookings/{id}/complete",
		rbacMw.RequireAuth(http.HandlerFunc(toursH.Complete)))
	mux.Handle("POST /api/v1/admin/bookings/{id}/dispatch",
		rbacMw.RequireAuth(rbac.RequirePermission("dispatch.manage")(http.HandlerFunc(dispatchH.Dispatch))))
	mux.Handle("GET /api/v1/admin/bookings/{id}/dispatch",
		rbacMw.RequireAuth(rbac.RequirePermission("dispatch.manage")(http.HandlerFunc(dispatchH.BookingOffers))))
	mux.Handle("POST /api/v1/admin/bookings/{id}/transition",
		rbacMw.RequireAuth(rbac.RequirePermission("dispatch.manage")(http.HandlerFunc(toursH.AdminTransition))))

	// Safety & reviews (spec §4.4, §12, §13.5, Phase 6). SOS and review
	// creation are self-scoped to booking participants; public review reads
	// carry no tourist identity.
	mux.Handle("POST /api/v1/bookings/{id}/sos",
		rbacMw.RequireAuth(http.HandlerFunc(safetyH.Trigger)))
	mux.Handle("POST /api/v1/bookings/{id}/review",
		rbacMw.RequireAuth(http.HandlerFunc(reviewsH.Create)))
	mux.HandleFunc("GET /api/v1/guides/{id}/reviews", reviewsH.List)

	// Incident workflow & quality queue (spec §12, §4.4) — incidents.read to
	// view, incidents.manage to act, reviews.moderate for the quality queue.
	mux.Handle("GET /api/v1/admin/incidents",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.read")(http.HandlerFunc(incidentsH.List))))
	mux.Handle("GET /api/v1/admin/incidents/{id}",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.read")(http.HandlerFunc(incidentsH.Detail))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/acknowledge",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.manage")(http.HandlerFunc(incidentsH.Acknowledge))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/notes",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.manage")(http.HandlerFunc(incidentsH.Note))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/escalate",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.manage")(http.HandlerFunc(incidentsH.Escalate))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/assign",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.manage")(http.HandlerFunc(incidentsH.Assign))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/resolve",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.manage")(http.HandlerFunc(incidentsH.Resolve))))
	mux.Handle("POST /api/v1/admin/incidents/{id}/close",
		rbacMw.RequireAuth(rbac.RequirePermission("incidents.manage")(http.HandlerFunc(incidentsH.Close))))
	mux.Handle("GET /api/v1/admin/quality-flags",
		rbacMw.RequireAuth(rbac.RequirePermission("reviews.moderate")(http.HandlerFunc(incidentsH.ListFlags))))
	mux.Handle("POST /api/v1/admin/quality-flags/{id}/resolve",
		rbacMw.RequireAuth(rbac.RequirePermission("reviews.moderate")(http.HandlerFunc(incidentsH.ResolveFlag))))

	// Finance: payout batches, state machine, export & levy report (spec
	// §8.4, Phase 7) — payouts.read to view, payouts.manage to act, the CSV
	// export decrypts destination refs and is audited, reports.read for the
	// tourism-levy report.
	mux.Handle("GET /api/v1/admin/payouts",
		rbacMw.RequireAuth(rbac.RequirePermission("payouts.read")(http.HandlerFunc(payoutsH.List))))
	mux.Handle("POST /api/v1/admin/payouts/batch",
		rbacMw.RequireAuth(rbac.RequirePermission("payouts.manage")(http.HandlerFunc(payoutsH.Batch))))
	mux.Handle("GET /api/v1/admin/payouts/export",
		rbacMw.RequireAuth(rbac.RequirePermission("payouts.manage")(http.HandlerFunc(payoutsH.Export))))
	mux.Handle("POST /api/v1/admin/payouts/{id}/transition",
		rbacMw.RequireAuth(rbac.RequirePermission("payouts.manage")(http.HandlerFunc(payoutsH.Transition))))
	mux.Handle("POST /api/v1/admin/payout-accounts/{id}/verify",
		rbacMw.RequireAuth(rbac.RequirePermission("payouts.manage")(http.HandlerFunc(payoutsH.VerifyAccount))))
	mux.Handle("GET /api/v1/admin/reports/tourism-levy",
		rbacMw.RequireAuth(rbac.RequirePermission("reports.read")(http.HandlerFunc(payoutsH.LevyReport))))

	// Training LMS (spec §4.3, Phase 8) — guides self-enroll and progress;
	// content admins author courses and see rosters (training.manage).
	mux.Handle("GET /api/v1/me/training/courses",
		rbacMw.RequireAuth(http.HandlerFunc(trainingH.MyCourses)))
	mux.Handle("GET /api/v1/me/training/courses/{id}",
		rbacMw.RequireAuth(http.HandlerFunc(trainingH.GuideCourseDetail)))
	mux.Handle("POST /api/v1/me/training/courses/{id}/enroll",
		rbacMw.RequireAuth(http.HandlerFunc(trainingH.Enroll)))
	mux.Handle("POST /api/v1/me/training/lessons/{id}/complete",
		rbacMw.RequireAuth(http.HandlerFunc(trainingH.CompleteLesson)))
	mux.Handle("POST /api/v1/me/training/courses/{id}/quiz",
		rbacMw.RequireAuth(http.HandlerFunc(trainingH.SubmitQuiz)))
	mux.Handle("GET /api/v1/me/training/certificates",
		rbacMw.RequireAuth(http.HandlerFunc(trainingH.Certificates)))
	mux.Handle("POST /api/v1/admin/training/courses",
		rbacMw.RequireAuth(rbac.RequirePermission("training.manage")(http.HandlerFunc(trainingH.CreateCourse))))
	mux.Handle("GET /api/v1/admin/training/courses",
		rbacMw.RequireAuth(rbac.RequirePermission("training.manage")(http.HandlerFunc(trainingH.ListCourses))))
	mux.Handle("GET /api/v1/admin/training/courses/{id}",
		rbacMw.RequireAuth(rbac.RequirePermission("training.manage")(http.HandlerFunc(trainingH.AdminCourseDetail))))
	mux.Handle("PATCH /api/v1/admin/training/courses/{id}",
		rbacMw.RequireAuth(rbac.RequirePermission("training.manage")(http.HandlerFunc(trainingH.PatchCourse))))
	mux.Handle("GET /api/v1/admin/training/courses/{id}/enrollments",
		rbacMw.RequireAuth(rbac.RequirePermission("training.manage")(http.HandlerFunc(trainingH.Roster))))

	// Executive reporting & permitted exports (Phase 8) — reports.read to
	// view, reports.export for the CSV, audit.read for the audit viewer.
	mux.Handle("GET /api/v1/admin/reports/kpis",
		rbacMw.RequireAuth(rbac.RequirePermission("reports.read")(http.HandlerFunc(reportingH.KPIs))))
	mux.Handle("GET /api/v1/admin/reports/bookings",
		rbacMw.RequireAuth(rbac.RequirePermission("reports.read")(http.HandlerFunc(reportingH.Bookings))))
	mux.Handle("GET /api/v1/admin/reports/bookings/export",
		rbacMw.RequireAuth(rbac.RequirePermission("reports.export")(http.HandlerFunc(reportingH.ExportBookings))))
	mux.Handle("GET /api/v1/admin/audit-logs",
		rbacMw.RequireAuth(rbac.RequirePermission("audit.read")(http.HandlerFunc(reportingH.Audit))))

	// Platform configuration (Phase 8): versioned notification templates
	// and the system-settings policy editor (settings.manage, audited).
	mux.Handle("GET /api/v1/admin/notification-templates",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(adminH.ListAdminTemplates))))
	mux.Handle("POST /api/v1/admin/notification-templates",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(adminH.CreateTemplate))))
	mux.Handle("POST /api/v1/admin/notification-templates/{id}/activate",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(adminH.ActivateTemplate))))
	mux.Handle("GET /api/v1/admin/settings",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(adminH.ListSettings))))
	mux.Handle("PUT /api/v1/admin/settings/{key}",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(adminH.PutSetting))))

	// Legal documents (M-24). Publishing inserts a NEW version — never an
	// update — because consent_records references (document, version).
	// Approval is separate and is what clears the draft banner publicly.
	mux.Handle("GET /api/v1/admin/legal",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(privacyH.AdminListLegal))))
	mux.Handle("POST /api/v1/admin/legal/{document}",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(privacyH.AdminPublishLegal))))
	mux.Handle("POST /api/v1/admin/legal/{document}/{version}/approve",
		rbacMw.RequireAuth(rbac.RequirePermission("settings.manage")(http.HandlerFunc(privacyH.AdminApproveLegal))))

	// WebSockets (spec §11, §13.5). Token via ?token= query or session
	// cookie on the upgrade; JSON messages after that. These live at the
	// host root (not under /api/v1).
	mux.HandleFunc("GET /ws/guide", rtServer.Guide)
	mux.HandleFunc("GET /ws/booking/{id}", rtServer.Booking)
	mux.HandleFunc("GET /ws/admin/operations", rtServer.AdminOperations)

	// Local object storage: signed upload/download URLs are validated inside
	// the handler (never public — stop condition 8). Only mounted for the
	// local adapter; R2 uses provider-presigned URLs instead.
	if fileHandler != nil {
		mux.Handle("PUT /api/v1/files/{key...}", fileHandler)
		mux.Handle("GET /api/v1/files/{key...}", fileHandler)
	}

	_ = log // access log middleware owns logging from here
	return &apiApp{
		handler: httpx.SecurityHeadersMiddleware(
			httpx.CORSMiddleware(cfg.CORSAllowedOrigins)(
				observability.RequestIDMiddleware(
					observability.AccessLogMiddleware(log, mux)))),
		hub:   hub,
		sweep: dispatchSvc.ExpireOffers,
		payoutBatch: func(ctx context.Context) (int, error) {
			now := time.Now()
			if now.Weekday() != time.Monday {
				return 0, nil
			}
			today := now.Format("2006-01-02")
			due, err := payoutsSvc.BatchDueToday(ctx, today)
			if err != nil || !due {
				return 0, err
			}
			created, _, err := payoutsSvc.RunBatch(ctx, today, "scheduler")
			return created, err
		},
	}, nil
}

// healthzHandler is the liveness probe: the process is up, no dependency
// checks (spec §22).
func healthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// readyzHandler is the readiness probe: verifies Postgres and Redis
// connectivity and fails with 503 when either is unreachable.
func readyzHandler(pool *pgxpool.Pool, rdb *goredis.Client) http.HandlerFunc {
	type check struct {
		Postgres string `json:"postgres"`
		Redis    string `json:"redis"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		checks := check{Postgres: "ok", Redis: "ok"}
		ready := true

		if err := pool.Ping(ctx); err != nil {
			checks.Postgres = "unavailable"
			ready = false
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			checks.Redis = "unavailable"
			ready = false
		}

		if !ready {
			httpx.WriteError(w, r, http.StatusServiceUnavailable,
				"NOT_READY", "one or more dependencies are unavailable", checks)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ready", "checks": checks})
	}
}

// openAPIDocument is the committed OpenAPI 3.1 document source. It must
// cover every endpoint the server mounts; docs/api/openapi.yaml is
// regenerated from this output by the docs owner.
const openAPIDocument = `openapi: "3.1.0"
info:
  title: ProGuideGH API
  version: "0.9.0"
  description: >
    Certified tour-guide marketplace API. Phase 1 covers identity, RBAC and
    profiles (spec §13.1, §13.2, §13.4, §13.6); Phase 2 adds the certification
    pipeline (§5) and the public catalog (§13.2, §27); Phase 3 adds guide
    search and availability (§10) plus quote/booking creation with the §8.2
    state machine, idempotent creation and the double-booking guard (§13.3);
    Phase 4 adds payments, the immutable double-entry ledger and receipts
    (§4.5, §8.3, §9, §14, §16.1, §17); Phase 5 adds the §10.3 dispatch
    algorithm (marketplace bookings may omit guide_id), the §11 live-location
    pipeline with WebSocket feeds (/ws/guide, /ws/booking/{id},
    /ws/admin/operations — documented at the end of the paths section) and the tour
    operations state edges (§8.2, §13.4–13.5); Phase 6 adds SOS/incident
    management (§12), the verified review flow with Appendix B tags (§4.4,
    §13.5) and the quality/retraining queue (§13.6); Phase 7 adds the guide
    wallet and statement, tokenized payout accounts, the weekly payout batch
    with the §8.4 transition machine and ledger-backed PAID postings, the
    finance CSV export and the tourism-levy report (§8, §13.6); Phase 8 adds
    the light LMS (courses, enrollments, server-scored quizzes, certificates,
    §4.3), executive KPIs and operational reports with permitted CSV exports,
    versioned notification templates, the system-settings policy editor and
    the append-only audit-log viewer (§1.2, §13.6).
servers:
  - url: /api/v1
paths:
  /healthz:
    get:
      operationId: getLiveness
      summary: Liveness probe
      responses:
        "200":
          description: Process is alive.
  /readyz:
    get:
      operationId: getReadiness
      summary: Readiness probe (Postgres + Redis connectivity)
      responses:
        "200":
          description: All dependencies reachable.
        "503":
          description: A dependency is unavailable.

  # --- Auth (spec §13.1) ----------------------------------------------------
  /auth/register:
    post:
      operationId: register
      summary: Create account (intent tourist|guide); unauthenticated
      responses:
        "201": {description: Account created.}
        "409": {description: Email already registered.}
  /auth/login:
    post:
      operationId: login
      summary: Credential login; sets session cookies; rate-limited
      description: Returns mfa_required + challenge when the account has MFA enabled; complete via /auth/login/mfa.
      responses:
        "200": {description: Session issued, or MFA challenge required.}
        "401": {description: Invalid credentials.}
  /auth/login/mfa:
    post:
      operationId: loginMFA
      summary: Complete the MFA step-up with challenge + TOTP/backup code; rate-limited
      responses:
        "200": {description: Session issued.}
        "401": {description: Invalid or expired challenge/code.}
  /auth/otp/request:
    post:
      operationId: requestOTP
      summary: Request a 6-digit OTP (sms|email); rate-limited
      description: In APP_ENV=local the code is also returned as dev_code.
      responses:
        "200": {description: Code issued.}
  /auth/otp/verify:
    post:
      operationId: verifyOTP
      summary: Verify an OTP (5-min TTL, max 5 attempts); rate-limited
      responses:
        "200": {description: Code verified.}
        "401": {description: Invalid or expired code.}
        "429": {description: Too many attempts.}
  /auth/refresh:
    post:
      operationId: refreshSession
      summary: Rotate the refresh token and issue a new access token
      description: "The refresh token is accepted from three transports, in priority order: the X-Refresh-Token header (native clients), a JSON body field refresh_token (native clients), or the pgh_refresh session cookie (web). Rotation, reuse detection and revocation are identical across transports; the response always returns the new tokens in the JSON body (and refreshes the cookies)."
      responses:
        "200": {description: New session issued (access_token, refresh_token, expires_at).}
        "401": {description: Session invalid, expired or revoked (reuse detection).}
  /auth/logout:
    post:
      operationId: logout
      summary: Revoke the current session chain and clear cookies
      description: Accepts the refresh token via the X-Refresh-Token header, a JSON body field refresh_token, or the pgh_refresh cookie (same priority order as /auth/refresh).
      responses:
        "200": {description: Logged out.}
  /auth/password/forgot:
    post:
      operationId: forgotPassword
      summary: Begin password reset (hashed code via email); rate-limited; uniform response
      responses:
        "200": {description: Reset code issued when the account exists.}
  /auth/password/reset:
    post:
      operationId: resetPassword
      summary: Complete password reset; revokes all sessions; rate-limited
      responses:
        "200": {description: Password updated.}
        "401": {description: Invalid or expired code.}

  # --- Self-service ---------------------------------------------------------
  /me/tourist-profile:
    get:
      operationId: getTouristProfile
      summary: Current user's tourist profile (auth required)
      responses:
        "200": {description: Profile.}
        "401": {description: Unauthenticated.}
        "404": {description: No tourist profile.}
    patch:
      operationId: updateTouristProfile
      summary: Partially update the tourist profile (auth required)
      responses:
        "200": {description: Updated profile.}
        "401": {description: Unauthenticated.}
  /me/mfa/enroll:
    post:
      operationId: enrollMFA
      summary: Start TOTP MFA enrollment; returns secret + otpauth URI (auth required)
      responses:
        "200": {description: Enrollment started.}
        "409": {description: MFA already enabled.}
  /me/mfa/verify:
    post:
      operationId: verifyMFA
      summary: Confirm enrollment with a TOTP code; returns one-time backup codes (auth required, audited)
      responses:
        "200": {description: MFA enabled; backup codes returned once.}
        "401": {description: Invalid code.}

  # --- Data-subject rights (Apple 5.1.1(v), Google Play data deletion,
  #     Ghana Data Protection Act 2012) --------------------------------------
  /content/marketing:
    get:
      operationId: getMarketingContent
      summary: "Published marketing-site content from the system_settings key marketing.site. PUBLIC — it feeds the public website. Returns a null content field when nothing has been published so the site falls back to its built-in launch copy rather than erroring"
      responses:
        "200": {description: Marketing content document, or null.}
  /legal/policies:
    get:
      operationId: getLegalPolicies
      summary: "Current terms/privacy/location policy versions and URLs. PUBLIC — both stores require the privacy policy to be reachable without an account, and sign-up must show it before one exists"
      responses:
        "200": {description: Published policy versions.}
  /me/consent:
    post:
      operationId: recordConsent
      summary: "Record acceptance of a policy version {document: terms|privacy|location, version}. Append-only: consent must be demonstrable, so a later acceptance is a new row (auth required)"
      responses:
        "201": {description: Consent recorded.}
        "400": {description: Unknown document or missing version.}
        "401": {description: Unauthenticated.}
  /me/export:
    get:
      operationId: exportMyData
      summary: "Subject-access export: account, profile, bookings, reviews and consent history as JSON. Financial records are excluded and the reason is stated in the payload notes. Audited as a privileged read (auth required)"
      responses:
        "200": {description: Personal data export.}
        "401": {description: Unauthenticated.}
  /me/deletion:
    get:
      operationId: previewAccountDeletion
      summary: "What deletion removes, what is retained and why, and whether anything currently blocks it (active booking, unsettled payout) (auth required)"
      responses:
        "200": {description: Deletion preview with can_delete and blockers.}
        "401": {description: Unauthenticated.}
  /me:
    delete:
      operationId: deleteMyAccount
      summary: "Irreversible account erasure. Anonymizes identity; deletes profile, verification documents (incl. private objects), payout accounts, MFA secrets, notifications and location history; revokes every session. Append-only financial and audit records are retained under the legal-obligation exemption and no longer identify a person. Audited (auth required)"
      responses:
        "200": {description: Account deleted; cleared data classes returned.}
        "401": {description: Unauthenticated.}
        "404": {description: Account not found.}
        "409": {description: Blocked by an active booking or unsettled payout; the specific reason is returned.}

  # --- Guides (spec §13.4) --------------------------------------------------
  /guides/apply:
    post:
      operationId: applyGuide
      summary: Create guide application shell + guide_applicant role + certification case (APPLIED); idempotent per user (auth required)
      responses:
        "201": {description: Application created (or existing profile returned), with its certification case.}
  /guides/documents:
    post:
      operationId: registerGuideDocument
      summary: Register document metadata and obtain a short-lived signed upload URL (auth required, guide profile must exist)
      responses:
        "201": {description: Document registered; upload_url returned.}
        "409": {description: No guide profile; apply first.}
  /guides/{id}:
    get:
      operationId: getGuide
      summary: Public guide detail; 404 unless the guide passes the §10.2 visibility gate (ACTIVE certification, unsuspended account, valid mandatory documents)
      responses:
        "200": {description: Public guide detail with languages and specialties.}
        "404": {description: Unknown id or guide not publicly visible.}
  /me/guide:
    get:
      operationId: getGuideDashboard
      summary: Guide dashboard aggregate — profile, current certification case, outstanding requirements, documents (auth required, own record)
      responses:
        "200": {description: Dashboard aggregate.}
        "404": {description: No guide profile; apply first.}
  /me/guide/certification:
    get:
      operationId: getGuideCertification
      summary: Certification pipeline detail — current case + immutable event history (auth required, own record)
      responses:
        "200": {description: Case and events.}
        "404": {description: No certification case.}
  /me/guide/profile:
    patch:
      operationId: updateGuideProfile
      summary: Partially update guide profile (bio, public_name, region_id, latitude/longitude operating base, languages with proficiency, specialty_ids); languages/specialties replaced atomically (auth required)
      responses:
        "200": {description: Updated profile with languages and specialties.}
        "400": {description: Unknown region/language/specialty or invalid proficiency/coordinates.}
        "404": {description: No guide profile; apply first.}

  # --- Search, availability & bookings (spec §10, §13.2–13.4) ----------------
  /guides/search:
    get:
      operationId: searchGuides
      summary: Search §10.2-eligible guides with the §10.1 filters (region_id or lat/lng+radius_km, starts_at+ends_at or package_id availability, language, min_proficiency, specialty_id, min_rating, elite, available_now); deterministic rank rating_avg desc/rating_count desc; offset pagination over the bounded eligible set
      responses:
        "200": {description: Ranked page of guides with total/limit/offset.}
        "400": {description: Invalid filter combination or value.}
  /me/guide/availability:
    post:
      operationId: setGuideAvailability
      summary: Go online/offline {online}; online is a Redis presence marker with a 300s TTL — heartbeat within the TTL to stay discoverable (auth required, guide profile must exist)
      responses:
        "200": {description: Presence updated; ttl_seconds returned.}
        "404": {description: No guide profile; apply first.}
  /me/guide/availability/schedule:
    put:
      operationId: replaceGuideSchedule
      summary: "Atomically replace the recurring weekly schedule {windows: [{weekday 0-6, start_time HH:MM, end_time HH:MM, timezone?}]}; empty array clears availability (auth required)"
      responses:
        "200": {description: The schedule after replacement.}
        "400": {description: Invalid weekday/clock/timezone or end not after start.}
        "404": {description: No guide profile; apply first.}
  /me/guide/availability/time-off:
    post:
      operationId: addGuideTimeOff
      summary: Record one-off unavailability {starts_at, ends_at, reason?}; wins over the weekly schedule for search and booking (auth required)
      responses:
        "201": {description: Time-off row created.}
        "400": {description: Invalid interval.}
        "404": {description: No guide profile; apply first.}
  /me/guide/availability/time-off/{id}:
    delete:
      operationId: deleteGuideTimeOff
      summary: Delete one of the guide's own time-off rows (auth required)
      responses:
        "204": {description: Deleted.}
        "404": {description: Unknown id or not the guide's row.}
  /bookings/quote:
    post:
      operationId: quoteBooking
      summary: Server-authoritative quote {package_id, starts_at, guests?} → package, computed interval and price breakdown (amount, platform_fee, tourism_levy, guide_payable_estimate) from EffectivePrice + system_settings percentages; client totals are never trusted (spec §14, §27)
      responses:
        "200": {description: Quote.}
        "404": {description: Package not found.}
        "422": {description: Package inactive.}
  /bookings:
    post:
      operationId: createBooking
      summary: "Create a PAYMENT_PENDING booking {package_id, guide_id?, starts_at, meeting_point?, meeting_lat/lng?, guests?, notes?}. Direct flow (guide_id set): §10.2 eligibility, availability and overlap validation. Marketplace flow (guide_id omitted): guide checks defer to Phase 5 dispatch after payment confirmation; pricing uses the package default rule. Idempotency-Key header REQUIRED — replay returns the original booking, reuse with a different payload conflicts (auth required)"
      responses:
        "201": {description: Booking created (reference PGH-XXXXX) with the price snapshot.}
        "200": {description: Idempotent replay of the original creation.}
        "400": {description: Validation failure or missing Idempotency-Key.}
        "404": {description: Package not found.}
        "409": {description: Overlapping active booking, or Idempotency-Key conflict/in-progress.}
        "422": {description: Guide not §10.2-eligible, guide unavailable, or package inactive.}
  /bookings/{id}:
    get:
      operationId: getBooking
      summary: Booking detail with immutable status-event history; visible to the owning tourist, the assigned guide or bookings.read holders — 404 for anyone else (auth required)
      responses:
        "200": {description: Booking and events.}
        "404": {description: Unknown id or not visible to the caller.}
  /me/bookings:
    get:
      operationId: listMyBookings
      summary: The caller's own bookings, newest first; cursor pagination (created_at,id keyset) per spec §14 (auth required)
      responses:
        "200": {description: Page of bookings with next_cursor.}
        "400": {description: Invalid cursor.}
  /me/guide/bookings:
    get:
      operationId: listGuideBookings
      summary: "Every booking assigned to the caller as guide — id, reference, status, package_name, starts_at/ends_at, meeting_point, num_guests, amount, tourist_name. Ordered upcoming-first (starts_at >= now ascending) then past (descending), so clients can split the list without re-sorting (auth required)"
      responses:
        "200": {description: The guide's assigned bookings.}
        "401": {description: Unauthenticated.}

  # --- Payments, ledger & receipts (spec §4.5, §8.3, §9, §13.3, §17) ---------
  /bookings/{id}/payment-intent:
    post:
      operationId: createPaymentIntent
      summary: Initialize a provider payment for a PAYMENT_PENDING booking against the server-authoritative amount snapshot (owner tourist only, auth required); Idempotency-Key REQUIRED — replay returns the same payment, provider reference and authorization_url
      responses:
        "201": {description: Payment initialized (PENDING) with provider reference and authorization_url.}
        "200": {description: Idempotent replay of the original initiation.}
        "400": {description: Missing Idempotency-Key.}
        "404": {description: Unknown booking or not the caller's booking.}
        "409": {description: Booking not awaiting payment, active payment exists, or Idempotency-Key conflict.}
  /webhooks/payments/{provider}:
    post:
      operationId: receivePaymentWebhook
      summary: Signed provider callback (public; the provider signature authenticates). Verified on the exact raw bytes before parsing; deduped on (provider, event_reference) in the same transaction as the side effects — payment SUCCEEDED, booking CONFIRMED, one balanced §9.1 ledger allocation, receipt issued, notifications queued. Replays are 200 no-ops.
      responses:
        "200": {description: Processed (or replay no-op).}
        "400": {description: Malformed payload or unknown payment reference.}
        "401": {description: Signature verification failed.}
        "404": {description: Unknown/inactive provider.}
  /bookings/{id}/receipt:
    get:
      operationId: getBookingReceipt
      summary: Receipt metadata plus a short-lived signed download URL for the PDF (owner tourist, assigned guide or bookings.read; spec §17)
      responses:
        "200": {description: Receipt metadata and download_url (expires_in seconds).}
        "404": {description: Unknown booking/receipt, not visible to the caller, or no receipt issued yet.}
  /payments/{id}:
    get:
      operationId: getPayment
      summary: Payment detail (owning tourist or payments.read; auth required)
      responses:
        "200": {description: Payment.}
        "404": {description: Unknown id or not visible to the caller.}
  /payments/{id}/refund:
    post:
      operationId: refundPayment
      summary: Full refund of a SUCCEEDED payment (permission payments.refund; audited; Idempotency-Key REQUIRED) — provider refund, payment REFUND_PENDING→REFUNDED, booking driven to REFUNDED through the §8.2 state machine, reversing ledger entries posted (originals preserved, §9.2)
      responses:
        "200": {description: Payment REFUNDED with refund id and reversal ledger reference.}
        "400": {description: Missing Idempotency-Key.}
        "403": {description: Missing payments.refund.}
        "404": {description: Payment not found.}
        "409": {description: Payment not refundable or Idempotency-Key conflict.}

  /files/{key}:
    put:
      operationId: uploadFile
      summary: Upload an object via a signed URL (local storage adapter; HMAC signature + expiry enforced)
      responses:
        "201": {description: Stored.}
        "403": {description: Invalid/expired signature.}
    get:
      operationId: downloadFile
      summary: Download an object via a signed URL (local storage adapter)
      responses:
        "200": {description: Object bytes.}
        "403": {description: Invalid/expired signature.}
        "404": {description: Not found.}

  # --- Public catalog (spec §13.2) ------------------------------------------
  /regions:
    get:
      operationId: listRegions
      summary: List Ghana's regions (unauthenticated)
      responses:
        "200": {description: All regions.}
  /specialties:
    get:
      operationId: listSpecialties
      summary: List guide specialties (unauthenticated, spec Appendix C)
      responses:
        "200": {description: All specialties.}
  /tour-packages:
    get:
      operationId: listTourPackages
      summary: List active tour packages with the current effective price from pricing_rules (unauthenticated, server-authoritative)
      responses:
        "200": {description: Active packages with current prices.}

  # --- Admin (spec §13.6) ---------------------------------------------------
  /admin/bookings:
    get:
      operationId: adminListBookings
      summary: "Operations bookings board: ?status=active (CONFIRMED/GUIDE_EN_ROUTE/GUIDE_ARRIVED/IN_PROGRESS), a single §8.2 status, or a comma list; rows carry reference, status, guide {id, name}, tourist {id, name}, package_name, starts_at, updated_at and last_event_at; most-recently-updated first; offset pagination (permission bookings.read)"
      responses:
        "200": {description: Page of bookings with total/limit/offset.}
        "400": {description: Invalid status filter.}
        "403": {description: Missing bookings.read.}
  /admin/users:
    get:
      operationId: adminListUsers
      summary: List users with roles, offset pagination (permission users.read)
      responses:
        "200": {description: Page of users.}
        "403": {description: Missing users.read.}
  /admin/users/{id}/roles:
    patch:
      operationId: adminSetUserRoles
      summary: Replace a user's role set (permission users.manage; audited; revokes sessions when roles removed)
      responses:
        "200": {description: Roles updated.}
        "403": {description: Missing users.manage.}
        "404": {description: User not found.}
  /admin/guides:
    get:
      operationId: adminListGuides
      summary: Guide queue with optional status filter, offset pagination (permission guides.read)
      responses:
        "200": {description: Page of guide profiles.}
        "403": {description: Missing guides.read.}
  /admin/certification/queue:
    get:
      operationId: adminCertificationQueue
      summary: Certification cases by stage with guide identity; optional ?status= filter (permission certification.read)
      responses:
        "200": {description: Page of certification cases.}
        "400": {description: Invalid status filter.}
        "403": {description: Missing certification.read.}
  /admin/certification/{caseId}:
    get:
      operationId: adminCertificationCase
      summary: Certification case detail with guide documents and immutable event history (permission certification.read)
      responses:
        "200": {description: Case, documents and events.}
        "403": {description: Missing certification.read.}
        "404": {description: Case not found.}
  /admin/certification/{caseId}/transition:
    post:
      operationId: adminCertificationTransition
      summary: Apply a validated state-machine transition {to_status, reason, evidence_ref?}; evidence stages (spec §5) require an evidence_ref plus a valid unexpired document; writes an immutable event and an audit row (permission certification.review)
      responses:
        "200": {description: Updated case and the new event.}
        "400": {description: Unknown to_status or missing reason.}
        "403": {description: Missing certification.review.}
        "404": {description: Case not found.}
        "409": {description: Illegal transition for the case's current status.}
        "422": {description: Evidence required for the target stage.}

  # --- Dispatch & tour operations (spec §10.3, §11, §13.4–13.5, Phase 5) ------
  /me/guide/offers:
    get:
      operationId: listGuideOffers
      summary: The caller's current OFFERED, unexpired dispatch offers with booking summaries (auth required). This is the realtime catch-up path — the same rows are pushed on /ws/guide.
      responses:
        "200": {description: Current offers.}
        "401": {description: Unauthenticated.}
  /offers/{id}/accept:
    post:
      operationId: acceptOffer
      summary: "Atomically accept one of the caller's offers (§10.3 step 4 — first valid acceptance wins). One transaction: offer OFFERED + unexpired (DB expires_at is authoritative), booking still CONFIRMED and guideless, guide assigned, siblings SUPERSEDED, immutable event written (auth required)"
      responses:
        "200": {description: Offer ACCEPTED; booking assigned to the caller.}
        "404": {description: Unknown offer (or another guide's offer).}
        "409": {description: Offer already resolved, another guide won, or the guide holds an overlapping active booking.}
        "410": {description: Offer expired.}
  /offers/{id}/decline:
    post:
      operationId: declineOffer
      summary: Decline one of the caller's offers; when the decline empties the booking's live offer set the next batch is dispatched (excluding decliners) (auth required)
      responses:
        "200": {description: Offer DECLINED.}
        "404": {description: Unknown offer (or another guide's offer).}
        "409": {description: Offer already resolved.}
        "410": {description: Offer expired.}
  /bookings/{id}/location:
    post:
      operationId: postBookingLocation
      summary: Guide location update (spec §11.1 payload minus booking_id, which is the path id). Assigned guide only, booking in GUIDE_EN_ROUTE..IN_PROGRESS. Writes the Redis current-position keys (60s TTL), persists a coarse checkpoint per the §11.2 retention policy and fans out to /ws/booking/{id} and /ws/admin/operations (auth required)
      responses:
        "202": {description: Position recorded; ttl_seconds returned.}
        "400": {description: Out-of-range coordinates/heading/speed/captured_at.}
        "403": {description: Caller is not the assigned guide.}
        "404": {description: Unknown booking.}
        "409": {description: Booking outside the active location window.}
    get:
      operationId: getBookingLocation
      summary: Current cached position (Redis). Owning tourist or assigned guide (active window only) or dispatch.manage (operations live map, §11.2); 404 for anyone else and outside the window — no historical movement is exposed (auth required)
      responses:
        "200": {description: Current position.}
        "404": {description: Unknown/invisible booking, inactive window, or no position cached.}
  /bookings/{id}/en-route:
    post:
      operationId: bookingEnRoute
      summary: CONFIRMED -> GUIDE_EN_ROUTE through the §8.2 state machine (assigned guide only)
      responses:
        "200": {description: Booking transitioned.}
        "404": {description: Unknown booking or not assigned to the caller.}
        "409": {description: Illegal transition for the current status.}
  /bookings/{id}/arrived:
    post:
      operationId: bookingArrived
      summary: GUIDE_EN_ROUTE -> GUIDE_ARRIVED through the §8.2 state machine (assigned guide only)
      responses:
        "200": {description: Booking transitioned.}
        "404": {description: Unknown booking or not assigned to the caller.}
        "409": {description: Illegal transition for the current status.}
  /bookings/{id}/start:
    post:
      operationId: bookingStart
      summary: GUIDE_ARRIVED -> IN_PROGRESS through the §8.2 state machine (assigned guide only)
      responses:
        "200": {description: Booking transitioned.}
        "404": {description: Unknown booking or not assigned to the caller.}
        "409": {description: Illegal transition for the current status.}
  /bookings/{id}/complete:
    post:
      operationId: bookingComplete
      summary: IN_PROGRESS -> COMPLETED (assigned guide only). Same transaction sets ends_at to the actual completion time and moves the booking's guide payable from pending to eligible in the ledger (§9.2, idempotent reference ELIGIBLE:<booking>); payout_delay_days is applied at payout time (Phase 7)
      responses:
        "200": {description: Booking COMPLETED.}
        "404": {description: Unknown booking or not assigned to the caller.}
        "409": {description: Illegal transition for the current status.}
  /admin/bookings/{id}/dispatch:
    post:
      operationId: adminDispatchBooking
      summary: Run (or re-run) a dispatch offer batch for a CONFIRMED guideless booking (permission dispatch.manage; audited). Idempotent while a batch is live — returns the live batch instead of duplicating offers
      responses:
        "200": {description: Batch result (offers, candidates, batch_seq, reused_live).}
        "403": {description: Missing dispatch.manage.}
        "404": {description: Booking not found.}
        "409": {description: Booking not awaiting dispatch (wrong status or guide assigned).}
    get:
      operationId: adminBookingDispatchStatus
      summary: Operations "why unmatched" view (§30.2) — the booking plus every offer batch with outcomes, newest first (permission dispatch.manage)
      responses:
        "200": {description: Booking summary and offer history.}
        "403": {description: Missing dispatch.manage.}
        "404": {description: Booking not found.}
  /admin/bookings/{id}/transition:
    post:
      operationId: adminBookingTransition
      summary: Operations override — one validated §8.2 transition {to_status, reason} with mandatory reason; the state machine still decides legality; audited (permission dispatch.manage)
      responses:
        "200": {description: Booking and the new event.}
        "400": {description: Missing reason.}
        "403": {description: Missing dispatch.manage.}
        "404": {description: Booking not found.}
        "409": {description: Illegal transition or overlapping active booking.}

  # --- Safety, reviews & quality (spec §4.4, §12, Phase 6) -------------------
  /bookings/{id}/sos:
    post:
      operationId: triggerSOS
      summary: Raise an SOS from an active booking (spec §12). Booking participant only (tourist or assigned guide); freshest coordinates required. One transaction creates the immutable SOS event and a CRITICAL incident; operations is alerted over /ws/admin/operations and the responder roster gets fallback notifications. Rate-limited per user (§15.2). The response names ProGuideGH operations as the responder — never police or emergency services
      responses:
        "201": {description: SOS event + incident created, with the operations-not-emergency-services message.}
        "400": {description: Missing/out-of-range coordinates.}
        "404": {description: Unknown booking or not a participant.}
        "409": {description: Booking is not active.}
        "429": {description: SOS rate limit exceeded.}
  /bookings/{id}/review:
    post:
      operationId: createReview
      summary: One verified review per completed booking (spec §4.4), by the booking's tourist only. Rating 1-5, optional body, tags from the Appendix B dictionary. Refreshes the guide's rating aggregate and evaluates the quality thresholds (low-rating retraining flag, Elite review flag)
      responses:
        "201": {description: Review created.}
        "400": {description: Invalid rating or non-Appendix-B tag.}
        "404": {description: Unknown booking or not the booking's tourist.}
        "409": {description: Booking already reviewed.}
        "422": {description: Booking not completed (or had no guide).}
  /guides/{id}/reviews:
    get:
      operationId: listGuideReviews
      summary: Public reviews for a guide plus the rating aggregate (offset pagination; no tourist identity exposed, §14)
      responses:
        "200": {description: Reviews, rating_avg, rating_count.}
        "404": {description: Malformed guide id.}
  /admin/incidents:
    get:
      operationId: adminListIncidents
      summary: Incident list with status/severity/type filters, offset pagination (permission incidents.read)
      responses:
        "200": {description: Incidents and total.}
        "403": {description: Missing incidents.read.}
  /admin/incidents/{id}:
    get:
      operationId: adminIncidentDetail
      summary: One incident plus its full timestamped, attributed audit trail — every acknowledgement, note, escalation, assignment and closure (§12 step 11; permission incidents.read)
      responses:
        "200": {description: Incident and events.}
        "403": {description: Missing incidents.read.}
        "404": {description: Incident not found.}
  /admin/incidents/{id}/acknowledge:
    post:
      operationId: adminAcknowledgeIncident
      summary: open -> acknowledged; for SOS incidents also marks the underlying SOS events acknowledged (permission incidents.manage; audited)
      responses:
        "200": {description: Incident updated.}
        "409": {description: Illegal status transition.}
  /admin/incidents/{id}/notes:
    post:
      operationId: adminIncidentNote
      summary: Append a timestamped note without changing status (permission incidents.manage; audited)
      responses:
        "200": {description: Incident updated.}
        "400": {description: Empty note body.}
  /admin/incidents/{id}/escalate:
    post:
      operationId: adminEscalateIncident
      summary: Bump severity one step (low -> medium -> high -> critical) with a trail entry (permission incidents.manage; audited)
      responses:
        "200": {description: Incident updated.}
        "400": {description: Already at maximum severity.}
  /admin/incidents/{id}/assign:
    post:
      operationId: adminAssignIncident
      summary: Route the incident to an operations user {user_id} (permission incidents.manage; audited)
      responses:
        "200": {description: Incident updated.}
        "400": {description: Missing user_id.}
  /admin/incidents/{id}/resolve:
    post:
      operationId: adminResolveIncident
      summary: Resolve with a mandatory resolution note {note} (permission incidents.manage; audited)
      responses:
        "200": {description: Incident updated.}
        "400": {description: Missing resolution note.}
        "409": {description: Illegal status transition.}
  /admin/incidents/{id}/close:
    post:
      operationId: adminCloseIncident
      summary: Close (terminal) with an optional note (permission incidents.manage; audited)
      responses:
        "200": {description: Incident updated.}
        "409": {description: Illegal status transition.}
  /admin/quality-flags:
    get:
      operationId: adminListQualityFlags
      summary: "The quality/retraining queue (spec §4.4): low_rating and elite_review flags, open first, with guide name and the rating at flag time (permission reviews.moderate)"
      responses:
        "200": {description: Quality flags.}
        "403": {description: Missing reviews.moderate.}
  /admin/quality-flags/{id}/resolve:
    post:
      operationId: adminResolveQualityFlag
      summary: Close one open quality flag with a mandatory note (permission reviews.moderate; audited)
      responses:
        "200": {description: Flag resolved.}
        "400": {description: Missing note.}
        "409": {description: Already resolved or not found.}

  # --- Wallet, payouts & finance (spec §8, Phase 7) -----------------------
  /me/guide/wallet:
    get:
      operationId: guideWallet
      summary: The caller's balance summary in minor units {pending, eligible, payout_eligible, in_flight, paid_total, payout_delay_days} — eligible is net of payout drawdowns (spec §8.1, P7-01)
      responses:
        "200": {description: Wallet summary.}
        "401": {description: Unauthenticated.}
  /me/guide/statement:
    get:
      operationId: guideStatement
      summary: Wallet statement — ledger movements on the eligible payable balance plus payout rows, newest first, keyset cursor pagination (?cursor&limit) (P7-01)
      responses:
        "200": {description: Statement entries with next_cursor.}
        "400": {description: Invalid cursor.}
  /me/guide/payout-account:
    get:
      operationId: guideGetPayoutAccount
      summary: The caller's current payout destination, masked (last four only) — account null when none registered (P7-02)
      responses:
        "200": {description: Masked payout account or null.}
    put:
      operationId: guidePutPayoutAccount
      summary: Register a new payout destination {provider, network?, account_ref}; encrypted at rest (AES-256-GCM), only the masked form is ever returned; audited (P7-02)
      responses:
        "200": {description: Account registered, masked ref returned.}
        "400": {description: Missing provider/account_ref.}
  /admin/payouts:
    get:
      operationId: adminListPayouts
      summary: Payout list with ?status&scheduled_for filters, offset pagination (permission payouts.read)
      responses:
        "200": {description: Payouts with total.}
        "403": {description: Missing payouts.read.}
  /admin/payouts/batch:
    post:
      operationId: adminRunPayoutBatch
      summary: Queue one QUEUED payout per guide whose eligible balance cleared the payout-delay hold, net of in-flight/paid payouts {scheduled_for?, default today}; idempotent per (guide, scheduled_for) — also runs automatically each Monday (P7-03, P7-07; permission payouts.manage; audited)
      responses:
        "200": {description: "{scheduled_for, eligible_guides, created}."}
        "400": {description: Bad scheduled_for.}
        "403": {description: Missing payouts.manage.}
  /admin/payouts/export:
    get:
      operationId: adminExportPayouts
      summary: "Finance transfer CSV (text/csv) of QUEUED/RETRY_QUEUED payouts for ?scheduled_for=YYYY-MM-DD with decrypted destination refs — the only plaintext-ref surface; audited (P7-04; permission payouts.manage)"
      responses:
        "200": {description: CSV attachment.}
        "400": {description: Bad scheduled_for.}
        "403": {description: Missing payouts.manage.}
  /admin/payouts/{id}/transition:
    post:
      operationId: adminTransitionPayout
      summary: "Move a payout through the §8.4 machine {to, failure_reason?, provider_reference?}: QUEUED→PROCESSING→PAID|FAILED, FAILED→RETRY_QUEUED|MANUAL_REVIEW, MANUAL_REVIEW→RETRY_QUEUED. PAID posts the balanced ledger move (debit guide_payable_eligible, credit tourist_clearing) atomically and links it; FAILED requires failure_reason (P7-05; permission payouts.manage; audited)"
      responses:
        "200": {description: Payout transitioned.}
        "400": {description: Missing failure_reason.}
        "404": {description: Not found.}
        "409": {description: Illegal transition.}
  /admin/payout-accounts/{id}/verify:
    post:
      operationId: adminVerifyPayoutAccount
      summary: Mark a payout account verified (permission payouts.manage; audited)
      responses:
        "200": {description: Account verified.}
        "404": {description: Not found.}
  /admin/reports/tourism-levy:
    get:
      operationId: adminTourismLevyReport
      summary: Tourism-levy payable balance (all time) plus ?from&to (YYYY-MM-DD, inclusive) period credits/debits in minor units (P7-06; permission reports.read)
      responses:
        "200": {description: Levy report.}
        "400": {description: Bad date bounds.}
        "403": {description: Missing reports.read.}

  # --- Training LMS, reporting & platform config (spec §4.3, §13.6, Phase 8)
  /me/training/courses:
    get:
      operationId: guideListCourses
      summary: Active courses with the caller's enrollment/progress overlaid (P8-01)
      responses:
        "200": {description: Courses with enrollment state.}
  /me/training/courses/{id}:
    get:
      operationId: guideCourseDetail
      summary: Course tree for a guide — quiz questions arrive with answer_index stripped; completed lessons flagged (P8-01)
      responses:
        "200": {description: Course detail.}
        "404": {description: Not found.}
  /me/training/courses/{id}/enroll:
    post:
      operationId: guideEnroll
      summary: Enroll on an active course; idempotent on re-enroll (UNIQUE guide/course, P8-01)
      responses:
        "200": {description: Enrollment.}
        "400": {description: Course not active.}
  /me/training/lessons/{id}/complete:
    post:
      operationId: guideCompleteLesson
      summary: Mark a lesson done (idempotent); when every lesson is complete and the quiz passed, the enrollment completes and a certificate is issued (P8-01)
      responses:
        "200": {description: Updated enrollment.}
        "404": {description: Lesson not in the caller's enrollments.}
  /me/training/courses/{id}/quiz:
    post:
      operationId: guideSubmitQuiz
      summary: "Score a quiz attempt server-side {answers: [int]} → {score, passed, enrollment}; attempts are stored, answers never leave the server (P8-01)"
      responses:
        "200": {description: Scored attempt.}
        "400": {description: Wrong answer count or no quiz.}
  /me/training/certificates:
    get:
      operationId: guideCertificates
      summary: The caller's issued certificates (PGH-CERT- serials) (P8-01)
      responses:
        "200": {description: Certificates.}
  /admin/training/courses:
    post:
      operationId: adminCreateCourse
      summary: Create a course with nested modules/lessons/quiz atomically (permission training.manage; audited)
      responses:
        "201": {description: Course created.}
        "400": {description: Invalid content.}
        "409": {description: Duplicate code.}
    get:
      operationId: adminListCourses
      summary: All courses incl. inactive with content counts (permission training.manage)
      responses:
        "200": {description: Courses.}
  /admin/training/courses/{id}:
    get:
      operationId: adminCourseDetail
      summary: Full course tree including quiz answers — admin-only surface (permission training.manage)
      responses:
        "200": {description: Course detail.}
        "404": {description: Not found.}
    patch:
      operationId: adminPatchCourse
      summary: Update title/description/pass_score/required flag/active (permission training.manage; audited)
      responses:
        "200": {description: Course updated.}
        "404": {description: Not found.}
  /admin/training/courses/{id}/enrollments:
    get:
      operationId: adminCourseRoster
      summary: Course roster with per-guide progress, best score and certificate serial (permission training.manage)
      responses:
        "200": {description: Roster.}
  /admin/reports/kpis:
    get:
      operationId: adminKPIs
      summary: Executive dashboard numbers — users, certified guides, bookings, GMV, platform revenue, SOS, ratings, payouts paid (30-day windows where stated; permission reports.read) (P8-02)
      responses:
        "200": {description: KPIs.}
        "403": {description: Missing reports.read.}
  /admin/reports/bookings:
    get:
      operationId: adminBookingsReport
      summary: Bookings report — per-status counts, GMV and refunded totals for ?from&to (default last 30 days; permission reports.read) (P8-02)
      responses:
        "200": {description: Report.}
        "400": {description: Bad date bounds.}
  /admin/reports/bookings/export:
    get:
      operationId: adminExportBookings
      summary: "Permitted bookings CSV (text/csv) for ?from&to (permission reports.export; audited) (P8-02)"
      responses:
        "200": {description: CSV attachment.}
        "403": {description: Missing reports.export.}
  /admin/audit-logs:
    get:
      operationId: adminAuditLogs
      summary: Append-only audit trail viewer with ?actor_id&action&entity_type&entity_id&from&to filters, offset pagination (permission audit.read, spec §1.2) (P8-04)
      responses:
        "200": {description: Audit entries with total.}
        "403": {description: Missing audit.read.}
  /admin/notification-templates:
    get:
      operationId: adminListTemplates
      summary: Every notification-template version, newest first per key (permission settings.manage) (P8-03)
      responses:
        "200": {description: Templates.}
    post:
      operationId: adminCreateTemplate
      summary: Create the next version of a template key, inactive {key, channel, subject, body} (permission settings.manage; audited) (P8-03)
      responses:
        "201": {description: Template version created.}
        "400": {description: Missing key/body or bad channel.}
  /admin/notification-templates/{id}/activate:
    post:
      operationId: adminActivateTemplate
      summary: Activate one version, atomically superseding the key's previous active version (permission settings.manage; audited) (P8-03)
      responses:
        "200": {description: Template activated.}
        "404": {description: Not found.}
  /admin/settings:
    get:
      operationId: adminListSettings
      summary: All system settings (policy configuration, permission settings.manage) (P8-04)
      responses:
        "200": {description: Settings.}
  /admin/legal:
    get:
      operationId: adminListLegalDocuments
      summary: "Every version of every legal document, newest first, with bodies (permission settings.manage)"
      responses:
        "200": {description: Legal document versions.}
        "403": {description: Missing settings.manage.}
  /admin/legal/{document}:
    post:
      operationId: adminPublishLegalDocument
      summary: "Publish a NEW version of terms/privacy/location {version, url?, summary, body}. Never updates an existing version — consent_records references (document, version), so rewriting text in place would silently re-point recorded consent at different words. Audited (permission settings.manage)"
      responses:
        "201": {description: Version published, unapproved.}
        "400": {description: Missing version or body.}
        "403": {description: Missing settings.manage.}
        "404": {description: Unknown document.}
        "409": {description: That version already exists.}
  /admin/legal/{document}/{version}/approve:
    post:
      operationId: adminApproveLegalDocument
      summary: "Mark a version counsel-approved, which removes the draft banner from the public page. Audited (permission settings.manage)"
      responses:
        "200": {description: Approved.}
        "403": {description: Missing settings.manage.}
        "404": {description: Unknown version, or already approved.}
  /admin/settings/{key}:
    put:
      operationId: adminPutSetting
      summary: "Upsert one setting {value: <any JSON>}; bumps the row version, audited with before/after (permission settings.manage) (P8-04)"
      responses:
        "200": {description: Setting saved.}
        "400": {description: Missing value.}

  # --- WebSocket feeds (spec §11, §13.5) ---------------------------------------
  # These are NOT REST endpoints: each is an HTTP GET that upgrades to a
  # WebSocket at the host root (outside the /api/v1 base). Authentication is
  # an access token in the ?token= query parameter or the session cookie.
  # Messages are JSON {type, data, sent_at}. Every feed has a REST catch-up
  # path, so clients reconnect and resync via REST (§31.27).
  /ws/guide:
    get:
      operationId: wsGuide
      summary: "WEBSOCKET UPGRADE — the caller's own dispatch offer feed. Message types: dispatch.offer (offer + booking summary + expires_at; also pushed as connect snapshot), dispatch.offer_resolved. Catch-up via GET /api/v1/me/guide/offers. 401 without a valid token."
      x-websocket: true
      responses:
        "101": {description: WebSocket upgrade.}
        "401": {description: Unauthenticated.}
  /ws/booking/{id}:
    get:
      operationId: wsBooking
      summary: "WEBSOCKET UPGRADE — one booking's feed for the owning tourist, assigned guide or bookings.read. Message types: booking.updated (status/assignment changes; connect snapshot), location.update (guide position during the active window only, §11.2) and sos.triggered (§12). Catch-up via GET /api/v1/bookings/{id} and GET /api/v1/bookings/{id}/location. 403 for anyone else."
      x-websocket: true
      responses:
        "101": {description: WebSocket upgrade.}
        "401": {description: Unauthenticated.}
        "403": {description: Booking not visible to the caller.}
  /ws/admin/operations:
    get:
      operationId: wsAdminOperations
      summary: "WEBSOCKET UPGRADE — operations feed for all active tours (permission dispatch.manage, §11.2). Message types: dispatch.batch, dispatch.unmatched, dispatch.batch_expired, booking.updated, location.update, sos.triggered, incident.updated."
      x-websocket: true
      responses:
        "101": {description: WebSocket upgrade.}
        "401": {description: Unauthenticated.}
        "403": {description: Missing dispatch.manage.}
`
