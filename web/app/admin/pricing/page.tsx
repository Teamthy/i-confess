import { AdminGate, AdminPricing } from "@/components/admin";

export const metadata = { title: "Pricing" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Pricing</h1>
      <p className="adm-lede">
        Plans are edited here, never hard-coded in an app. Amounts are written in minor units — kobo, cents — because
        that is what the API stores, and every known currency must carry a value.
      </p>
      <AdminGate>
        <AdminPricing />
      </AdminGate>
    </>
  );
}
