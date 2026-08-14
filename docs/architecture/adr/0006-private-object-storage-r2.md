# ADR 0006: Private object storage with signed URLs (Cloudflare R2)

**Status:** Accepted
**Date:** 2026-08-13
**Spec reference:** §1.2, §6.1, §16.4, §17

## Context

Guide verification documents, certificates, profile photos and generated receipts contain personal data. Public exposure of verification documents is an explicit stop condition (Appendix D). The database is the wrong place for binary payloads.

## Decision

All sensitive files live in private Cloudflare R2 buckets and are accessed only through short-lived presigned URLs (upload and download).

Rules:

- Buckets are private by default; no public-read objects for verification documents or receipts (§1.2, §15.3).
- Presigned upload validates MIME type, extension, declared category and maximum size; scan files where feasible before marking documents usable (§16.4).
- Object keys must not contain raw personal data (§16.4).
- Receipt PDFs are generated server-side and stored in R2; downloads use short-lived signed URLs (§17).
- Staging and production use separate buckets (§15.3).
- Development/CI may run against a mock signing abstraction when R2 credentials are unavailable (§31.29).

## Consequences

- Document access is always mediated by the authorization layer; URL expiry limits leak blast radius.
- A "public personal documents" incident is structurally hard to create, but CORS/bucket misconfiguration remains a launch-checklist validation item (§33).
- Signed-URL generation is a small, testable platform component; storage backend could change without touching domain code.
