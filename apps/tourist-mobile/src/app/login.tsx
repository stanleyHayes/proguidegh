/**
 * Login screen (Phase M, M-07). Email + password; MFA code step when the
 * account requires it (privileged roles per spec §15.2). No card fields,
 * no token handling here — tokens go straight to expo-secure-store via
 * the session module.
 */
import { useState } from "react";
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";
import { useRouter } from "expo-router";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";
import { useSession, errorMessage } from "@/lib/session";

export default function LoginScreen() {
  const router = useRouter();
  const { signIn, signInMfa } = useSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [mfaChallenge, setMfaChallenge] = useState<string | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      await signIn(email.trim(), password);
      router.replace("/");
    } catch (err) {
      if (err instanceof Error && err.message === "MFA_REQUIRED") {
        setMfaChallenge((err as { challenge?: string }).challenge ?? "");
      } else {
        setError(errorMessage(err, "Could not sign in. Try again."));
      }
    } finally {
      setBusy(false);
    }
  }

  async function submitMfa() {
    if (!mfaChallenge) return;
    setBusy(true);
    setError(null);
    try {
      await signInMfa(mfaChallenge, mfaCode.trim());
      router.replace("/");
    } catch (err) {
      setError(errorMessage(err, "That code did not work. Try again."));
    } finally {
      setBusy(false);
    }
  }

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.flex}
    >
      <ScrollView contentContainerStyle={styles.page}>
        <Text style={styles.heading}>
          {mfaChallenge ? "Enter your security code" : "Sign in"}
        </Text>

        {error ? (
          <View accessibilityLiveRegion="polite" style={styles.errorBox}>
            <Text style={styles.errorText}>{error}</Text>
          </View>
        ) : null}

        {mfaChallenge ? (
          <>
            <Text style={styles.body}>
              This account requires multi-factor authentication. Enter the
              6-digit code from your authenticator app.
            </Text>
            <Text nativeID="mfa-label" style={styles.label}>
              Security code
            </Text>
            <TextInput
              accessibilityLabelledBy="mfa-label"
              autoComplete="one-time-code"
              autoFocus
              inputMode="numeric"
              maxLength={6}
              onChangeText={setMfaCode}
              style={styles.input}
              value={mfaCode}
            />
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ disabled: busy || mfaCode.length !== 6 }}
              disabled={busy || mfaCode.length !== 6}
              onPress={submitMfa}
              style={[styles.button, (busy || mfaCode.length !== 6) && styles.buttonDisabled]}
            >
              {busy ? (
                <ActivityIndicator color={colors.surface} />
              ) : (
                <Text style={styles.buttonLabel}>Verify and sign in</Text>
              )}
            </Pressable>
          </>
        ) : (
          <>
            <Text nativeID="email-label" style={styles.label}>
              Email
            </Text>
            <TextInput
              accessibilityLabelledBy="email-label"
              autoCapitalize="none"
              autoComplete="email"
              autoCorrect={false}
              inputMode="email"
              onChangeText={setEmail}
              style={styles.input}
              value={email}
            />
            <Text nativeID="password-label" style={styles.label}>
              Password
            </Text>
            <TextInput
              accessibilityLabelledBy="password-label"
              autoComplete="current-password"
              onChangeText={setPassword}
              secureTextEntry
              style={styles.input}
              value={password}
            />
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ disabled: busy || !email || !password }}
              disabled={busy || !email || !password}
              onPress={submit}
              style={[styles.button, (busy || !email || !password) && styles.buttonDisabled]}
            >
              {busy ? (
                <ActivityIndicator color={colors.surface} />
              ) : (
                <Text style={styles.buttonLabel}>Sign in</Text>
              )}
            </Pressable>
          </>
        )}
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1, backgroundColor: colors.surface },
  page: { padding: space[6], gap: space[3] },
  heading: { color: colors.ink, fontSize: fontSize.xl, fontWeight: "700" },
  body: { color: colors.muted, fontSize: fontSize.base },
  label: { color: colors.ink, fontSize: fontSize.sm, fontWeight: "600" },
  input: {
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
    color: colors.ink,
    fontSize: fontSize.base,
    minHeight: 44,
    paddingHorizontal: space[3],
  },
  button: {
    alignItems: "center",
    backgroundColor: colors.primary,
    borderRadius: radius.md,
    justifyContent: "center",
    marginTop: space[2],
    minHeight: 44,
  },
  buttonDisabled: { opacity: 0.5 },
  buttonLabel: { color: colors.surface, fontSize: fontSize.base, fontWeight: "600" },
  errorBox: {
    backgroundColor: colors.surfaceAlt,
    borderRadius: radius.md,
    padding: space[3],
  },
  errorText: { color: colors.danger, fontSize: fontSize.sm },
});
