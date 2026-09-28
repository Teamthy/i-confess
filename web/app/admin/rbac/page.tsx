import { AdminGate } from "@/components/admin";
import { RBACConsole } from "@/components/super-admin";

export const metadata = { title: "RBAC" };

export default function Page() {
  return (
    <AdminGate>
      <RBACConsole />
    </AdminGate>
  );
}
