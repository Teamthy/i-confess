import Link from "next/link";
import type { Category, Confession, Voice } from "@/lib/api";
import { railStyle } from "@/lib/categoryColor";

/**
 * Section furniture: eyebrow + display line + optional lede, with an
 * editorial index number where a band needs one.
 */
export function SectionHead({
  index,
  eyebrow,
  title,
  lede,
  split = false,
  id,
}: {
  index?: string;
  eyebrow: string;
  title: string;
  lede?: string;
  split?: boolean;
  id?: string;
}) {
  return (
    <div className={`ic-section-head ${split ? "ic-section-head--split" : ""}`}>
      <div>
        {index && (
          <span className="ic-index" aria-hidden="true">
            {index}
          </span>
        )}{" "}
        <p className="ic-eyebrow">{eyebrow}</p>
        <h2 className="ic-display" id={id} style={{ marginTop: "var(--ic-spacing-4)" }}>
          {title}
        </h2>
      </div>
      {lede && <p className="ic-lede">{lede}</p>}
    </div>
  );
}

/** A confession preview card linking to its detail page. */
export function ConfessionCard({
  confession,
  category,
}: {
  confession: Confession;
  category?: Category;
}) {
  const text =
    confession.short_text ||
    confession.medium_text ||
    confession.description ||
    "A confession to speak over your life.";
  return (
    <article className="ic-card ic-card--hover ic-confession-card">
      <div className="ic-confession-card__kicker">
        {category && <span>{category.name}</span>}
        {confession.intensity >= 4 && <span aria-hidden="true">·</span>}
        {confession.intensity >= 4 && <span>Intense</span>}
      </div>
      <h3>{confession.title}</h3>
      <p>{text}</p>
      <footer>
        <span>Spoken experience</span>
        <Link href={`/confessions/${confession.id}`} className="ic-btn--text" style={{ fontSize: "var(--ic-font-size-caption)" }}>
          Listen
        </Link>
      </footer>
    </article>
  );
}

/** A voice card. Voices resolve to /voices/{id}; no voice-detail API exists yet (audit §5). */
export function VoiceCard({ voice }: { voice: Voice }) {
  return (
    <Link href={`/voices/${voice.id}`} className="ic-card ic-card--hover ic-voice-card">
      <span className="ic-voice-avatar" aria-hidden="true">
        {voice.name.charAt(0)}
      </span>
      <div>
        <h3>
          {voice.name}
          {voice.premium && (
            <>
              {" "}
              <span className="ic-chip">Premium</span>
            </>
          )}
        </h3>
      </div>
      {voice.description && <p>{voice.description}</p>}
      <footer>
        <span className="ic-cap">{voice.gender || voice.type}</span>
        <span aria-hidden="true">·</span>
        <span>Hear a sample</span>
      </footer>
    </Link>
  );
}

/** A category card used in editorial grids (index page, related rails). */
export function CategoryCard({ category }: { category: Category }) {
  return (
    <Link href={`/categories/${category.slug}`} className="ic-cat-card" style={railStyle(category.slug)}>
      <span className="ic-cat-card__initial" aria-hidden="true">
        {category.name.charAt(0)}
      </span>
      {category.premium && <span className="ic-rail__flag">Premium</span>}
      <h3>{category.name}</h3>
      {category.description && <p>{category.description}</p>}
      <span className="ic-cat-card__go">Explore →</span>
    </Link>
  );
}

/**
 * Loading skeleton that mirrors the three-column confession grid, so layout
 * does not shift when data lands (§51).
 */
export function ConfessionGridSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div className="ic-grid ic-grid--3" aria-hidden="true">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="ic-card ic-confession-card">
          <div className="ic-skeleton" style={{ width: "40%", height: 12 }} />
          <div className="ic-skeleton" style={{ width: "75%", height: 20 }} />
          <div className="ic-skeleton" style={{ height: 14 }} />
          <div className="ic-skeleton" style={{ height: 14, width: "90%" }} />
        </div>
      ))}
    </div>
  );
}

/** Shared human error state (§50). */
export function ErrorState({
  title = "Something went wrong",
  body = "We couldn't load this experience. Please try again.",
  retry,
}: {
  title?: string;
  body?: string;
  retry?: boolean;
}) {
  return (
    <div className="ic-state" role="alert">
      <h3>{title}</h3>
      <p>{body}</p>
      {retry && (
        <a href={typeof window === "undefined" ? "#" : window.location.href} className="ic-btn ic-btn--secondary">
          Try again
        </a>
      )}
    </div>
  );
}
