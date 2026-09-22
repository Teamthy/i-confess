import { AppPage } from "@/components/app";
export const metadata = { title: "Shared with you" };
export default async function Page({ params }: { params: Promise<{ token: string }> }) { const { token } = await params; return <AppPage kind="shared" token={token} />; }
