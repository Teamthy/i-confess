import React from "react";
import Header from "@/components/header";
import Footer from "@/components/footer";

/* The Bible section is part of the site, not a detached reader: it carries the
   same header and footer as every other public route. */
export default function BibleLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Header />
      {children}
      <Footer />
    </>
  );
}
