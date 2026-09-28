import { AdminContent, AdminGate } from "@/components/admin";

export const metadata = { title: "Content" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Content</h1>
      <p className="adm-lede">
        The lifecycle is a forward-only graph the API enforces: draft → content review → theological review → audio
        production → audio QA → approved → published. An illegal move is refused by the server, not hidden here.
      </p>
      <AdminGate>
        <AdminContent />
      </AdminGate>
    </>
  );
}
