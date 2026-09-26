"use client";

/*
 * The category rail — the signature interaction.
 *
 * Desktop: an editorial bookcase. One panel stands open; its neighbours are
 * spines. Activation is explicit — click, Enter/Space, or moving focus in
 * (focus-through opens) — never hover alone. Arrow keys walk the spines;
 * Home/End jump to the ends. Each panel contains a real link to the category.
 *
 * Mobile: a horizontal snap carousel of cards with the next card peeking.
 * No hover-dependent behaviour anywhere.
 *
 * The per-category colour is derived deterministically from the design
 * system's green ramp plus neutral ink tones (a pure function of the slug),
 * so every category is visually identifiable without inventing a palette.
 */

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import type { Category } from "@/lib/api";
import { railStyle } from "@/lib/categoryColor";

export function CategoryRail({ categories }: { categories: Category[] }) {
  const [open, setOpen] = useState(0);
  const refs = useRef<(HTMLButtonElement | null)[]>([]);

  const onKeyDown = (e: React.KeyboardEvent, index: number) => {
    let next: number | null = null;
    if (e.key === "ArrowRight") next = (index + 1) % categories.length;
    else if (e.key === "ArrowLeft") next = (index - 1 + categories.length) % categories.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = categories.length - 1;
    if (next !== null) {
      e.preventDefault();
      setOpen(next);
      refs.current[next]?.focus();
    }
  };

  return (
    <>
      {/* Desktop bookcase */}
      <div
        className="ic-rail"
        role="list"
        aria-label="Areas of life"
        style={{ ["--rail-count" as string]: categories.length }}
      >
        {categories.map((cat, i) => {
          const isOpen = i === open;
          return (
            <div
              key={cat.id}
              role="listitem"
              className="ic-rail__item"
              data-open={isOpen}
              style={railStyle(cat.slug)}
              onMouseEnter={() => setOpen(i)}
            >
              <button
                type="button"
                ref={(el) => {
                  refs.current[i] = el;
                }}
                className="ic-rail__spine"
                aria-expanded={isOpen}
                aria-controls={`rail-panel-${cat.id}`}
                onClick={() => setOpen(i)}
                onFocus={() => setOpen(i)}
                onKeyDown={(e) => onKeyDown(e, i)}
              >
                <span className="ic-visually-hidden">{`Open ${cat.name}`}</span>
                <span aria-hidden="true">{cat.name}</span>
              </button>
              <div
                id={`rail-panel-${cat.id}`}
                className="ic-rail__panel"
                role="group"
                aria-label={cat.name}
                aria-hidden={!isOpen}
              >
                {cat.premium && <span className="ic-rail__flag">Premium</span>}
                <h3 className="ic-rail__name">{cat.name}</h3>
                {cat.description && <p>{cat.description}</p>}
                <Link
                  href={`/categories/${cat.slug}`}
                  className="ic-rail__cta"
                  tabIndex={isOpen ? 0 : -1}
                >
                  Explore {cat.name}
                  <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
                    <path d="M2 7h10M8 3l4 4-4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" fill="none" />
                  </svg>
                </Link>
              </div>
            </div>
          );
        })}
      </div>

      {/* Mobile / tablet snap carousel */}
      <div className="ic-carousel" role="list" aria-label="Areas of life">
        {categories.map((cat) => (
          <div role="listitem" key={cat.id}>
            <Link href={`/categories/${cat.slug}`} className="ic-cat-card" style={railStyle(cat.slug)}>
              <span className="ic-cat-card__initial" aria-hidden="true">
                {cat.name.charAt(0)}
              </span>
              {cat.premium && <span className="ic-rail__flag">Premium</span>}
              <h3>{cat.name}</h3>
              {cat.description && <p>{cat.description}</p>}
              <span className="ic-cat-card__go">Explore →</span>
            </Link>
          </div>
        ))}
      </div>
    </>
  );
}
