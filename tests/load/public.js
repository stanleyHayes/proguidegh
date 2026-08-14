// Public-endpoint load test (P9-02): health, catalog and guide search —
// the highest-traffic anonymous surfaces.
import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: __ENV.DURATION || "30s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
  },
};

const BASE = __ENV.API_URL || "http://localhost:8080";

export default function () {
  const search = http.get(
    `${BASE}/api/v1/guides/search?region_id=&language=en&page=1`,
  );
  check(search, { "search 200": (r) => r.status === 200 });

  const regions = http.get(`${BASE}/api/v1/regions`);
  check(regions, { "regions 200": (r) => r.status === 200 });

  const packages = http.get(`${BASE}/api/v1/tour-packages`);
  check(packages, { "packages 200": (r) => r.status === 200 });

  const health = http.get(`${BASE}/readyz`);
  check(health, { "readyz 200": (r) => r.status === 200 });

  sleep(0.1);
}
