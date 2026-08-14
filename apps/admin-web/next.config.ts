import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  transpilePackages: ["@proguidegh/ui", "@proguidegh/contracts"],
};

export default nextConfig;
