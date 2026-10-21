import { CirclePlusIcon } from "lucide-react";
import Link from "next/link";

import { requireSession } from "@/features/auth";
import { Button } from "@/features/shared/components/ui/button";

export default async function OrganizationsPage() {
  const user = await requireSession();

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-medium">Organisationen</h1>
          <p className="text-sm text-muted-foreground">
            Organisationen, in denen du Mitglied bist.
          </p>
        </div>
        <Button render={<Link href="/organizations/create" />} nativeButton={false}>
          <CirclePlusIcon />
          Organisation erstellen
        </Button>
      </div>

      {user.organizations.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Du bist noch in keiner Organisation Mitglied.
        </p>
      ) : (
        <ul className="flex flex-col gap-2">
          {user.organizations.map((organization) => (
            <li
              key={organization}
              className="flex items-center justify-between rounded-lg border p-4"
            >
              <span className="text-sm font-medium">{organization}</span>
              {organization === user.activeOrganization && (
                <span className="text-xs text-muted-foreground">Aktiv</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
