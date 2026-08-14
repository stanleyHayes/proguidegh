**GUIDE GHANA**

*"ADAPT ON THE GO"*

Certified Tourist Guide Supply System

End-to-End Product Requirements, System Design, Architecture, Delivery
Plan & AI Agent Build Instructions

**Prepared for: ADAPT Africa + Ghana Tourism Authority + Swedish
Chamber\**
Target launch: Accra, Cape Coast and Kumasi • 90-day V1 delivery target\
Document version: 1.0 • August 2026

  -----------------------------------------------------------------------
  **PRIMARY PURPOSE\**
  This document is intentionally written so a capable AI coding agent can
  build Guide Ghana from an empty repository through production
  deployment without relying on the original conversation. Where a detail
  is not specified, the agent must follow the decision rules and defaults
  stated in this document rather than inventing a new architecture.
  -----------------------------------------------------------------------

  -----------------------------------------------------------------------

# 1. Agent Operating Contract

This section is authoritative. An AI coding agent must read it before
generating code. If a later section conflicts with this section, the
safer and simpler production-ready interpretation wins.

## 1.1 Build objective

Build a secure, auditable, production-ready marketplace that allows
tourists to discover and book certified guides, allows guides to
receive/perform paid assignments and grow professionally, and gives
authorized administrators real-time operational, financial, quality and
safety oversight.

## 1.2 Non-negotiable engineering decisions

- Use a modular monolith for V1. Do not introduce Kubernetes or
  independently deployed domain microservices unless a later scaling
  decision explicitly requires it.

- Use PostgreSQL as the source of truth for users, guides, bookings,
  tours, reviews, payments, payouts, certification, commissions, tourism
  levies, audit references and all financial state.

- MongoDB is optional and secondary. Only use it for clearly
  semi-structured data such as learning content, dynamic form payloads
  or integration payload archives. The core system must work without
  MongoDB.

- Use Redis for ephemeral/high-frequency state: online status, current
  guide location, WebSocket presence, distributed locks, rate-limits,
  short-lived dispatch offers and cached queries.

- Use an immutable ledger model for money movements. Never compute a
  wallet balance only from mutable booking/payment records.

- Never store raw card data. All card/MoMo collection must be handled by
  the selected payment provider using provider-hosted/tokenized flows.

- All sensitive documents live in private object storage and are
  accessed through short-lived signed URLs.

- Every privileged or financially significant action must be auditable.

- Every API mutation that may be retried by mobile clients or webhooks
  must support idempotency.

- Build the system in phases and keep each phase deployable and
  testable.

## 1.3 Definition of project completion

- Tourist can register/login, search, book, pay, track an active tour,
  receive a receipt and submit a verified review.

- Guide can apply, complete verification/certification workflow, go
  available, receive/accept jobs, conduct a tour, view earnings and
  receive eligible payouts.

- Admin can approve/disable guides, manage certifications, monitor
  active tours, see live guide locations, manage incidents, inspect
  transactions, reconcile payouts/levies, manage pricing and view
  analytics.

- Payment and webhook flows survive retries without duplicate bookings,
  ledger postings or payouts.

- Role-based access, audit logging, observability, backups, CI/CD,
  staging and production deployments are in place.

- Critical journeys have automated tests and documented operational
  runbooks.

# 2. Product Context & Success Metrics

## 2.1 Problems to solve

  -----------------------------------------------------------------------
  **Problem**                         **Platform response**
  ----------------------------------- -----------------------------------
  Guide knowledge gaps                Certification, structured training,
                                      specialist/language profiles,
                                      review quality loop.

  Unprofessional service              Standard pricing, verified
                                      profiles, receipts, ratings,
                                      uniforms/certification records and
                                      accountability.

  Tourist safety                      Identity/background checks,
                                      active-tour tracking, SOS workflow,
                                      insurance evidence and admin
                                      incident response.

  Youth unemployment                  A transparent marketplace and
                                      dispatch engine matching certified
                                      guides to paid tours.

  Poor tourism operational data       Aggregated real-time dashboards for
                                      tour demand, guide supply,
                                      locations, revenues and quality.
  -----------------------------------------------------------------------

## 2.2 Year-one business targets

  -----------------------------------------------------------------------
  **Metric**              **Baseline / stated     **Y1 target**
                          value**                 
  ----------------------- ----------------------- -----------------------
  Certified guides        2,140                   5,000

  Tours per month         8,420                   20,000

  Revenue to youth        GHS 3.8M                GHS 15M

  Average tourist rating  4.8/5                   4.9/5

  Gross bookings          \-                      GHS 25M
  -----------------------------------------------------------------------

## 2.3 Core product KPIs

- Search-to-book conversion rate

- Payment success rate

- Median time to guide acceptance

- Tour completion rate

- Cancellation/refund rate

- Active guides by region/time block

- Average rating and rating distribution

- Repeat tourist booking rate

- Guide utilization and earnings distribution

- SOS/incidents per 1,000 tours

- Payout success/retry rate

- Platform revenue and Tourism Levy accrual/reconciliation

# 3. Personas, Roles & Permissions

  -----------------------------------------------------------------------
  **Role**                            **Primary capabilities**
  ----------------------------------- -----------------------------------
  Tourist                             Account/profile, search, booking,
                                      payment, active-tour tracking, SOS,
                                      receipts, reviews, support.

  Guide Applicant                     Application, document upload,
                                      training/certification status,
                                      profile completion.

  Certified Guide                     Availability, job feed,
                                      accept/decline, tour operations,
                                      navigation links, earnings/wallet,
                                      courses.

  Elite Guide                         Certified Guide capabilities plus
                                      eligibility for premium
                                      assignments/rate rules.

  Operations Agent                    Monitor tours, intervene in
                                      dispatch, assist cancellations,
                                      incident management.

  Verifier / Certification Officer    Review
                                      identity/background/certification
                                      evidence and approve/reject stages.

  Finance Officer                     Payments, refunds, ledger, payout
                                      batches, levy reconciliation,
                                      reports.

  Content/Training Admin              Courses, lessons, exams, tags,
                                      tourism specialties/content.

  Administrator                       Operational configuration, users,
                                      guides, tours, pricing and
                                      dashboards.

  Super Admin                         Role administration, system
                                      configuration, financial overrides,
                                      integrations and audit access.

  Hotel/B2B User - Phase 2            Corporate account, priority booking
                                      pool, invoices, subscription and
                                      reports.
  -----------------------------------------------------------------------

  -----------------------------------------------------------------------
  **RBAC RULE\**
  Permissions, not UI visibility, enforce authorization. Every backend
  handler must call the authorization layer. Frontend guards are
  convenience only.
  -----------------------------------------------------------------------

  -----------------------------------------------------------------------

# 4. Functional Scope by Module

## 4.1 Tourist application

- Email/phone signup and OTP verification; social login can be added
  later.

- Profile with name, nationality, phone, language preference, emergency
  contact and optional accessibility needs.

- Location/specialty/language/rating/availability search.

- Tour package discovery and price quote before booking.

- Matched guide results showing verified badges, rating, languages,
  specialty, number of completed tours and availability.

- Booking with date/time, pickup/meeting point, duration/package, number
  of guests and notes.

- Card and Ghana MoMo payment through payment provider.

- Booking status timeline and notifications.

- Active-tour map, guide position, tour start/end status and SOS button.

- Receipts downloadable as PDF.

- Verified post-tour rating: 1--5 stars, text, selectable quality tags.

- Booking history, receipts, cancellations/refunds and support.

## 4.2 Guide application

- Application and identity profile.

- Upload required verification documents and profile image.

- Track certification pipeline stage and outstanding requirements.

- Guide dashboard: status, availability, earnings, upcoming tours, new
  jobs.

- Go online/offline with location permission explanation.

- Receive nearby eligible tour offers and accept in one action.

- Conflict prevention: a guide cannot accept overlapping tours.

- Tour lifecycle: travel-to-meeting, arrived, start, active, complete;
  operations override requires reason.

- Wallet: available, pending, paid, adjustments and statement.

- Weekly MoMo payout default; payout account verification required.

- Upskill courses, progress, assessment results and badges.

- Rating trends and retraining status.

- Elite status eligibility when rating/volume/compliance criteria are
  met.

## 4.3 Admin command panel

- KPI cards for certified guides, tours, youth earnings and tourist
  rating.

- Live Ghana map showing active/available/on-tour guides with filters by
  region/status.

- Active tour operations board.

- Recent tours table with booking, guide, tourist, location, status,
  amount and timestamps.

- Guide application/certification review queues.

- Security/incident dashboard with SOS events, severity, assignment and
  resolution.

- Financial dashboard with collections, fees, levy accrual, refunds,
  guide payable, payout batches and reconciliation.

- Quality dashboard: ratings, complaints, low-rated guides, retraining
  queue.

- Configuration for tour packages, base pricing, commission, levy,
  cancellation windows and payout delay.

- User/role/permission administration.

- Audit log viewer with filters/export.

- Reports and CSV export with permissions.

## 4.4 Review & rating system

Only a completed, verified booking can create one review. A review must
reference booking_id, tourist_id and guide_id. Editing can be allowed
for a short policy-defined window, but original values must remain
auditable. Rating below 4.0 rolling threshold creates a quality flag;
rating above 4.8 plus configurable minimum completed tours may qualify a
guide for Elite review.

## 4.5 Payment, receipt & payout system

- Payment is initiated before booking becomes confirmed.

- Provider webhook is authoritative for asynchronous payment
  success/failure.

- Generate human-readable receipt reference such as GG-88291 plus UUID
  primary identifier.

- Receipt shows tour, guide, gross amount, currency, payment method,
  insurance indicator and transaction reference.

- Default financial rule from concept: 15% platform fee and 3% Tourism
  Levy. Both must be configurable effective-dated rules, not hard-coded
  constants.

- Guide payable becomes pending after collection and eligible according
  to payout-delay policy after completion.

- Default payout policy: weekly/T+7 to verified MoMo payout account.

- Refunds and disputes post reversing ledger entries; never delete
  original ledger entries.

# 5. Certification & Trust Pipeline

Implement certification as an explicit state machine. Administrative
transitions require the appropriate permission, evidence references and
an audit entry.

> APPLIED -\> IDENTITY_PENDING -\> IDENTITY_VERIFIED -\>
> BACKGROUND_CHECK_PENDING -\> BACKGROUND_VERIFIED -\> TRAINING -\>
> EXAM_PENDING -\> CERTIFIED -\> INSURANCE_ACTIVE -\> ACTIVE

Terminal/exception states include REJECTED, SUSPENDED, EXPIRED and
REQUIRES_RETRAINING. Reactivation must preserve historical certification
records.

  -----------------------------------------------------------------------
  **Stage**               **Required evidence /   **Owner**
                          action**                
  ----------------------- ----------------------- -----------------------
  APPLIED                 Application and         Applicant
                          required profile fields 

  IDENTITY_VERIFIED       Approved ID evidence    Verifier

  BACKGROUND_VERIFIED     Approved Police/other   Verifier
                          background evidence     

  TRAINING                Required                Training team
                          modules/enrollment      

  CERTIFIED               Passed examination;     Certification officer
                          certificate             
                          number/expiry           

  INSURANCE_ACTIVE        Policy evidence +       Operations
                          validity period         

  ACTIVE                  All mandatory controls  System/Admin
                          valid                   
  -----------------------------------------------------------------------

# 6. V1 Architecture

V1 uses a modular monolith to minimize operational overhead while
preserving strong internal domain boundaries. The Go application can
later extract modules into separate services if measured scale requires
it.

> Clients\
> Tourist Web/PWA (Next.js)\
> Guide Web/PWA (Next.js; native wrapper/mobile app later if needed)\
> Admin Portal (Next.js)\
> \| HTTPS / WebSocket\
> Cloudflare DNS/WAF/CDN\
> \|\
> Vercel (Next.js frontends)\
> \|\
> Render: Go API + Worker\
> \|\-\-\-- PostgreSQL (system of record)\
> \|\-\-\-- Redis / Upstash (ephemeral/realtime/cache/locks)\
> \|\-\-\-- MongoDB Atlas (optional semi-structured data only)\
> \|\-\-\-- Cloudflare R2 (private objects)\
> \|\-\-\-- Resend (email)\
> \|\-\-\-- SMS provider\
> \|\-\-\-- FCM (push)\
> \|\-\-\-- Paystack/Hubtel (payments/payouts)\
> \|\-\-\-- Google Maps Platform\
> \|\-\-\-- Sentry + OpenTelemetry

## 6.1 Runtime components

  -----------------------------------------------------------------------
  **Component**           **Technology**          **Responsibility**
  ----------------------- ----------------------- -----------------------
  Web apps                Next.js 16 + TypeScript Tourist, guide and
                                                  admin web experiences.

  API                     Go                      REST API, auth, domain
                                                  logic, WebSockets,
                                                  provider callbacks.

  Worker                  Go on Render            Emails, push, PDF jobs,
                                                  webhook retries, payout
                                                  batches, scheduled
                                                  quality/expiry checks.

  PostgreSQL              Managed PostgreSQL      Durable transactional
                                                  state and ledger.

  Redis                   Upstash Redis           Presence, live
                                                  coordinates, locks,
                                                  idempotency cache, rate
                                                  limits, offer TTLs.

  MongoDB                 MongoDB Atlas optional  Semi-structured
                                                  learning/integration
                                                  payloads if justified.

  Object storage          Cloudflare R2           Guide files,
                                                  certificates, photos,
                                                  generated receipts.

  Maps                    Google Maps Platform    Map tiles, geocoding,
                                                  route/ETA as required.

  Email                   Resend                  Transactional email.

  Payments                Paystack primary /      Collection, MoMo/card,
                          Hubtel adapter          transfers; use adapter
                                                  interface.

  Push                    Firebase Cloud          Mobile/PWA push alerts.
                          Messaging               

  Observability           Sentry + OpenTelemetry  Errors, traces,
                                                  metrics/log
                                                  correlation.
  -----------------------------------------------------------------------

# 7. Repository & Code Organization

## 7.1 Recommended monorepo

> guide-ghana/\
> apps/\
> tourist-web/ \# Next.js\
> guide-web/ \# Next.js\
> admin-web/ \# Next.js\
> services/\
> api/ \# Go HTTP/WebSocket app\
> worker/ \# Go jobs/schedulers\
> packages/\
> ui/ \# shared React design system\
> contracts/ \# generated TS API types / OpenAPI client\
> config/ \# shared frontend configuration\
> infra/\
> render/\
> vercel/\
> cloudflare/\
> scripts/\
> docs/\
> architecture/\
> runbooks/\
> api/\
> .github/workflows/\
> Makefile\
> README.md

## 7.2 Go module layout

> services/api/\
> cmd/api/main.go\
> internal/\
> platform/{config,db,redis,httpx,auth,rbac,audit,events,storage,observability}\
> auth/\
> users/\
> tourists/\
> guides/\
> certification/\
> tours/\
> bookings/\
> dispatch/\
> tracking/\
> payments/\
> ledger/\
> payouts/\
> reviews/\
> training/\
> notifications/\
> incidents/\
> admin/\
> reporting/\
> migrations/\
> openapi/

Each domain module should expose application/service interfaces and keep
database/repository concerns behind interfaces. Do not create a generic
repository abstraction that hides SQL semantics. Prefer explicit SQL
queries and transactions.

# 8. Data Model

Use UUID primary keys unless a provider-specific natural key is
explicitly required. Include created_at and updated_at on mutable
entities. Use timestamptz. Use soft-deletion only for entities where
legal/audit retention requires preserving history; financial and audit
records are append-only.

## 8.1 Core tables

  ----------------------------------------------------------------------------------
  **Table**               **Important columns**    **Purpose**
  ----------------------- ------------------------ ---------------------------------
  users                   id, email, phone_e164,   Global identity record.
                          password_hash, status,   
                          last_login_at            

  user_roles              user_id, role_id         RBAC assignment.

  roles                   id, code, name           Role definitions.

  permissions             id, code                 Permission definitions.

  role_permissions        role_id, permission_id   RBAC mapping.

  tourist_profiles        user_id, full_name,      Tourist profile.
                          nationality,             
                          preferred_language,      
                          emergency_contact\_\*    

  guide_profiles          user_id, public_name,    Guide public/employment profile.
                          bio, rating_avg,         
                          rating_count,            
                          elite_status, region_id  

  guide_languages         guide_id, language_code, Searchable language skills.
                          proficiency              

  guide_specialties       guide_id, specialty_id   Searchable tourism specialty.

  guide_documents         id, guide_id, type,      Private verification evidence
                          object_key, status,      metadata.
                          expires_at               

  certification_cases     id, guide_id, status,    Certification state machine root.
                          assigned_to, opened_at,  
                          completed_at             

  certification_events    id, case_id,             Immutable workflow history.
                          from_status, to_status,  
                          actor_id, reason,        
                          evidence_ref             

  tour_packages           id, code, name,          Standardized tour catalog.
                          duration_minutes,        
                          base_price, currency,    
                          active                   

  pricing_rules           id, package_id,          Effective-dated prices.
                          region_id, amount,       
                          effective_from,          
                          effective_to             

  bookings                id, reference,           Booking aggregate.
                          tourist_id, guide_id,    
                          package_id, starts_at,   
                          ends_at, status,         
                          meeting\_\*              

  booking_status_events   id, booking_id,          Immutable booking state history.
                          from_status, to_status,  
                          actor_id, metadata       

  payments                id, booking_id,          Payment attempts/results.
                          provider,                
                          provider_reference,      
                          amount, currency,        
                          status, paid_at          

  refunds                 id, payment_id,          Refund attempts/results.
                          provider_reference,      
                          amount, status, reason   

  ledger_accounts         id, owner_type,          Logical accounting accounts.
                          owner_id, currency, code 

  ledger_transactions     id, reference, type,     Immutable transaction header.
                          booking_id, occurred_at  

  ledger_entries          id, transaction_id,      Balanced debit/credit entries.
                          account_id, direction,   
                          amount                   

  payout_accounts         id, guide_id, provider,  Guide payout destination
                          network,                 metadata.
                          account_ref_tokenized,   
                          verified_at              

  payouts                 id, guide_id, amount,    Payout lifecycle.
                          currency, status,        
                          provider_reference,      
                          scheduled_for            

  reviews                 id, booking_id UNIQUE,   Verified review.
                          tourist_id, guide_id,    
                          rating, body, created_at 

  review_tags             review_id, tag_id        Knowledgeable/Punctual/Friendly
                                                   etc.

  incidents               id, booking_id, type,    Safety/support incident.
                          severity, status,        
                          reported_by,             
                          assigned_to, occurred_at 

  sos_events              id, booking_id, user_id, SOS evidence.
                          latitude, longitude,     
                          accuracy, triggered_at,  
                          acknowledged_at          

  notifications           id, user_id, channel,    Notification delivery history.
                          template, status,        
                          provider_reference       

  audit_logs              id, actor_id, action,    Privileged/action audit.
                          entity_type, entity_id,  
                          before_json, after_json, 
                          ip, created_at           

  idempotency_keys        key, scope,              Mutation replay protection.
                          response_code,           
                          response_body_hash,      
                          expires_at               

  regions                 id, code, name           Ghana regions.

  specialties             id, code, name           Guide specialties.

  system_settings         key, value_json, version Controlled configuration.
  ----------------------------------------------------------------------------------

## 8.2 Booking state machine

> DRAFT -\> PAYMENT_PENDING -\> CONFIRMED -\> GUIDE_EN_ROUTE -\>
> GUIDE_ARRIVED -\> IN_PROGRESS -\> COMPLETED
>
> Exceptional: PAYMENT_FAILED, CANCELLED_BY_TOURIST, CANCELLED_BY_GUIDE,
> CANCELLED_BY_ADMIN, NO_SHOW, REFUND_PENDING, REFUNDED

Every transition is validated in a single domain service. Do not allow
arbitrary status writes from controllers or admin forms.

## 8.3 Payment state machine

> CREATED -\> PENDING -\> SUCCEEDED \| FAILED \| EXPIRED -\>
> REFUND_PENDING -\> PARTIALLY_REFUNDED \| REFUNDED

## 8.4 Payout state machine

> PENDING_ELIGIBILITY -\> ELIGIBLE -\> QUEUED -\> PROCESSING -\> PAID \|
> FAILED -\> RETRY_QUEUED \| MANUAL_REVIEW

# 9. Ledger & Financial Rules

Financial correctness is a first-class requirement. Store monetary
amounts as integer minor units where provider/currency semantics permit;
otherwise use PostgreSQL NUMERIC with explicit scale. Never use
floating-point for money.

## 9.1 Example booking allocation

For a GHS 450 booking using the concept defaults: platform fee 15% = GHS
67.50, Tourism Levy 3% = GHS 13.50, and guide gross payable before other
policy deductions = GHS 369.00. Gateway processing fees should be
recorded separately according to who bears the fee.

## 9.2 Ledger invariants

- Every ledger transaction balances.

- Posted entries are immutable.

- Corrections are made with reversal/adjustment transactions.

- Provider reference uniqueness prevents duplicate postings.

- Booking completion moves guide payable from pending to eligible
  according to payout policy.

- Refunds reduce the correct revenue/levy/payable accounts based on
  policy.

- Finance exports can reconcile provider settlements to internal ledger
  totals.

# 10. Search, Matching & Dispatch

## 10.1 Search filters

- Region/city or coordinates/radius

- Date/time availability

- Guide language/proficiency

- Specialty

- Minimum rating

- Elite status

- Package eligibility

- Optional accessibility/special requirement tags

## 10.2 Availability rules

A guide is searchable only if ACTIVE certification status, mandatory
documents not expired, account not suspended, available for the
requested period and not already assigned to an overlapping booking. Use
PostgreSQL constraints/transactional checks to prevent double booking.

## 10.3 Dispatch algorithm - V1

1.  Filter to eligible, available guides within configured radius or
    region.

2.  Calculate score from distance/ETA, rating, specialty match, language
    match, recent workload fairness and acceptance reliability.

3.  Offer to the best N candidates in a small batch with an expiry (for
    example 20--45 seconds) stored in Redis.

4.  First valid acceptance wins using a database transaction/distributed
    lock. Expire other offers.

5.  If no acceptance, expand radius/batch and notify operations after
    configured attempts.

Do not use ML in V1. Persist scoring features/outcomes so a future model
can be evaluated offline.

# 11. Live Location & Realtime Architecture

Guide devices should send compact location updates directly to Guide
Ghana. Do not reverse-geocode every ping through Google Maps.

> Guide client -\> authenticated WebSocket/HTTPS location update -\> Go
> realtime service -\> Redis current-location key -\> Tourist/Admin
> WebSocket subscription

## 11.1 Location payload

> {\
> \"booking_id\": \"uuid\",\
> \"latitude\": 5.6037,\
> \"longitude\": -0.1870,\
> \"accuracy_m\": 8.5,\
> \"heading\": 120,\
> \"speed_mps\": 3.1,\
> \"captured_at\": \"RFC3339 timestamp\"\
> }

## 11.2 Privacy and retention

- Only collect guide live location when needed for availability/active
  operational purpose and according to disclosed policy.

- Tourist receives access only to the assigned guide for the relevant
  booking window.

- Admin live map requires authorized operations permission.

- Keep high-frequency coordinates in Redis with TTL; persist coarse
  checkpoints/events needed for safety/audit according to retention
  policy.

- Do not expose guide home/private historical movements to tourists.

# 12. SOS & Incident Management

6.  User triggers SOS from active booking.

7.  Client immediately sends booking/user ID and freshest coordinates;
    retry if network unavailable.

8.  API creates immutable SOS event and HIGH/CRITICAL incident.

9.  Operations dashboard receives realtime alert; assigned responder
    acknowledges.

10. Configured channels send fallback SMS/push/email to authorized
    responders/emergency contacts as policy permits.

11. Every acknowledgement, note, escalation and closure is timestamped
    and audited.

  -----------------------------------------------------------------------
  **SAFETY REQUIREMENT\**
  Do not promise automatic police/emergency dispatch unless a formal
  integration and operating agreement exists. The software must clearly
  distinguish "SOS sent to Guide Ghana operations" from any external
  emergency-service response.
  -----------------------------------------------------------------------

  -----------------------------------------------------------------------

# 13. API Specification

Expose REST JSON under /api/v1 and WebSocket endpoints under /ws.
Generate and commit an OpenAPI specification. Frontends consume
generated TypeScript contracts rather than hand-maintained duplicate
types.

## 13.1 Authentication

  ------------------------------------------------------------------------------
  **Method**              **Path**                       **Purpose**
  ----------------------- ------------------------------ -----------------------
  POST                    /api/v1/auth/register          Create account.

  POST                    /api/v1/auth/login             Credential login.

  POST                    /api/v1/auth/otp/request       Request phone/email
                                                         OTP.

  POST                    /api/v1/auth/otp/verify        Verify OTP.

  POST                    /api/v1/auth/refresh           Rotate refresh token.

  POST                    /api/v1/auth/logout            Revoke session.

  POST                    /api/v1/auth/password/forgot   Begin reset.

  POST                    /api/v1/auth/password/reset    Complete reset.
  ------------------------------------------------------------------------------

## 13.2 Tourist & search

  ----------------------------------------------------------------------------
  **Method**              **Path**                     **Purpose**
  ----------------------- ---------------------------- -----------------------
  GET                     /api/v1/tour-packages        List active
                                                       packages/prices.

  GET                     /api/v1/guides/search        Search eligible guides.

  GET                     /api/v1/guides/{id}          Public guide detail.

  GET                     /api/v1/me/tourist-profile   Current profile.

  PATCH                   /api/v1/me/tourist-profile   Update profile.
  ----------------------------------------------------------------------------

## 13.3 Booking & payment

  ---------------------------------------------------------------------------------------------
  **Method**              **Path**                               **Purpose**
  ----------------------- -------------------------------------- ------------------------------
  POST                    /api/v1/bookings/quote                 Return server-authoritative
                                                                 quote.

  POST                    /api/v1/bookings                       Create booking/payment-pending
                                                                 record; idempotent.

  GET                     /api/v1/bookings/{id}                  Booking detail.

  POST                    /api/v1/bookings/{id}/payment-intent   Initialize provider payment.

  POST                    /api/v1/webhooks/payments/{provider}   Signed provider callback.

  POST                    /api/v1/bookings/{id}/cancel           Request cancellation.

  GET                     /api/v1/bookings/{id}/receipt          Receipt metadata/signed
                                                                 download.

  POST                    /api/v1/payments/{id}/refund           Privileged/policy-controlled
                                                                 refund.
  ---------------------------------------------------------------------------------------------

## 13.4 Guide & dispatch

  --------------------------------------------------------------------------------
  **Method**              **Path**                         **Purpose**
  ----------------------- -------------------------------- -----------------------
  POST                    /api/v1/guides/apply             Create guide
                                                           application.

  POST                    /api/v1/guides/documents         Obtain upload
                                                           URL/register document.

  GET                     /api/v1/me/guide                 Guide
                                                           dashboard/profile.

  POST                    /api/v1/me/guide/availability    Go online/offline and
                                                           set availability.

  GET                     /api/v1/me/guide/offers          Current job offers.

  POST                    /api/v1/offers/{id}/accept       Atomically accept
                                                           offer.

  POST                    /api/v1/offers/{id}/decline      Decline offer.

  POST                    /api/v1/bookings/{id}/arrived    Guide arrived.

  POST                    /api/v1/bookings/{id}/start      Start tour.

  POST                    /api/v1/bookings/{id}/complete   Complete tour.

  GET                     /api/v1/me/guide/wallet          Wallet summary.

  GET                     /api/v1/me/guide/statement       Ledger-derived
                                                           statement.
  --------------------------------------------------------------------------------

## 13.5 Reviews, realtime and safety

  --------------------------------------------------------------------------------
  **Method**              **Path**                         **Purpose**
  ----------------------- -------------------------------- -----------------------
  POST                    /api/v1/bookings/{id}/review     One verified review per
                                                           completed booking.

  GET                     /api/v1/guides/{id}/reviews      Public reviews.

  POST                    /api/v1/bookings/{id}/sos        Create SOS event.

  POST                    /api/v1/bookings/{id}/location   Fallback HTTPS location
                                                           update.

  WS                      /ws/guide                        Guide offers/location
                                                           channel.

  WS                      /ws/booking/{id}                 Tourist active-booking
                                                           channel.

  WS                      /ws/admin/operations             Authorized admin
                                                           operations feed.
  --------------------------------------------------------------------------------

## 13.6 Admin

Admin endpoints live under /api/v1/admin and must have explicit
permission checks. Include guide/certification queues, users, pricing,
bookings, incidents, payouts, refunds, analytics, exports, roles,
permissions, configuration and audit logs.

# 14. API Engineering Standards

- Use RFC 3339 timestamps in JSON.

- Use consistent error envelope: code, message, details, request_id.

- Use cursor pagination for high-volume event/history tables; offset
  pagination is acceptable for low-volume admin lists.

- Validate request payloads at transport boundary and domain invariants
  in service layer.

- Use Idempotency-Key for booking creation, payment initiation, refund
  and other replay-sensitive mutations.

- Use provider webhook signature verification and retain a hash/raw
  payload archive as policy permits.

- Every response includes/request logs correlate a request_id.

- Avoid exposing internal database IDs or sensitive provider tokens
  where not needed.

- Return server-authoritative calculated price; never trust
  client-supplied totals.

# 15. Authentication & Security

## 15.1 Session model

Use short-lived access tokens and rotating refresh tokens stored in
secure, HttpOnly, SameSite cookies for browser clients where
architecture permits. Persist refresh-session identifiers/revocation
state. For future native mobile clients, use secure OS credential
storage and the same backend session model.

## 15.2 Password and account protections

- Argon2id or bcrypt with strong current parameters.

- Rate-limit login, OTP, password reset, payment initiation and SOS
  abuse vectors.

- MFA required for Super Admin and finance roles; strongly recommended
  for all privileged staff.

- Step-up authentication for sensitive role, payout-account and refund
  actions.

- Admin sessions have shorter idle timeout than tourist sessions.

- Suspend/revoke sessions on account compromise or role removal.

## 15.3 Data security

- TLS everywhere.

- Secrets only in environment/secret manager; never commit secrets.

- Private R2 buckets and short-lived signed URLs.

- Field-level encryption/tokenization for sensitive payout identifiers
  when applicable.

- Separate staging and production databases, provider keys and storage
  buckets.

- Principle of least privilege for database/service credentials.

- Regular dependency and container vulnerability scans.

- Content Security Policy and secure HTTP headers on web apps.

# 16. External Integrations

## 16.1 Payment adapter

> type PaymentProvider interface {\
> InitializePayment(ctx context.Context, req InitializePaymentRequest)
> (\... )\
> VerifyPayment(ctx context.Context, providerRef string) (\... )\
> Refund(ctx context.Context, req RefundRequest) (\... )\
> CreateTransfer(ctx context.Context, req TransferRequest) (\... )\
> VerifyWebhook(headers http.Header, rawBody \[\]byte) error\
> }

Implement one provider first (recommended Paystack for the initial build
based on the selected stack), then add Hubtel through the same interface
if commercial/operational requirements demand it.

## 16.2 Maps adapter

Wrap geocoding, route/ETA and place lookup behind an interface so
cost/rate-limits can be controlled. Cache stable geocoding results.
Apply daily quotas and alerting.

## 16.3 Messaging

Use Resend for email. Use an SMS provider adapter for OTP/critical
fallback. Use Firebase Cloud Messaging for push. Notification templates
must be versioned/configurable and record delivery state.

## 16.4 Object storage

Use presigned upload/download URLs. Validate MIME type, extension,
declared category and maximum size. Scan files if feasible before
marking verification documents usable. Object keys must not contain raw
personal data.

# 17. Receipt & PDF Generation

Generate receipts server-side after confirmed payment. Receipt number
must be unique and immutable. Store generated PDF in private R2 and
expose a short-lived signed download URL.

- Guide Ghana logo/name and receipt reference

- Booking/tour package and date/time

- Tourist/guide display names as policy permits

- Gross amount and currency

- Payment method and provider transaction reference

- Platform/levy breakdown where policy requires

- Insurance Covered badge only when coverage is actually active for the
  booking

- Issue timestamp and support/contact instructions

# 18. Frontend Applications & Screens

## 18.1 Tourist web/PWA routes

- / - landing/home

- /search - guide/package search

- /guides/\[id\] - guide profile

- /checkout/\[bookingId\] - payment/confirmation

- /bookings - history

- /bookings/\[id\] - status/active-tour view

- /bookings/\[id\]/review - review form

- /receipts/\[id\] - receipt view/download

- /profile - user profile

- /support - support/incidents

## 18.2 Guide web/PWA routes

- /guide - dashboard

- /guide/apply - application

- /guide/verification - pipeline/documents

- /guide/jobs - live job feed

- /guide/tours - scheduled/history

- /guide/tours/\[id\] - tour operations

- /guide/wallet - wallet/statement

- /guide/payouts - payouts/account

- /guide/training - courses

- /guide/profile - public profile and credentials

## 18.3 Admin routes

- /admin - command center

- /admin/map - live guide/tour map

- /admin/guides - guide directory

- /admin/certification - verification queues

- /admin/tours - tour operations

- /admin/incidents - safety desk

- /admin/finance - revenue/levy dashboard

- /admin/payouts - payout batches

- /admin/reviews - quality/retraining

- /admin/training - learning content

- /admin/pricing - packages/rules

- /admin/users - users/RBAC

- /admin/audit - audit logs

- /admin/settings - integrations/policy configuration

## 18.4 Design requirements

- Mobile-first tourist and guide applications.

- Admin optimized for desktop but responsive.

- Accessible color contrast, focus states, form labels and keyboard
  navigation.

- Loading/skeleton, empty, offline, retry and error states designed
  explicitly.

- Map is never the sole representation of operational state; provide a
  tabular/list fallback.

- Show confirmation dialogs for destructive/financial/admin actions.

- Use shared design tokens/components from packages/ui.

# 19. Training & Upskill Module

Phase 1 may implement a light LMS inside the core platform; do not
overbuild a SCORM platform unless required.

- Courses with title, description, category, language,
  required/optional, cover image.

- Modules/lessons with text/video/link/PDF.

- Enrollment and progress.

- Quiz/assessment with pass threshold.

- Completion certificate/badge.

- Mandatory training blocks guide activation when configured.

- Examples: Mandarin for Guides, First Aid, Ghana History, Hospitality
  Standards.

- Admin reporting for completion and scores.

# 20. Notifications Matrix

  ------------------------------------------------------------------------------------
  **Event**              **Tourist**       **Guide**            **Admin/Operations**
  ---------------------- ----------------- -------------------- ----------------------
  Booking confirmed      Email + push/SMS  Push + email         Operational feed
                         optional                               

  New job offer          \-                Push +               Dispatch monitor
                                           in-app/WebSocket     

  Guide accepted         Push/in-app       In-app               Operations feed

  Tour starting soon     Push              Push                 Optional

  SOS                    Immediate in-app  If                   Critical realtime +
                         confirmation      tourist-triggered,   fallback channels
                                           guide notification   
                                           by policy            

  Tour completed         Review prompt     Earnings update      Operations feed

  Payout paid            \-                Push + email/SMS     Finance log

  Certification status   \-                Push + email         Queue update

  Document/certificate   \-                Push + email         Compliance queue
  expiry                                                        
  ------------------------------------------------------------------------------------

# 21. Background Jobs & Schedulers

- Expire stale dispatch offers.

- Booking reminders and no-response escalations.

- Guide document/certification expiry warnings and deactivation.

- Review quality aggregation and retraining flag evaluation.

- Generate receipts/certificates asynchronously where appropriate.

- Payment reconciliation/verification for uncertain callbacks.

- Payout eligibility calculation and weekly payout batch creation.

- Retry failed notifications/webhooks/transfers with bounded exponential
  backoff.

- Archive/aggregate location checkpoints according to retention policy.

- Daily operational/financial metric rollups.

Use an explicit jobs table or reliable queue strategy for durable work.
Redis Pub/Sub alone is not a durable job queue. Every job handler must
be idempotent.

# 22. Observability & Operations

## 22.1 Logs

- Structured JSON logs in Go.

- Include request_id, user_id when safe, booking_id, provider_reference
  and job_id correlation fields.

- Never log passwords, raw authorization headers, full verification
  documents or sensitive payment tokens.

- Audit log is separate from diagnostic logs.

## 22.2 Metrics

- HTTP request rate/error/latency

- WebSocket connections/reconnects

- Booking created/confirmed/completed/cancelled

- Payment successes/failures by provider/channel

- Dispatch offer acceptance time

- Active available guides

- SOS count/acknowledgement latency

- Payout success/failure/retry

- Notification success/failure

- DB pool saturation and slow queries

- Redis errors/memory/command volume

## 22.3 Alerts

- Payment/webhook failure spike

- Booking confirmation mismatch

- Payout failure spike

- SOS alert delivery failure

- API 5xx or latency threshold breach

- Database/Redis unavailable or connection saturation

- Storage upload failure

- Provider quota/rate limit nearing threshold

# 23. Testing Strategy

## 23.1 Unit tests

- Pricing calculations and effective-dated configuration

- Booking/payment/certification/payout state-machine transition rules

- Dispatch scoring/eligibility

- Ledger balancing and allocations

- Cancellation/refund policies

- RBAC permission decisions

- Review/Elite/retraining rules

## 23.2 Integration tests

- PostgreSQL repositories against real disposable Postgres

- Redis lock/TTL and realtime state behavior

- Provider adapters using mock HTTP servers/fixtures

- Webhook signature and idempotency handling

- Object storage signing abstraction

- Background job retries

## 23.3 End-to-end tests

- Tourist register -\> book -\> provider success webhook -\> confirmed
  booking

- Guide becomes available -\> receives offer -\> accepts -\> tour
  lifecycle -\> completion

- Completed booking -\> receipt -\> verified review

- Completed/eligible earning -\> payout batch -\> provider success -\>
  wallet statement

- SOS -\> admin incident appears/acknowledged

- Low rating -\> quality flag/retraining queue

- Expired certification -\> guide no longer searchable/dispatchable

## 23.4 Performance tests

Before production launch, load-test search, booking creation, webhook
bursts, active guide location updates and admin realtime subscriptions.
The 5,000-guide Y1 target does not require Kubernetes by itself; scale
Render instances/database only from measured bottlenecks.

# 24. CI/CD & Environments

## 24.1 Environments

  -----------------------------------------------------------------------
  **Environment**         **Purpose**             **Data rule**
  ----------------------- ----------------------- -----------------------
  Local                   Developer/agent work    Seeded synthetic data
                                                  only.

  CI                      Automated tests         Ephemeral databases.

  Staging                 Release                 No production PII.
                          verification/provider   
                          test mode               

  Production              Live platform           Strict
                                                  access/audit/backup
                                                  controls.
  -----------------------------------------------------------------------

## 24.2 Pull request pipeline

12. Formatting/linting: gofmt/go vet/staticcheck; ESLint/TypeScript.

13. Unit tests and coverage reporting.

14. Integration tests with disposable PostgreSQL/Redis.

15. Build Go binaries and Next.js apps.

16. Security/dependency scanning.

17. Validate OpenAPI generation/contracts and database migrations.

18. Preview deployments for frontend where appropriate.

## 24.3 Deployment

Vercel deploys Next.js apps. Render Blueprint/config deploys API and
worker. Database migrations run as a controlled pre-deploy/release step
and must be backwards-compatible for rolling deployment. Never auto-run
destructive migrations on production startup.

# 25. Environment Variables

> APP_ENV=local\|staging\|production\
> DATABASE_URL=\...\
> REDIS_URL=\...\
> JWT_OR_SESSION_SECRET=\...\
> R2_ENDPOINT=\...\
> R2_ACCESS_KEY_ID=\...\
> R2_SECRET_ACCESS_KEY=\...\
> R2_BUCKET_PRIVATE=\...\
> RESEND_API_KEY=\...\
> PAYMENT_PROVIDER=paystack\
> PAYSTACK_SECRET_KEY=\...\
> PAYSTACK_WEBHOOK_SECRET=\...\
> GOOGLE_MAPS_API_KEY_SERVER=\...\
> NEXT_PUBLIC_GOOGLE_MAPS_API_KEY=\... \# browser-restricted key only\
> FCM_PROJECT_ID=\...\
> SENTRY_DSN=\...\
> SMS_PROVIDER=\...\
> SMS_API_KEY=\...

Keep an .env.example containing names and safe descriptions only.
Production secrets are injected through platform secret settings.
Browser API keys must be domain/API restricted and never grant server
capabilities.

# 26. Database Migrations & Seed Data

- Use a versioned migration tool compatible with Go (for example
  golang-migrate or goose).

- Create extensions only when needed and supported by managed
  PostgreSQL.

- Seed static roles/permissions, Ghana regions, review tags, specialties
  and initial tour packages.

- Do not seed production users with known passwords.

- Add indexes based on query patterns: booking start/status/guide, guide
  status/region/rating, payment provider reference, payout status/date,
  audit entity/time.

- Consider PostGIS only if spatial query complexity justifies it; V1 can
  begin with latitude/longitude plus application distance/radius logic
  if sufficient.

# 27. Initial Pricing & Policy Configuration

  -----------------------------------------------------------------------
  **Configuration**       **Initial value**       **Implementation**
  ----------------------- ----------------------- -----------------------
  4-hour City Tour        GHS 250                 tour_packages +
                                                  pricing_rules

  8-hour Heritage Tour    GHS 450                 tour_packages +
                                                  pricing_rules

  24-hour Multi-region    GHS 900                 tour_packages +
                                                  pricing_rules

  Platform commission     15%                     effective-dated
                                                  financial rule

  Tourism Levy            3%                      effective-dated
                                                  financial rule

  Certification fee       GHS 300                 billable
                                                  item/configuration

  Hotel B2B               GHS 5,000/month         subscription
  subscription - Phase 2                          plan/configuration

  Guide payout delay      T+7 / weekly            payout policy

  Quality retraining      average \< 4.0          quality policy
  threshold                                       

  Elite rating threshold  average \> 4.8          quality policy plus
                                                  minimum tour count
  -----------------------------------------------------------------------

All initial values are business defaults from the concept, not immutable
code. Admin changes require permission and audit, and financial changes
are effective-dated.

# 28. 90-Day Delivery Roadmap

The roadmap below is the default execution order. The AI agent should
not start later phases before the foundation and acceptance tests of
earlier phases are green, except for isolated UI work that does not
create architecture divergence.

## Phase 0 - Days 1--5: Foundation

- Create monorepo and developer tooling.

- Bootstrap three Next.js apps and Go API/worker.

- Local Docker Compose for PostgreSQL and Redis.

- Configuration loader, structured logging, request IDs,
  health/readiness endpoints.

- Migration framework and initial schemas.

- OpenAPI baseline and generated frontend client.

- GitHub CI and staging deployment skeleton.

### Exit criteria

All applications build in CI; API connects to Postgres/Redis; migration
up/down process is tested; staging endpoints are reachable; no business
functionality yet.

## Phase 1 - Days 6--15: Identity, RBAC & Profiles

- Registration/login/OTP/password reset/session rotation.

- Users, roles, permissions and admin authorization middleware.

- Tourist profile.

- Guide application/profile shell and private document upload flow.

- Admin user/guide directory.

- Audit framework for privileged mutations.

### Exit criteria

Tourist and guide applicant accounts work; admin access is
permission-enforced; sensitive documents are private and signed;
automated auth/RBAC tests pass.

## Phase 2 - Days 16--27: Certification & Catalog

- Certification case/state machine and admin review queues.

- Document evidence/expiry model.

- Training shell or required-training flags.

- Regions, languages, specialties, tour packages and effective-dated
  pricing.

- Public guide profile visibility only for eligible status.

### Exit criteria

Admin can move a test guide through an audited certification process to
ACTIVE; only ACTIVE guides can appear publicly.

## Phase 3 - Days 28--40: Search, Booking & Availability

- Guide availability schedules/online state.

- Search filters/ranking and guide detail.

- Quote endpoint with server-authoritative pricing.

- Booking aggregate/state machine and overlap checks.

- Tourist checkout UX before payment integration.

### Exit criteria

A tourist can search, receive a quote and create a payment-pending
booking without double-booking a guide.

## Phase 4 - Days 41--52: Payments, Ledger & Receipts

- Paystack provider adapter/test-mode integration.

- Payment initialization and signed webhook handling.

- Idempotency protection.

- Ledger accounts/transactions/entries and booking allocation.

- Receipt generation and R2 storage.

- Refund skeleton/admin policy flow.

### Exit criteria

A test payment confirms exactly one booking, creates one balanced ledger
allocation and produces a downloadable receipt even if the provider
webhook is replayed multiple times.

## Phase 5 - Days 53--64: Dispatch, Realtime & Tour Operations

- Redis-backed guide online/presence/location.

- Dispatch offer/scoring/batch/TTL.

- Atomic accept and overlap prevention.

- WebSocket channels for guide/tourist/admin.

- Tour lifecycle: en route, arrived, start, complete.

- Admin live operations map/list.

### Exit criteria

A nearby ACTIVE guide can accept one offer, location streams to
authorized tourist/admin, and the booking completes through valid
transitions.

## Phase 6 - Days 65--72: Safety, Reviews & Quality

- SOS endpoint/realtime admin alert/fallback notifications.

- Incident dashboard/workflow.

- Verified review flow and tags.

- Rating aggregation, low-rating flags and Elite qualification review.

- Quality/retraining queue.

### Exit criteria

SOS reaches admin operations with coordinates and audit trail; only
completed bookings can review; quality thresholds create expected flags.

## Phase 7 - Days 73--80: Wallet, Payouts & Finance

- Guide ledger-derived wallet/statement.

- Payout account verification fields.

- Eligibility scheduler and weekly payout batch.

- Provider transfer integration or safe manual export fallback until
  production transfer credentials are approved.

- Retry/manual-review states and finance dashboard.

- Tourism Levy accrual/reconciliation reports.

### Exit criteria

Completed eligible earnings can be batched without duplicate payout;
provider retries are idempotent; finance can reconcile totals.

## Phase 8 - Days 81--86: Training, Analytics & Admin Polish

- Light LMS/course progress/quiz as prioritized.

- Executive KPI dashboard and operational reports.

- CSV exports with permissions.

- Notification templates/settings.

- Audit viewer and policy configuration.

- Mobile/PWA polish and offline/retry UX.

## Phase 9 - Days 87--90: Hardening & Launch

- Security review and dependency scan.

- Load/performance tests.

- Backup/restore drill.

- Production environment and provider live keys.

- Domain/DNS/Cloudflare configuration.

- Monitoring/alerts/on-call runbooks.

- Data retention/privacy review and legal policy pages.

- Smoke test Accra/Cape Coast/Kumasi launch configuration.

- Launch checklist signed off by product, operations, finance and
  technical owner.

# 29. Epics & Required Stories

  -----------------------------------------------------------------------
  **Epic**                            **Scope**
  ----------------------------------- -----------------------------------
  E01 Platform Foundation             Monorepo, CI, environments, config,
                                      logging, DB/Redis, migrations,
                                      OpenAPI.

  E02 Identity & Access               Registration, login, OTP, sessions,
                                      RBAC, MFA for privileged users.

  E03 Tourist Experience              Profile, search, booking, payment,
                                      tracking, history, receipts,
                                      reviews.

  E04 Guide Onboarding                Application, documents,
                                      verification status, certification.

  E05 Guide Marketplace               Availability, offers, acceptance,
                                      tour lifecycle, earnings.

  E06 Certification & Training        Workflow, evidence, courses, exams,
                                      expiry/retraining.

  E07 Booking & Pricing               Packages, effective pricing,
                                      booking states, cancellations.

  E08 Payments & Ledger               Collections, webhooks, ledger,
                                      refunds, receipts.

  E09 Payouts                         Wallet, payout accounts,
                                      eligibility, batches, transfers,
                                      reconciliation.

  E10 Dispatch & Tracking             Matching, Redis offers, WebSockets,
                                      GPS, map.

  E11 Safety & Incidents              SOS, operations alerts, incident
                                      workflow.

  E12 Reviews & Quality               Verified reviews, ratings,
                                      Elite/retraining rules.

  E13 Admin & Reporting               Command panel, queues, maps,
                                      reports, audit.

  E14 Notifications                   Email, SMS, push, templates,
                                      retries.

  E15 Observability & Security        Metrics, tracing, alerts, scanning,
                                      backup/runbooks.

  E16 B2B Hotels - Phase 2            Organization accounts, priority
                                      pool, subscription/invoicing.
  -----------------------------------------------------------------------

# 30. Acceptance Criteria - Critical Journeys

## 30.1 Booking and payment

- Given an ACTIVE available guide and valid package, tourist receives a
  quote generated only by backend rules.

- Creating a booking twice with the same idempotency key returns the
  same logical booking.

- Booking is not CONFIRMED from client redirect alone; confirmed only
  after verified provider state/webhook.

- A replayed success webhook does not duplicate ledger entries,
  notifications or receipt.

- Receipt amount/reference match internal payment and booking records.

## 30.2 Dispatch

- Only ACTIVE, eligible, available guides receive offers.

- Two simultaneous accept requests cannot both assign different guides
  to the same booking.

- One guide cannot hold overlapping confirmed/in-progress tours.

- Expired offers cannot be accepted.

- Operations can see why a booking has not been matched.

## 30.3 Review

- Only the tourist owning a COMPLETED booking may review.

- Maximum one review per booking.

- Rating aggregate updates transactionally/eventually with no double
  count.

- Quality threshold flags are reproducible from stored reviews/policy.

## 30.4 Finance

- Every booking allocation creates balanced ledger entries.

- Refund creates reversing entries and preserves original history.

- Payout cannot exceed eligible guide balance.

- Same provider payout callback/reference cannot mark/pay twice.

- Finance report totals reconcile to ledger and provider settlement
  inputs.

# 31. AI Agent Implementation Protocol

The following instructions tell an autonomous coding agent how to
execute this document.

19. Read the whole specification and create
    docs/implementation-status.md listing every phase, epic and
    acceptance criterion as unchecked items.

20. Create an Architecture Decision Record (ADR) for each major
    decision: modular monolith, PostgreSQL source of truth, Redis
    realtime state, optional MongoDB, payment adapter, object storage,
    session strategy and ledger.

21. Implement Phase 0 completely, run tests/builds and update
    implementation-status.md with evidence before Phase 1.

22. For each feature, implement in vertical slices: migration -\>
    repository -\> domain/service -\> handler/OpenAPI -\> frontend -\>
    tests -\> observability -\> documentation.

23. Never silently change a business rule. Put configurable values in
    policy/config tables and document defaults.

24. Before committing a database migration, verify backward
    compatibility and indexes for new query paths.

25. Before completing a payment/financial task, add replay/idempotency
    tests and ledger-invariant tests.

26. Before completing a privileged/admin task, add permission and audit
    tests.

27. Before completing a realtime task, add disconnect/reconnect/expiry
    behavior.

28. At the end of each phase, run formatting, lint, unit tests,
    integration tests, frontend typecheck/build and relevant E2E tests.
    Record command results in the phase checklist.

29. If an external credential is unavailable, implement the adapter
    against provider sandbox/mock, document the required secret, and
    keep production enablement behind configuration. Do not block
    unrelated work.

30. If this document leaves a low-level implementation detail open,
    choose the smallest maintainable solution consistent with the
    architecture. Record materially important choices as ADRs.

31. Do not introduce a new paid service without documenting why existing
    selected services cannot meet the requirement.

# 32. Suggested Agent Task Template

> Task: \<feature name\>\
> Goal: \<user/business outcome\>\
> Dependencies: \<completed tasks/migrations/integrations\>\
> Backend:\
> - domain rules\
> - migration/schema\
> - repository/service\
> - API/OpenAPI\
> Frontend:\
> - routes/components/states\
> Security:\
> - authorization / validation / rate limit\
> Observability:\
> - logs / metrics / alerts\
> Tests:\
> - unit / integration / E2E / idempotency as applicable\
> Acceptance criteria:\
> - Given/When/Then outcomes\
> Definition of done:\
> - code + migrations + tests + docs + staging smoke test

# 33. Launch Checklist

- Production Vercel and Render projects created with least-privilege
  team access.

- Production PostgreSQL backup policy enabled and restore procedure
  tested.

- Production Redis configured with access controls.

- R2 private bucket/CORS/signed URL settings validated.

- Domain/DNS/SSL and Cloudflare WAF/rate limits configured.

- Payment live account/webhook URL/signature secret configured and a
  small live transaction reconciled.

- Payout production path approved and tested safely.

- SMS sender/provider approval completed if required.

- Resend domain authenticated (SPF/DKIM) and production sender
  configured.

- Google Maps keys restricted by domain/API and spend quotas/alerts
  configured.

- Sentry/OTel dashboards and critical alerts operational.

- Admin MFA and emergency access procedure tested.

- Pricing, commission, levy, cancellation, refund, insurance and payout
  policies approved.

- Required legal/privacy/terms/consent text approved by responsible
  parties.

- Operational incident/SOS roster and escalation process documented.

- Pilot guide dataset fully verified; no placeholder "verified" badges.

- Synthetic/demo data removed from production.

- Smoke tests completed on multiple mobile devices and desktop admin.

- Rollback/runbook and support ownership documented.

# 34. Phase-2 / Post-V1 Backlog

- Hotel/B2B subscription accounts and priority guide pool.

- Native iOS/Android apps if PWA limitations affect GPS/background
  operation or push reliability.

- Multi-language tourist UI.

- In-app chat with privacy controls.

- Dynamic surge/seasonal pricing subject to policy.

- Corporate/embassy accounts and invoicing.

- Tour bundles, attractions/ticket integrations.

- Insurance-provider API integration.

- Formal police/authority verification API integration where available.

- Guide equipment/uniform inventory.

- Referral/loyalty program.

- AI trip planner using vetted tourism content.

- ML-assisted dispatch only after sufficient historical data and offline
  evaluation.

- Data warehouse/BI pipeline for national tourism analysis.

- Expansion to additional countries with country-specific policy/payment
  adapters.

# 35. Cost-aware Service Decisions

  -------------------------------------------------------------------------
  **Need**                **V1 selection**        **Rule**
  ----------------------- ----------------------- -------------------------
  Frontend hosting        Vercel                  Use Pro when
                                                  production/team limits
                                                  require it; enforce usage
                                                  alerts.

  Go hosting              Render                  One API + one worker
                                                  initially; scale
                                                  vertically/horizontally
                                                  from metrics.

  Primary database        Render PostgreSQL       Mandatory source of
                                                  truth.

  Redis                   Upstash                 Use for
                                                  ephemeral/realtime only;
                                                  avoid turning it into
                                                  permanent truth.

  MongoDB Atlas           Optional                Do not provision unless a
                                                  concrete document-data
                                                  need exists.

  Email                   Resend                  Transactional email only.

  Maps                    Google Maps             Quota/cache expensive
                                                  calls; raw GPS goes
                                                  directly to Guide Ghana.

  Storage                 Cloudflare R2           Private storage with
                                                  signed URLs.

  Payments                Paystack primary,       No raw card handling;
                          adapter-compatible with negotiate commercial
                          Hubtel                  rates at scale.

  Push                    FCM                     Primary low-cost push
                                                  channel.

  Monitoring              Sentry + OpenTelemetry  Start lean, preserve
                                                  portability.
  -------------------------------------------------------------------------

# 36. Final Build Principles

- Correctness before cleverness.

- Financial and safety flows are never "eventually implemented later"
  after launch.

- PostgreSQL is the durable truth; Redis is ephemeral; MongoDB is
  optional.

- State machines are explicit and centrally enforced.

- Provider webhooks are hostile/retriable input until authenticated and
  deduplicated.

- Sensitive access is least-privilege and audited.

- Configuration changes are traceable and financial rules are
  effective-dated.

- Realtime UX must degrade gracefully to refresh/polling where feasible.

- Do not spend on infrastructure complexity that the measured load does
  not require.

- Every phase leaves the product deployable, observable and testable.

# Appendix A. Minimum Permission Codes

- guides.read

- guides.manage

- certification.read

- certification.review

- bookings.read

- bookings.manage

- dispatch.manage

- incidents.read

- incidents.manage

- payments.read

- payments.refund

- payouts.read

- payouts.manage

- ledger.read

- reviews.moderate

- training.manage

- pricing.manage

- reports.read

- reports.export

- users.read

- users.manage

- rbac.manage

- audit.read

- settings.manage

# Appendix B. Initial Review Tags

- Knowledgeable

- Punctual

- Friendly

- Professional

- Helpful

- Great Storyteller

- Safety Conscious

- Good Communicator

- Local Expert

- Exceeded Expectations

# Appendix C. Minimum Seed Specialties

- City Tours

- Heritage & History

- Culture & Arts

- Food & Culinary

- Nature & Ecotourism

- Adventure

- Nightlife

- Religious Heritage

- Business/Conference Support

- Photography Tours

- Family Tours

- Accessible Tourism

- Multi-region Tours

# Appendix D. Agent Stop Conditions

The agent must treat the following as blockers for production launch,
but not blockers for continuing unrelated development:

- No verified production payment webhook/signature setup.

- Ledger invariant tests failing.

- Critical auth/RBAC bypass.

- SOS event cannot reach operations dashboard.

- No database backup/restore procedure.

- Admin privileged accounts lack required MFA.

- Guide "verified/insured" badge can be displayed without actual valid
  evidence/status.

- Critical personal documents are publicly accessible.

- Duplicate payout possible under retries/concurrency.

- Production secrets committed to repository.

END OF SPECIFICATION
