import { AppPage } from "@/components/app";
export const metadata = { title: "Community" };
export default async function Page({ params }: { params: Promise<{ slug: string }> }) { const { slug } = await params; return <AppPage kind="communityDetail" slug={slug} />; }
