import { AdminGate } from "@/components/admin";
import { BibleAdminConsole } from "@/components/super-admin";

export const metadata = { title: "Bible Platform" };

export default function Page() {
  return (
    <AdminGate>
      <BibleAdminConsole />
    </AdminGate>
  );
}
