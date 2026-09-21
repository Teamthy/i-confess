import Link from "next/link";
import { Wordmark } from "./Wordmark";
import { SITE_URL } from "@/lib/site";

const PRODUCT = [
  { href: "/explore", label: "Explore" },
  { href: "/categories", label: "Categories" },
  { href: "/sessions", label: "Sessions" },
  { href: "/voices", label: "Voices" },
  { href: "/premium", label: "Premium" },
  { href: "/download", label: "Download" },
];

const COMPANY = [
  { href: "/about", label: "About" },
  { href: "/community", label: "Community" },
  { href: "/journal", label: "Journal" },
  { href: "/contact", label: "Contact" },
];

const LEGAL = [
  { href: "/privacy", label: "Privacy" },
  { href: "/terms", label: "Terms" },
  { href: "/cookies", label: "Cookies" },
  { href: "/community-guidelines", label: "Community guidelines" },
];

/**
 * The footer is the final chapter: the wordmark large, one brand sentence,
 * three link columns, legal line. No clutter.
 */
export function SiteFooter() {
  return (
    <footer className="ic-footer">
      <div className="ic-container">
        <div className="ic-footer__grid">
          <div>
            <Wordmark onDark />
            <p className="ic-footer__statement">
              A quiet place to speak what you believe — and to hear it again
              until it becomes part of you.
            </p>
          </div>
          <div className="ic-footer__col">
            <h3>Product</h3>
            <ul role="list">
              {PRODUCT.map((l) => (
                <li key={l.href}>
                  <Link href={l.href}>{l.label}</Link>
                </li>
              ))}
            </ul>
          </div>
          <div className="ic-footer__col">
            <h3>Company</h3>
            <ul role="list">
              {COMPANY.map((l) => (
                <li key={l.href}>
                  <Link href={l.href}>{l.label}</Link>
                </li>
              ))}
            </ul>
          </div>
          <div className="ic-footer__col">
            <h3>Legal</h3>
            <ul role="list">
              {LEGAL.map((l) => (
                <li key={l.href}>
                  <Link href={l.href}>{l.label}</Link>
                </li>
              ))}
            </ul>
          </div>
        </div>
        <div className="ic-footer__legal">
          <p>© {new Date().getFullYear()} iCONFESS. All rights reserved.</p>
          <p>
            <a href={`${SITE_URL}/terms`} rel="license">
              Terms
            </a>{" "}
            · Written content is reviewed before publication. Your private
            confessions stay private.
          </p>
        </div>
      </div>
    </footer>
  );
}
