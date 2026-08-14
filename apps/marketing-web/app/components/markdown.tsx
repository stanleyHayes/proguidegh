import type { ReactNode } from "react";

/**
 * Minimal markdown renderer for legal documents.
 *
 * Renders to React elements rather than an HTML string, so there is no
 * `dangerouslySetInnerHTML` and therefore no injection surface — which matters
 * because this text is admin-editable and will eventually be edited by people
 * who are not engineers.
 *
 * Supports exactly the subset the legal documents use: `##`/`###` headings,
 * paragraphs, `-` bullet lists, `1.` ordered lists, `**bold**`, `*italic*` and
 * `[text](url)`. Anything else renders as literal text, which is the safe
 * failure: an unsupported construct looks wrong rather than disappearing.
 */

/** Split a line into text and inline formatting. */
function inline(text: string, keyPrefix: string): ReactNode[] {
  const out: ReactNode[] = [];
  // One pass over bold, italic and links, longest-delimiter first so that
  // `**` is never mistaken for two `*`.
  const pattern = /(\*\*[^*]+\*\*|\*[^*]+\*|\[[^\]]+\]\([^)]+\))/g;
  let last = 0;
  let match: RegExpExecArray | null;
  let i = 0;

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > last) out.push(text.slice(last, match.index));
    const token = match[0];
    const key = `${keyPrefix}-i${i++}`;

    if (token.startsWith("**")) {
      out.push(<strong key={key}>{token.slice(2, -2)}</strong>);
    } else if (token.startsWith("[")) {
      const split = token.indexOf("](");
      const label = token.slice(1, split);
      const href = token.slice(split + 2, -1);
      // Only http(s) and mailto survive; anything else renders as plain text
      // so an editor cannot introduce a javascript: URL.
      const safe = /^(https?:|mailto:|\/)/i.test(href);
      out.push(
        safe ? (
          <a href={href} key={key} style={{ color: "var(--green)" }}>
            {label}
          </a>
        ) : (
          <span key={key}>{label}</span>
        ),
      );
    } else {
      out.push(<em key={key}>{token.slice(1, -1)}</em>);
    }
    last = match.index + token.length;
  }

  if (last < text.length) out.push(text.slice(last));
  return out;
}

export function Markdown({ source }: { source: string }) {
  const blocks: ReactNode[] = [];
  const lines = source.replace(/\r\n/g, "\n").split("\n");

  let paragraph: string[] = [];
  let bullets: string[] = [];
  let ordered: string[] = [];
  let key = 0;

  function flushParagraph() {
    if (paragraph.length === 0) return;
    const text = paragraph.join(" ").trim();
    paragraph = [];
    if (text !== "") {
      blocks.push(
        <p className="doc__p" key={`p${key}`}>
          {inline(text, `p${key++}`)}
        </p>,
      );
    }
  }

  function flushBullets() {
    if (bullets.length === 0) return;
    const items = bullets;
    bullets = [];
    blocks.push(
      <ul className="doc__list" key={`u${key}`}>
        {items.map((item, index) => (
          <li key={index}>{inline(item, `u${key}-${index}`)}</li>
        ))}
      </ul>,
    );
    key += 1;
  }

  function flushOrdered() {
    if (ordered.length === 0) return;
    const items = ordered;
    ordered = [];
    blocks.push(
      <ol className="doc__list" key={`o${key}`}>
        {items.map((item, index) => (
          <li key={index}>{inline(item, `o${key}-${index}`)}</li>
        ))}
      </ol>,
    );
    key += 1;
  }

  function flushAll() {
    flushParagraph();
    flushBullets();
    flushOrdered();
  }

  for (const raw of lines) {
    const line = raw.trimEnd();

    if (line.trim() === "") {
      flushAll();
      continue;
    }
    if (line.startsWith("### ")) {
      flushAll();
      blocks.push(
        <h3 className="doc__h3" key={`h3${key++}`}>
          {line.slice(4)}
        </h3>,
      );
      continue;
    }
    if (line.startsWith("## ")) {
      flushAll();
      blocks.push(
        <h2 className="doc__h2" key={`h2${key++}`}>
          {line.slice(3)}
        </h2>,
      );
      continue;
    }
    if (/^- /.test(line.trim())) {
      flushParagraph();
      flushOrdered();
      bullets.push(line.trim().slice(2));
      continue;
    }
    if (/^\d+\.\s/.test(line.trim())) {
      flushParagraph();
      flushBullets();
      ordered.push(line.trim().replace(/^\d+\.\s/, ""));
      continue;
    }
    flushBullets();
    flushOrdered();
    paragraph.push(line.trim());
  }
  flushAll();

  return <div className="doc">{blocks}</div>;
}
