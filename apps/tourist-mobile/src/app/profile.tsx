/**
 * Tourist profile (Phase M, M-12) — GET/PATCH /me/tourist-profile.
 *
 * Writes are blocked while offline rather than queued; see lib/offline.tsx
 * for why this app has no write queue.
 */
import { useCallback, useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";
import { useOnline } from "@/lib/offline";
import { Card, ErrorState, LoadingState, PrimaryButton } from "@/lib/ui";

interface Profile {
  fullName: string;
  nationality: string;
  preferredLanguage: string;
  emergencyContactName: string;
  emergencyContactPhone: string;
}

const EMPTY: Profile = {
  fullName: "",
  nationality: "",
  preferredLanguage: "",
  emergencyContactName: "",
  emergencyContactPhone: "",
};

function parseProfile(data: unknown): Profile {
  const outer = data as Record<string, unknown> | null;
  const rec = (outer?.profile ?? outer) as Record<string, unknown> | null;
  const get = (key: string): string => {
    const value = rec?.[key];
    return typeof value === "string" ? value : "";
  };
  return {
    fullName: get("full_name"),
    nationality: get("nationality"),
    preferredLanguage: get("preferred_language"),
    emergencyContactName: get("emergency_contact_name"),
    emergencyContactPhone: get("emergency_contact_phone_e164"),
  };
}

export default function ProfileScreen() {
  const { client, signOut } = useSession();
  const online = useOnline();
  const router = useRouter();
  const [profile, setProfile] = useState<Profile>(EMPTY);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const refresh = useCallback(() => {
    setLoading(true);
    setError(null);
    client
      .api("/me/tourist-profile")
      .then((data) => setProfile(parseProfile(data)))
      .catch((err: unknown) =>
        setError(errorMessage(err, "Could not load your profile.")),
      )
      .finally(() => setLoading(false));
  }, [client]);

  useEffect(() => {
    const t = setTimeout(refresh, 0);
    return () => clearTimeout(t);
  }, [refresh]);

  async function save() {
    setSaving(true);
    setSaveError(null);
    setSaved(false);
    try {
      // The PATCH decoder rejects unknown fields (DisallowUnknownFields) and
      // an empty full_name — omit it rather than send "" when cleared.
      await client.api("/me/tourist-profile", {
        method: "PATCH",
        body: {
          full_name: profile.fullName.trim() === "" ? undefined : profile.fullName.trim(),
          nationality: profile.nationality,
          preferred_language: profile.preferredLanguage,
          emergency_contact_name: profile.emergencyContactName,
          emergency_contact_phone_e164: profile.emergencyContactPhone,
        },
      });
      setSaved(true);
    } catch (err: unknown) {
      setSaveError(errorMessage(err, "Could not save your profile."));
    } finally {
      setSaving(false);
    }
  }

  function field(key: keyof Profile, label: string, placeholder?: string) {
    return (
      <View style={styles.field}>
        <Text style={styles.label}>{label}</Text>
        <TextInput
          accessibilityLabel={label}
          onChangeText={(text) => {
            setSaved(false);
            setProfile((current) => ({ ...current, [key]: text }));
          }}
          placeholder={placeholder}
          placeholderTextColor={colors.muted}
          style={styles.input}
          value={profile[key]}
        />
      </View>
    );
  }

  if (loading) return <LoadingState label="Loading your profile…" />;
  if (error) return <ErrorState message={error} onRetry={refresh} />;

  return (
    <ScrollView contentContainerStyle={styles.page}>
      <Card>
        {field("fullName", "Full name")}
        {field("nationality", "Nationality")}
        {field("preferredLanguage", "Preferred language", "en")}
      </Card>

      <Card>
        <Text style={styles.sectionTitle}>Emergency contact</Text>
        <Text style={styles.muted}>
          Used only if you raise an SOS during a tour.
        </Text>
        {field("emergencyContactName", "Contact name")}
        {field("emergencyContactPhone", "Contact phone", "+233…")}
      </Card>

      {saveError ? <ErrorState message={saveError} /> : null}
      {saved ? <Text style={styles.saved}>Profile saved.</Text> : null}
      {!online ? (
        <Text style={styles.muted}>
          Saving needs a connection. Your edits stay on screen until you reconnect.
        </Text>
      ) : null}

      <PrimaryButton
        busy={saving}
        disabled={!online}
        label="Save profile"
        onPress={() => void save()}
      />
      <PrimaryButton
        label="Privacy & data"
        onPress={() => router.push("/privacy")}
      />
      <PrimaryButton label="Sign out" onPress={() => void signOut()} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  page: { gap: space[4], padding: space[4] },
  sectionTitle: { color: colors.ink, fontSize: fontSize.base, fontWeight: "600" },
  muted: {
    color: colors.muted,
    fontSize: fontSize.sm,
    lineHeight: fontSize.sm * 1.5,
  },
  field: { gap: space[1] },
  label: { color: colors.ink, fontSize: fontSize.sm, fontWeight: "600" },
  input: {
    borderColor: colors.border,
    borderRadius: radius.sm,
    borderWidth: 1,
    color: colors.ink,
    fontSize: fontSize.base,
    minHeight: 44,
    paddingHorizontal: space[3],
  },
  saved: { color: colors.success, fontSize: fontSize.sm, fontWeight: "600" },
});
