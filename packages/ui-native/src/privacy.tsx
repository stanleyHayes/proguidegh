/**
 * Privacy & data screen, shared by both native apps (Phase M, M-20).
 *
 * Exists to satisfy three obligations that all want the same screen:
 *   * Apple App Store Review 5.1.1(v) — an app with account creation must
 *     offer account deletion from inside the app.
 *   * Google Play "Data deletion" policy — same.
 *   * Ghana Data Protection Act 2012 — access to, and erasure of, personal data.
 *
 * Design rules that are compliance requirements, not preferences:
 *   - Deletion states plainly that it is permanent, and lists what survives it
 *     BEFORE the confirm step. The list comes from the server so the app and
 *     the privacy policy cannot drift apart.
 *   - A refusal names the specific, temporary reason (an unfinished booking, an
 *     unsettled payout). "Contact support" is not an acceptable answer.
 *   - Confirmation is an explicit second step, never a single tap.
 */
import { useCallback, useEffect, useState } from "react";
import { Linking, ScrollView, StyleSheet, Text, View } from "react-native";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { Card, ErrorState, LoadingState, PrimaryButton } from "./index";

/** Minimal surface the screen needs; both apps' clients satisfy it. */
export interface PrivacyApi {
  api<T = unknown>(
    path: string,
    options?: { method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE" },
  ): Promise<T>;
}

interface Blocker {
  reason: string;
  message: string;
}

interface Preview {
  canDelete: boolean;
  blockers: Blocker[];
  retained: string[];
  removed: string[];
}

interface Policy {
  document: string;
  version: string;
  url: string;
}

function parsePreview(data: unknown): Preview {
  const rec = (data ?? {}) as Record<string, unknown>;
  const list = (value: unknown): string[] =>
    Array.isArray(value) ? value.filter((v): v is string => typeof v === "string") : [];
  const blockers = Array.isArray(rec.blockers)
    ? rec.blockers.flatMap((b) => {
        const entry = b as Record<string, unknown>;
        return typeof entry?.message === "string"
          ? [{ reason: String(entry.reason ?? ""), message: entry.message }]
          : [];
      })
    : [];
  return {
    canDelete: rec.can_delete === true,
    blockers,
    retained: list(rec.retained),
    removed: list(rec.removed),
  };
}

function parsePolicies(data: unknown): Policy[] {
  const rec = (data ?? {}) as Record<string, unknown>;
  const raw = Array.isArray(rec.policies) ? rec.policies : [];
  return raw.flatMap((p) => {
    const entry = p as Record<string, unknown>;
    return typeof entry?.document === "string" && typeof entry?.url === "string"
      ? [{
          document: entry.document,
          version: String(entry.version ?? ""),
          url: entry.url,
        }]
      : [];
  });
}

const POLICY_LABELS: Record<string, string> = {
  terms: "Terms of service",
  privacy: "Privacy policy",
  location: "Location sharing policy",
};

export interface PrivacyScreenProps {
  client: PrivacyApi;
  /** Called after erasure completes so the app can clear its session. */
  onDeleted: () => void;
}

export function PrivacyScreen({ client, onDeleted }: PrivacyScreenProps) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [policies, setPolicies] = useState<Policy[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const [exportDone, setExportDone] = useState(false);

  const load = useCallback(async () => {
    try {
      const [previewData, policyData] = await Promise.all([
        client.api("/me/deletion"),
        client.api("/legal/policies"),
      ]);
      setPreview(parsePreview(previewData));
      setPolicies(parsePolicies(policyData));
      setError(null);
    } catch {
      setError("Could not load your privacy settings. Check your connection.");
      setPreview((current) => current ?? null);
    }
  }, [client]);

  useEffect(() => {
    const t = setTimeout(() => void load(), 0);
    return () => clearTimeout(t);
  }, [load]);

  async function requestExport() {
    setExporting(true);
    setExportDone(false);
    try {
      await client.api("/me/export");
      setExportDone(true);
    } catch {
      setError("Could not build your data export. Try again shortly.");
    } finally {
      setExporting(false);
    }
  }

  async function confirmDelete() {
    setDeleting(true);
    setDeleteError(null);
    try {
      await client.api("/me", { method: "DELETE" });
      onDeleted();
    } catch (err: unknown) {
      // The server's message is the specific reason (active booking, unsettled
      // payout); surface it rather than a generic failure.
      const message =
        err instanceof Error && err.message ? err.message : "Could not delete your account.";
      setDeleteError(message);
      setConfirming(false);
      await load();
    } finally {
      setDeleting(false);
    }
  }

  if (!preview && !error) return <LoadingState label="Loading privacy settings…" />;

  return (
    <ScrollView contentContainerStyle={styles.page}>
      {error ? <ErrorState message={error} onRetry={() => void load()} /> : null}

      <Card>
        <Text style={styles.sectionTitle}>Your documents</Text>
        {policies.length === 0 ? (
          <Text style={styles.muted}>Policy documents are unavailable right now.</Text>
        ) : (
          policies.map((policy) => (
            <PrimaryButton
              key={policy.document}
              label={POLICY_LABELS[policy.document] ?? policy.document}
              onPress={() => void Linking.openURL(policy.url)}
            />
          ))
        )}
      </Card>

      <Card>
        <Text style={styles.sectionTitle}>Get a copy of your data</Text>
        <Text style={styles.muted}>
          We will assemble everything we hold about you — your account, profile,
          bookings, reviews and the policies you have accepted.
        </Text>
        {exportDone ? (
          <Text style={styles.success}>
            Your data was prepared. Contact support if you need it as a file.
          </Text>
        ) : null}
        <PrimaryButton
          busy={exporting}
          label="Request my data"
          onPress={() => void requestExport()}
        />
      </Card>

      <Card>
        <Text style={styles.sectionTitleDanger}>Delete your account</Text>

        {preview?.removed.length ? (
          <>
            <Text style={styles.listHeading}>This will permanently remove:</Text>
            {preview.removed.map((item) => (
              <Text key={item} style={styles.listItem}>
                • {item}
              </Text>
            ))}
          </>
        ) : null}

        {preview?.retained.length ? (
          <>
            <Text style={styles.listHeading}>This will be kept:</Text>
            {preview.retained.map((item) => (
              <Text key={item} style={styles.listItem}>
                • {item}
              </Text>
            ))}
          </>
        ) : null}

        {preview && !preview.canDelete ? (
          <View style={styles.blocked}>
            {preview.blockers.map((blocker) => (
              <Text key={blocker.reason} style={styles.blockedText}>
                {blocker.message}
              </Text>
            ))}
          </View>
        ) : null}

        {deleteError ? <ErrorState message={deleteError} /> : null}

        {confirming ? (
          <>
            <Text style={styles.confirmText}>
              This cannot be undone. You will be signed out immediately and will not
              be able to sign in again with this account.
            </Text>
            <PrimaryButton
              busy={deleting}
              label="Yes, delete my account permanently"
              onPress={() => void confirmDelete()}
            />
            <PrimaryButton label="Cancel" onPress={() => setConfirming(false)} />
          </>
        ) : (
          <PrimaryButton
            disabled={!preview?.canDelete}
            label="Delete my account"
            onPress={() => setConfirming(true)}
          />
        )}
      </Card>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[4], padding: space[4] },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  sectionTitleDanger: { color: colors.danger, fontSize: fontSize.base, fontWeight: "700" },
  muted: {
    color: colors.muted,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
  listHeading: {
    color: colors.ink,
    fontSize: fontSize.sm,
    fontWeight: "600",
    marginTop: space[2],
  },
  listItem: {
    color: colors.muted,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
  blocked: {
    backgroundColor: colors.surfaceAlt,
    borderColor: colors.border,
    borderRadius: radius.sm,
    borderWidth: 1,
    marginTop: space[2],
    padding: space[3],
  },
  blockedText: {
    color: colors.warning,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
  confirmText: {
    color: colors.danger,
    fontSize: fontSize.sm,
    fontWeight: "600",
    lineHeight: fontSize.sm * 1.5,
    marginTop: space[2],
  },
  success: { color: colors.success, fontSize: fontSize.sm, fontWeight: "600" },
});
