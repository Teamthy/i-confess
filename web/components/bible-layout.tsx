"use client";

import React, { useEffect, useState } from "react";
import Header from "@/components/header";
import Footer from "@/components/footer";
import { AppShell } from "@/components/app";
import { useApp } from "@/lib/store";

/** Keep public Bible routes in the marketing frame for visitors, but preserve
 * the authenticated app navigation when a signed-in reader opens /bible. */
export default function BibleRouteLayout({ children }: { children: React.ReactNode }) {
  const { user } = useApp();
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => setHydrated(true), []);

  if (hydrated && user) return <AppShell>{children}</AppShell>;
  return (
    <>
      <Header />
      {children}
      <Footer />
    </>
  );
}
