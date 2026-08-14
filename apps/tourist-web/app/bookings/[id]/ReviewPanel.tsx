"use client";

import { useState } from "react";
import { Alert, Button, Card, Select } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";

/**
 * Verified review form (spec §4.4) — one review per completed booking.
 * Tags come from the Appendix B dictionary; the server rejects anything
 * outside it, so this list is the client mirror of that contract.
 */

const REVIEW_TAGS = [
  "Knowledgeable",
  "Punctual",
  "Friendly",
  "Professional",
  "Helpful",
  "Great Storyteller",
  "Safety Conscious",
  "Good Communicator",
  "Local Expert",
  "Exceeded Expectations",
] as const;

export default function ReviewPanel({
  bookingId,
  status,
}: {
  bookingId: string;
  status?: string;
}) {
  const [rating, setRating] = useState(5);
  const [body, setBody] = useState("");
  const [tags, setTags] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  if (status !== "COMPLETED") return null;

  function toggleTag(tag: string) {
    setTags((current) =>
      current.includes(tag)
        ? current.filter((t) => t !== tag)
        : [...current, tag],
    );
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api(`/bookings/${bookingId}/review`, {
        method: "POST",
        body: {
          rating,
          body: body.trim() === "" ? undefined : body.trim(),
          tags,
        },
      });
      setDone(true);
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setDone(true); // already reviewed — nothing more to do here
        return;
      }
      setError(errorMessage(err, "Your review could not be saved."));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card title="Review your tour">
      {done ? (
        <Alert tone="success" title="Thank you">
          <p>Your review has been recorded.</p>
        </Alert>
      ) : (
        <form className="stack" onSubmit={(e) => void submit(e)}>
          <Select label="Rating"
            id="review-rating"
            value={String(rating)}
            onChange={(e) => setRating(Number(e.target.value))}
          >
            {[5, 4, 3, 2, 1].map((n) => (
              <option key={n} value={n}>
                {n} star{n === 1 ? "" : "s"}
              </option>
            ))}
          </Select>

          <fieldset className="stack">
            <legend>What stood out? (optional)</legend>
            {REVIEW_TAGS.map((tag) => (
              <label key={tag}>
                <input
                  type="checkbox"
                  checked={tags.includes(tag)}
                  onChange={() => toggleTag(tag)}
                />{" "}
                {tag}
              </label>
            ))}
          </fieldset>

          <label htmlFor="review-body">Your review (optional)</label>
          <textarea
            id="review-body"
            rows={4}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="How was your tour?"
          />

          {error ? <Alert tone="error">{error}</Alert> : null}
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Submit review"}
          </Button>
        </form>
      )}
    </Card>
  );
}
