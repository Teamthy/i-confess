import { AdminGate } from "@/components/admin";
import { SecurityConsole } from "@/components/super-admin";

export const metadata = { title: "Security" };

export default function Page() {
  return (
    <AdminGate>
      <SecurityConsole />
    </AdminGate>
  );
}
