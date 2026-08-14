/**
 * Re-export of the shared native component set (Phase M).
 *
 * The implementation lives in `@proguidegh/ui-native` so the tourist and guide
 * apps cannot drift. This alias exists so screens keep importing `@/lib/ui`.
 * `@proguidegh/ui` is DOM-only and must never be imported here (§M.3).
 */
export * from "@proguidegh/ui-native";
