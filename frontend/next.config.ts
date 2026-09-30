import type { NextConfig } from "next";

// 1. READ THE INTERNAL BACKEND ADDRESS
const backendURL = process.env.BACKEND_URL;

if (!backendURL) {
  throw new Error("BACKEND_URL is required");
}

const nextConfig: NextConfig = {
  // 2. FORWARD API REQUESTS TO GO
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${backendURL}/:path*`,
      },
    ];
  },
};

export default nextConfig;
