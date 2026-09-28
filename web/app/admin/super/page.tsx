import { AdminGate } from "@/components/admin";
import { SuperAdminOverview } from "@/components/super-admin";

export const metadata = { title: "Super Admin" };

export default function Page() {
  return (
    <AdminGate>
      <SuperAdminOverview />
    </AdminGate>
  );
}
