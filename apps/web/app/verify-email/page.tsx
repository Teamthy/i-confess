import { Suspense } from "react";
import { VerifyEmailPanel } from "./VerifyEmailPanel";

export const metadata = { title: "Verify email", robots: { index: false } };

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={<div className="ic-skeleton" style={{ height: "14rem" }} />}>
      <VerifyEmailPanel />
    </Suspense>
  );
}
