import { AdminGate } from "@/components/admin";
import { EraseConsole } from "@/components/super-admin";

export const metadata = { title: "Immediate Erasure" };

export default function Page() {
  return (
    <AdminGate>
      <EraseConsole />
    </AdminGate>
  );
}
