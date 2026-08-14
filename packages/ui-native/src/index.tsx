/**
 * Small shared RN components for the tourist app (Phase M). Web uses
 * @proguidegh/ui; native keeps its own components over the same tokens
 * (agent_plan.md §M.3 — ui is DOM-only and must not be imported here).
 */
import type { ReactNode } from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { colors, fontSize, radius, space } from "@proguidegh/tokens";

export function LoadingState({ label = "Loading…" }: { label?: string }) {
  return (
    <View style={[styles.center, styles.statePanel]}>
      <View style={styles.loadingMark}>
        <ActivityIndicator color={colors.surface} size="small" />
      </View>
      <Text style={styles.muted}>{label}</Text>
    </View>
  );
}

export function ErrorState({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <View style={[styles.center, styles.errorBox]} accessibilityLiveRegion="polite">
      <Text style={styles.errorText}>{message}</Text>
      {onRetry ? (
        <Pressable accessibilityRole="button" onPress={onRetry} style={styles.retry}>
          <Text style={styles.retryLabel}>Try again</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

export function EmptyState({ title, body }: { title: string; body?: string }) {
  return (
    <View style={[styles.center, styles.statePanel]}>
      <View style={styles.emptyMark}><Text style={styles.emptyMarkText}>PG</Text></View>
      <Text style={styles.emptyTitle}>{title}</Text>
      {body ? <Text style={styles.muted}>{body}</Text> : null}
    </View>
  );
}

export function Badge({
  label,
  tone = "neutral",
}: {
  label: string;
  tone?: "neutral" | "success" | "gold";
}) {
  return (
    <View
      style={[
        styles.badge,
        tone === "success" && styles.badgeSuccess,
        tone === "gold" && styles.badgeGold,
      ]}
    >
      <Text
        style={[
          styles.badgeText,
          (tone === "success" || tone === "gold") && styles.badgeTextLight,
        ]}
      >
        {label}
      </Text>
    </View>
  );
}

/** Horizontal single-select chip row — the RN stand-in for a <select>. */
export function ChipSelect({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: { id: string; name: string }[];
  value: string | null;
  onChange: (id: string | null) => void;
}) {
  return (
    <View style={styles.chipGroup}>
      <Text style={styles.chipLabel}>{label}</Text>
      <View style={styles.chipRow}>
        <Chip label="Any" selected={value === null} onPress={() => onChange(null)} />
        {options.map((opt) => (
          <Chip
            key={opt.id}
            label={opt.name}
            selected={value === opt.id}
            onPress={() => onChange(opt.id)}
          />
        ))}
      </View>
    </View>
  );
}

function Chip({
  label,
  selected,
  onPress,
}: {
  label: string;
  selected: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ selected }}
      onPress={onPress}
      style={[styles.chip, selected && styles.chipSelected]}
    >
      <Text style={[styles.chipText, selected && styles.chipTextSelected]}>
        {label}
      </Text>
    </Pressable>
  );
}

export function Card({ children }: { children: ReactNode }) {
  return <View style={styles.card}>{children}</View>;
}

export function PrimaryButton({
  label,
  onPress,
  disabled = false,
  busy = false,
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  busy?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: disabled || busy, busy }}
      disabled={disabled || busy}
      onPress={onPress}
      style={[styles.primary, (disabled || busy) && styles.primaryDisabled]}
    >
      {busy ? (
        <ActivityIndicator color={colors.surface} />
      ) : (
        <Text style={styles.primaryLabel}>{label}</Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  center: { alignItems: "center", gap: space[2], padding: space[6] },
  statePanel: {
    backgroundColor: colors.surface,
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
    marginVertical: space[3],
  },
  loadingMark: {
    alignItems: "center",
    backgroundColor: colors.primaryStrong,
    borderRadius: radius.md,
    height: 44,
    justifyContent: "center",
    width: 44,
  },
  emptyMark: {
    alignItems: "center",
    backgroundColor: colors.surfaceAlt,
    borderRadius: radius.md,
    height: 44,
    justifyContent: "center",
    width: 44,
  },
  emptyMarkText: { color: colors.primary, fontSize: fontSize.sm, fontWeight: "700" },
  muted: { color: colors.muted, fontSize: fontSize.sm, textAlign: "center" },
  errorBox: {
    backgroundColor: colors.surfaceAlt,
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
  },
  errorText: { color: colors.danger, fontSize: fontSize.sm, textAlign: "center" },
  retry: { minHeight: 44, justifyContent: "center", paddingHorizontal: space[4] },
  retryLabel: { color: colors.primary, fontSize: fontSize.base, fontWeight: "600" },
  emptyTitle: { color: colors.ink, fontSize: fontSize.lg, fontWeight: "600" },
  badge: {
    alignSelf: "flex-start",
    backgroundColor: colors.surfaceAlt,
    borderColor: colors.border,
    borderRadius: radius.sm,
    borderWidth: 1,
    paddingHorizontal: space[2],
    paddingVertical: 2,
  },
  badgeSuccess: { backgroundColor: colors.success, borderColor: colors.success },
  badgeGold: { backgroundColor: colors.gold, borderColor: colors.gold },
  badgeText: { color: colors.muted, fontSize: fontSize.sm },
  badgeTextLight: { color: colors.surface, fontWeight: "600" },
  chipGroup: { gap: space[1] },
  chipLabel: { color: colors.ink, fontSize: fontSize.sm, fontWeight: "600" },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: space[2] },
  chip: {
    borderColor: colors.border,
    backgroundColor: colors.surface,
    borderRadius: radius.sm,
    borderWidth: 1,
    justifyContent: "center",
    minHeight: 36,
    paddingHorizontal: space[3],
  },
  chipSelected: { backgroundColor: colors.primary, borderColor: colors.primary },
  chipText: { color: colors.ink, fontSize: fontSize.sm },
  chipTextSelected: { color: colors.surface, fontWeight: "600" },
  card: {
    backgroundColor: colors.surface,
    borderColor: colors.border,
    borderRadius: radius.md,
    borderWidth: 1,
    gap: space[3],
    padding: space[6],
  },
  primary: {
    alignItems: "center",
    backgroundColor: colors.primary,
    borderRadius: radius.sm,
    justifyContent: "center",
    minHeight: 50,
    paddingHorizontal: space[4],
  },
  primaryDisabled: { opacity: 0.5 },
  primaryLabel: { color: colors.surface, fontSize: fontSize.base, fontWeight: "700" },
});
