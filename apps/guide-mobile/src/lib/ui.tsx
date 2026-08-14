/**
 * Shared native component set (Phase M).
 *
 * The implementation lives in `@proguidegh/ui-native` so the tourist and guide
 * apps cannot drift. `@proguidegh/ui` is DOM-only and must never be imported
 * here (§M.3).
 */
export * from "@proguidegh/ui-native";
