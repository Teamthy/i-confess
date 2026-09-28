import { AdminGate, AdminOverview } from "@/components/admin";

export const metadata = { title: "Overview" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Platform overview</h1>
      <AdminGate>
        <AdminOverview />
      </AdminGate>
    </>
  );
}
