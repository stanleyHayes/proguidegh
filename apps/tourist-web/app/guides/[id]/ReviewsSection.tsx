"use client";

import { useEffect, useState } from "react";
import { Badge, Card } from "@proguidegh/ui";
import { api } from "../../lib/api";

/**
 * Public reviews for a guide (spec §13.5, P6-04). The endpoint exposes no
 * tourist identity — reviews render as rating + tags + body only. Failures
 * degrade to silence: a missing review list must never block booking.
 */

interface PublicReview {
  id: string;
  rating: number;
  body?: string | null;
  tags: string[];
  created_at: string;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function parseReviews(data: unknown): PublicReview[] {
  const list = asRecord(data)?.reviews;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      rating: Number(r.rating ?? 0),
      body: (r.body as string | null) ?? null,
      tags: Array.isArray(r.tags) ? (r.tags as string[]) : [],
      created_at: String(r.created_at ?? ""),
    }));
}

export default function ReviewsSection({ guideId }: { guideId: string }) {
  const [reviews, setReviews] = useState<PublicReview[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    void api(`/guides/${guideId}/reviews`)
      .then((data) => {
        if (!cancelled) setReviews(parseReviews(data));
      })
      .catch(() => {
        if (!cancelled) setReviews([]);
      });
    return () => {
      cancelled = true;
    };
  }, [guideId]);

  if (!reviews || reviews.length === 0) return null;

  return (
    <Card title={`Reviews (${reviews.length})`}>
      <ul className="stack" aria-label="Guide reviews">
        {reviews.map((review) => (
          <li key={review.id} className="stack">
            <p aria-label={`Rated ${review.rating} out of 5`}>
              <span aria-hidden="true">
                {"★".repeat(review.rating)}
                {"☆".repeat(Math.max(0, 5 - review.rating))}
              </span>{" "}
              <span className="muted">
                {new Date(review.created_at).toLocaleDateString(undefined, {
                  day: "numeric",
                  month: "short",
                  year: "numeric",
                })}
              </span>
            </p>
            {review.tags.length > 0 ? (
              <div className="badge-row">
                {review.tags.map((tag) => (
                  <Badge key={tag} tone="neutral">
                    {tag}
                  </Badge>
                ))}
              </div>
            ) : null}
            {review.body ? <p>{review.body}</p> : null}
          </li>
        ))}
      </ul>
    </Card>
  );
}
