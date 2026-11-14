import type { APIRoute } from "astro";

// Points crawlers at the sitemap once SITE_URL gives the build an absolute origin.
export const GET: APIRoute = ({ site }) =>
  new Response(`User-agent: *\nAllow: /\n${site ? `\nSitemap: ${new URL("sitemap-index.xml", site)}\n` : ""}`, {
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
