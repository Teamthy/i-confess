/**
 * Site-wide constants shared by server and client modules. Kept out of
 * app/layout.tsx so client components never import a route module.
 */
export const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://iconfess.app";
export const SITE_NAME = "iCONFESS";
export const SUPPORT_EMAIL = "support@iconfess.app";
