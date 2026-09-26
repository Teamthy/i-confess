import React from "react";
import Header from "@/components/header";
import Footer from "@/components/footer";
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (<><Header /><div className="auth-wrap">{children}</div><Footer /></>);
}
