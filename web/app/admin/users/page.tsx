import { AdminGate, AdminUsers } from "@/components/admin";

export const metadata = { title: "People" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">People</h1>
      <p className="adm-lede">
        Admin roles are re-read from the database on every request, so granting and revoking take effect
        immediately — including for a session already in flight.
      </p>
      <AdminGate>
        <AdminUsers />
      </AdminGate>
    </>
  );
}
