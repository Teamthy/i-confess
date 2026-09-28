import { AdminGate } from "@/components/admin";
import { ModerationConsole } from "@/components/super-admin";

export const metadata = { title: "Moderation" };

export default function Page() {
  return (
    <AdminGate>
      <ModerationConsole />
    </AdminGate>
  );
}
