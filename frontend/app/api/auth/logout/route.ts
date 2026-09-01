import { handleLogout } from "@/features/auth"

/** POST only, so a third-party page cannot log the user out with an <img> tag. */
export async function POST() {
  return handleLogout()
}
