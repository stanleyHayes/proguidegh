import type { MetadataRoute } from "next";
import { getContent, SITE_URL } from "./lib/content";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const { destinations } = await getContent();
  const now = new Date();

  const staticRoutes = [
    { path: "", priority: 1 },
    { path: "/destinations", priority: 0.9 },
    { path: "/become-a-guide", priority: 0.9 },
    { path: "/safety", priority: 0.8 },
    { path: "/pricing", priority: 0.8 },
    { path: "/faq", priority: 0.7 },
    { path: "/about", priority: 0.6 },
    { path: "/contact", priority: 0.5 },
    // Legal routes are listed deliberately: the app stores check that a
    // privacy policy is publicly reachable and indexable.
    { path: "/legal/terms", priority: 0.4 },
    { path: "/legal/privacy", priority: 0.4 },
    { path: "/legal/location", priority: 0.4 },
    { path: "/account/delete", priority: 0.4 },
  ];

  return [
    ...staticRoutes.map((route) => ({
      url: `${SITE_URL}${route.path}`,
      lastModified: now,
      priority: route.priority,
    })),
    ...destinations.map((d) => ({
      url: `${SITE_URL}/destinations/${d.slug}`,
      lastModified: now,
      priority: 0.8,
    })),
  ];
}
