import { ConfessionDetail } from "@/components/marketing";
export const metadata = { title: "Confession" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <ConfessionDetail slug={slug} />; }
