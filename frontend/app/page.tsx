import { EventList } from "@/features/catalog"

export default function Page() {
  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Entdecke Veranstaltungen in deiner Region
        </h1>
      </div>
      <EventList />
    </div>
  )
}
