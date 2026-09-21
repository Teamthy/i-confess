import { Suspense } from "react";
import { ResetForm } from "./ResetForm";

export const metadata = { title: "Reset password", robots: { index: false } };

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={<div className="ic-skeleton" style={{ height: "16rem" }} />}>
      <ResetForm />
    </Suspense>
  );
}
