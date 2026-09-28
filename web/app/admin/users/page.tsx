import { AdminGate } from "@/components/admin";
import { SuperAdminUsers } from "@/components/super-admin";

export const metadata = { title: "Users & RBAC" };

export default function Page() {
  return (
    <AdminGate>
      <SuperAdminUsers />
    </AdminGate>
  );
}
