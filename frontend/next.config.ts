import type { NextConfig } from "next"

const nextConfig: NextConfig = {
  output: "standalone",
  experimental: {
    serverActions: {
      // EVENTHUB-264: event images of up to 2 MiB reach the draft actions as a
      // File, plus the form fields and multipart overhead.
      bodySizeLimit: "3mb",
    },
  },
}

export default nextConfig
