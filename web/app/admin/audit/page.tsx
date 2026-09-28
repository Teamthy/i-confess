import { AdminAudit, AdminGate } from "@/components/admin";

export const metadata = { title: "Audit" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Audit</h1>
      <p className="adm-lede">Security counters and the most recent privileged actions, newest first.</p>
      <AdminGate>
        <AdminAudit />
      </AdminGate>
    </>
  );
}
