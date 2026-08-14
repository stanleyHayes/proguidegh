/**
 * Privacy & data (Phase M, M-20) — account deletion, data export and the
 * legal documents. Implementation is shared: see @proguidegh/ui-native/privacy
 * for why each element is a store requirement rather than a design choice.
 */
import { useRouter } from "expo-router";
import { PrivacyScreen } from "@proguidegh/ui-native/privacy";
import { useSession } from "@/lib/session";

export default function Privacy() {
  const { client, signOut } = useSession();
  const router = useRouter();

  return (
    <PrivacyScreen
      client={client}
      onDeleted={() => {
        // The account is gone server-side; drop local tokens and return to
        // the sign-in screen rather than leaving a dead session in memory.
        void signOut();
        router.replace("/login");
      }}
    />
  );
}
