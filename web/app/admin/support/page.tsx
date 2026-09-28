import { AdminGate } from "@/components/admin";
import { SupportConsole } from "@/components/super-admin";

export const metadata = { title: "Support" };

export default function Page() {
  return (
    <AdminGate>
      <SupportConsole />
    </AdminGate>
  );
}
