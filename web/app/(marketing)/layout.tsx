import React from "react";
import Header from "@/components/header";
import Footer from "@/components/footer";
export default function MarketingLayout({ children }: { children: React.ReactNode }) {
  // <main> is the page's landmark, and it grows so a short page (legal,
  // contact, 404) keeps the footer at the bottom of the window.
  return (
    <div className="site-shell">
      <Header />
      <main id="main" className="site-main">{children}</main>
      <Footer />
    </div>
  );
}
