import { SessionDetail } from "@/components/marketing";
export const metadata = { title: "Session" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <SessionDetail slug={slug} />; }
