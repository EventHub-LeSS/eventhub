import type { NextConfig } from "next"

const nextConfig: NextConfig = {
  output: "standalone",
  experimental: {
    serverActions: {
      // EVENTHUB-264: event images of up to 2 MiB reach the draft actions as a
      // base64 data URL, which is about a third larger than the file itself.
      bodySizeLimit: "4mb",
    },
  },
}

export default nextConfig
