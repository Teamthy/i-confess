import { AdminGate } from "@/components/admin";
import { SystemHealthConsole } from "@/components/super-admin";

export const metadata = { title: "System" };

export default function Page() {
  return (
    <AdminGate>
      <SystemHealthConsole />
    </AdminGate>
  );
}
