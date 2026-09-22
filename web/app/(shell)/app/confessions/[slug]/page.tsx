import { AppPage } from "@/components/app";
export const metadata = { title: "Confession" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <AppPage kind="confession" slug={slug} />; }
