import Link from "next/link";

export const metadata = {
  title: "Offline — ProGuideGH",
};

export default function OfflinePage() {
  return (
    <div className="stack">
      <h1>You are offline</h1>
      <p className="muted">
        ProGuideGH could not reach the network and this page is not cached on
        your device yet. Check your connection and try again.
      </p>
      <p className="nav-actions">
        {/* Plain anchor on purpose: force a full network retry, not a client nav. */}
        <a className="gg-button gg-button--primary" href="/">
          Retry
        </a>
        <Link className="gg-button gg-button--secondary" href="/">
          Home
        </Link>
      </p>
    </div>
  );
}
