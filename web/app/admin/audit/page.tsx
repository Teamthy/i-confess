import { AdminGate } from "@/components/admin";
import { AuditConsole } from "@/components/super-admin";

export const metadata = { title: "Audit Log" };

export default function Page() {
  return (
    <AdminGate>
      <AuditConsole />
    </AdminGate>
  );
}
