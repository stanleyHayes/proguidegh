// Authenticated booking-flow load test (P9-02): register → quote →
// booking create (idempotent) → list. Exercises the write path and the
// idempotency layer under concurrency.
import http from "k6/http";
import { check, sleep } from "k6";
import { uuidv4 } from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

export const options = {
  vus: Number(__ENV.VUS || 10),
  duration: __ENV.DURATION || "30s",
  thresholds: {
    http_req_failed: ["rate<0.02"],
    http_req_duration: ["p(95)<1000"],
  },
};

const BASE = __ENV.API_URL || "http://localhost:8080";
const JSON_HEADERS = { "Content-Type": "application/json" };

function jsonHeaders(token, extra = {}) {
  return { ...JSON_HEADERS, Authorization: `Bearer ${token}`, ...extra };
}

export function setup() {
  // A package id for quotes/bookings.
  const res = http.get(`${BASE}/api/v1/tour-packages`);
  const packages = res.json("tour_packages") || res.json("packages") || [];
  return { packageId: packages.length > 0 ? packages[0].id : null };
}

export default function (data) {
  if (!data.packageId) return;

  const email = `load-${uuidv4()}@example.com`;
  const register = http.post(
    `${BASE}/api/v1/auth/register`,
    JSON.stringify({
      intent: "tourist",
      email,
      password: "loadtest-loadtest",
      full_name: "Load Tester",
    }),
    { headers: JSON_HEADERS },
  );
  check(register, { "register 201": (r) => r.status === 201 });
  const token = register.json("access_token");
  if (!token) return;

  const startsAt = new Date(Date.now() + 7 * 864e5).toISOString();
  const quote = http.post(
    `${BASE}/api/v1/bookings/quote`,
    JSON.stringify({ package_id: data.packageId, starts_at: startsAt, guests: 2 }),
    { headers: JSON_HEADERS },
  );
  check(quote, { "quote 200": (r) => r.status === 200 });

  // Idempotency-Key makes a duplicate submission a replay, not a double
  // booking — submit the same key twice and expect one booking.
  const idemKey = uuidv4();
  const payload = JSON.stringify({
    package_id: data.packageId,
    starts_at: startsAt,
    guests: 2,
  });
  const first = http.post(`${BASE}/api/v1/bookings`, payload, {
    headers: jsonHeaders(token, { "Idempotency-Key": idemKey }),
  });
  check(first, { "create 201": (r) => r.status === 201 });
  const replay = http.post(`${BASE}/api/v1/bookings`, payload, {
    headers: jsonHeaders(token, { "Idempotency-Key": idemKey }),
  });
  check(replay, {
    "idempotent replay": (r) =>
      r.status === 201 &&
      r.json("booking.id") === first.json("booking.id"),
  });

  const mine = http.get(`${BASE}/api/v1/me/bookings`, {
    headers: jsonHeaders(token),
  });
  check(mine, { "list 200": (r) => r.status === 200 });

  sleep(0.2);
}
