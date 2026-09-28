import BibleRouteLayout from "@/components/bible-layout";

/* Signed-in readers get the product shell and its sidebar; public readers keep
   the site's normal navigation and footer. */
export default function BibleLayout({ children }: { children: React.ReactNode }) {
  return <BibleRouteLayout>{children}</BibleRouteLayout>;
}
