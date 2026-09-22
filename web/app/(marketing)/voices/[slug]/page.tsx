import { VoiceDetail } from "@/components/marketing";
export const metadata = { title: "Voice" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <VoiceDetail slug={slug} />; }
