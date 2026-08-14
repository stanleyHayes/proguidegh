type FooterIconName = "search" | "pin" | "safety" | "price" | "badge" | "about" | "mail" | "help" | "terms" | "privacy" | "login" | "partner";

const paths: Record<FooterIconName, React.ReactNode> = {
  search: <><circle cx="10.5" cy="10.5" r="5.5"/><path d="m15 15 4 4"/></>,
  pin: <><path d="M12 21s6-5.2 6-11a6 6 0 1 0-12 0c0 5.8 6 11 6 11Z"/><circle cx="12" cy="10" r="2"/></>,
  safety: <><path d="M12 3 5 6v5c0 4.6 2.8 8.1 7 10 4.2-1.9 7-5.4 7-10V6l-7-3Z"/><path d="m9 12 2 2 4-5"/></>,
  price: <><path d="M4 7.5V4h3.5L20 16.5 16.5 20 4 7.5Z"/><circle cx="7.5" cy="7.5" r="1"/></>,
  badge: <><circle cx="12" cy="9" r="5"/><path d="m8.5 13-1 8 4.5-2.5 4.5 2.5-1-8"/></>,
  about: <><circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.1"/></>,
  mail: <><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m4 7 8 6 8-6"/></>,
  help: <><circle cx="12" cy="12" r="9"/><path d="M9.8 9a2.4 2.4 0 1 1 3.2 2.3c-.7.3-1 1-1 1.7M12 17h.01"/></>,
  terms: <><path d="M6 3h9l3 3v15H6V3Z"/><path d="M14 3v4h4M9 12h6M9 16h6"/></>,
  privacy: <><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3"/></>,
  login: <><path d="M10 5H5v14h5M14 8l4 4-4 4M8 12h10"/></>,
  partner: <><path d="M8.5 12.5 5 9l-3 3 5.5 5.5a2 2 0 0 0 2.8 0l1.2-1.2M15.5 12.5 19 9l3 3-5.5 5.5a2 2 0 0 1-2.8 0L8 11.8a2 2 0 0 1 0-2.8l1-1a2 2 0 0 1 2.8 0l1.2 1.2"/></>,
};

export function FooterIcon({ name }: { name: FooterIconName }) {
  return <span className="foot__icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg></span>;
}
