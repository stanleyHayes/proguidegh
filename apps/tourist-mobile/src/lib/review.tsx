/**
 * Verified review form (Phase 6 parity with tourist-web; spec §4.4) —
 * one review per completed booking, tags from the Appendix B dictionary
 * (the server rejects anything outside it; this list mirrors that contract).
 */
import { useState } from "react";
import { StyleSheet, Text, TextInput } from "react-native";
import { colors, fontSize, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { Card, ChipSelect, PrimaryButton } from "@/lib/ui";

const RATINGS = [
  { id: "5", name: "5 stars" },
  { id: "4", name: "4 stars" },
  { id: "3", name: "3 stars" },
  { id: "2", name: "2 stars" },
  { id: "1", name: "1 star" },
];

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
].map((tag) => ({ id: tag, name: tag }));

export function ReviewForm({ bookingId }: { bookingId: string }) {
  const { client } = useSession();
  const [rating, setRating] = useState<string | null>("5");
  const [tag, setTag] = useState<string | null>(null);
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function submit() {
    if (!rating) return;
    setBusy(true);
    setError(null);
    try {
      await client.api(`/bookings/${bookingId}/review`, {
        method: "POST",
        body: {
          rating: Number(rating),
          body: body.trim() === "" ? undefined : body.trim(),
          tags: tag ? [tag] : [],
        },
      });
      setDone(true);
    } catch (err) {
      // 409 means the booking was already reviewed — treat as done.
      if (err instanceof Error && "status" in err && (err as { status?: number }).status === 409) {
        setDone(true);
        return;
      }
      setError(errorMessage(err, "Your review could not be saved."));
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <Card>
        <Text style={styles.title}>Thank you</Text>
        <Text style={styles.muted}>Your review has been recorded.</Text>
      </Card>
    );
  }

  return (
    <Card>
      <Text style={styles.title}>Review your tour</Text>
      <ChipSelect label="Rating" options={RATINGS} value={rating} onChange={setRating} />
      <ChipSelect
        label="What stood out? (optional)"
        options={REVIEW_TAGS}
        value={tag}
        onChange={setTag}
      />
      <TextInput
        accessibilityLabel="Your review (optional)"
        multiline
        onChangeText={setBody}
        placeholder="How was your tour? (optional)"
        placeholderTextColor={colors.muted}
        style={styles.input}
        value={body}
      />
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <PrimaryButton busy={busy} disabled={!rating} label="Submit review" onPress={() => void submit()} />
    </Card>
  );
}

const styles = StyleSheet.create({
  title: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  muted: { color: colors.muted, fontSize: fontSize.sm },
  error: { color: colors.danger, fontSize: fontSize.sm },
  input: {
    borderColor: colors.border,
    borderRadius: 8,
    borderWidth: 1,
    color: colors.ink,
    fontSize: fontSize.base,
    minHeight: 72,
    paddingHorizontal: space[3],
    paddingTop: space[2],
    textAlignVertical: "top",
  },
});
