import React from "react";
import Link from "next/link";
import { Logo } from "./ui";

const LINKS: [string, string][] = [["/explore", "Explore"], ["/bible", "Bible"], ["/categories", "Categories"], ["/voices", "Voices"], ["/journal", "Journal"], ["/faq", "FAQ"], ["/about", "About"]];
export default function Footer() {
  return (
    <footer className="site-footer">
      <div className="container-wide">
        <div className="footer-bar">
          <Logo tagline />
          <nav className="footer-links" aria-label="Footer">
            {LINKS.map(([h, l]) => <Link key={l} href={h}>{l}</Link>)}
          </nav>
        </div>
        <div className="footer-bar" style={{ paddingTop: 0, justifyContent: "space-between" }}>
          <span className="footer-copy">© 2026 I CONFESS. All rights reserved.</span>
          <span className="footer-copy"><Link href="/privacy" style={{ marginRight: 14 }}>Privacy</Link><Link href="/terms" style={{ marginRight: 14 }}>Terms</Link><Link href="/cookies">Cookies</Link></span>
        </div>
      </div>
    </footer>
  );
}
