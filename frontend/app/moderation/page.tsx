import { requireModerator } from "@/features/auth"

export default async function ModerationPage() {
  const user = await requireModerator()

  return (
    <div className="flex flex-1 flex-col gap-4 p-6">
      <div>
        <h1 className="font-medium">Moderation</h1>
        <p className="text-sm text-muted-foreground">
          Signed in as {user.name}.
        </p>
      </div>
    </div>
  )
}
