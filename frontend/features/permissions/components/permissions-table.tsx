"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { EllipsisIcon, PencilIcon } from "lucide-react"
import { useState, type FormEvent } from "react"

import {
  globalRoles,
  roleLabels,
  type AdminUser,
  type AdminUsersPage,
  type GlobalRole,
} from "@/features/permissions/lib/roles"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/features/shared/components/ui/dropdown-menu"
import { Label } from "@/features/shared/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/features/shared/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"
import { toast } from "@/features/shared/components/ui/toast"

const usersKey = ["admin", "users"] as const

async function checkResponse(response: Response, fallback: string) {
  if (response.ok) return
  const body = await response.json().catch(() => null)
  throw new Error(typeof body?.detail === "string" ? body.detail : fallback)
}

export function PermissionsTable() {
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<AdminUser | null>(null)
  const [role, setRole] = useState<GlobalRole | null>(null)
  const queryClient = useQueryClient()
  const pageKey = [...usersKey, page] as const

  const users = useQuery({
    queryKey: pageKey,
    queryFn: async (): Promise<AdminUsersPage> => {
      const response = await fetch(`/api/admin/users?page=${page}`, {
        cache: "no-store",
      })
      await checkResponse(
        response,
        "Die Benutzer konnten nicht geladen werden."
      )
      return response.json()
    },
    retry: false,
  })
  const update = useMutation({
    mutationFn: async ({
      userId,
      role: selectedRole,
    }: {
      userId: string
      role: GlobalRole
    }) => {
      const response = await fetch(`/api/admin/users/${userId}/roles`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ roles: [selectedRole] }),
      })
      await checkResponse(
        response,
        "Die Rolle konnte nicht gespeichert werden."
      )
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: usersKey })
    },
  })

  function openEditor(user: AdminUser) {
    if (user.missingInKeycloak) return
    update.reset()
    setRole(user.roles.length === 1 ? user.roles[0] : null)
    setEditing(user)
  }

  const canSave =
    editing && role && (editing.roles.length !== 1 || editing.roles[0] !== role)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!editing || !role || !canSave || update.isPending) return
    const { userId, email } = editing
    update.mutate(
      { userId, role },
      {
        onSuccess: () => {
          queryClient.setQueryData<AdminUsersPage>(
            pageKey,
            (current) =>
              current && {
                ...current,
                items: current.items.map((user) =>
                  user.userId === userId ? { ...user, roles: [role] } : user
                ),
              }
          )
          setEditing(null)
          toast.add({
            type: "success",
            title: `Die Rolle von Benutzer ${email} wurde erfolgreich auf ${roleLabels[role]} geändert.`,
          })
        },
      }
    )
  }

  const data = users.data
  const lastPage = data ? Math.max(1, Math.ceil(data.total / data.limit)) : 1

  return (
    <>
      <section className="flex flex-col gap-4" aria-label="Benutzerverwaltung">
        {users.isPending && (
          <output className="rounded-lg border p-4 text-sm text-muted-foreground">
            Benutzer werden geladen …
          </output>
        )}
        {users.isError && (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4 text-sm">
            <p role="alert" className="text-destructive">
              {users.error.message}
            </p>
            <Button variant="outline" onClick={() => void users.refetch()}>
              Erneut laden
            </Button>
          </div>
        )}
        {data && (
          <>
            <div className="rounded-lg border">
              <Table
                aria-label="Registrierte Benutzer"
                aria-busy={users.isFetching}
              >
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col" className="pl-4">
                      Name
                    </TableHead>
                    <TableHead scope="col">E-Mail</TableHead>
                    <TableHead scope="col">Rollen</TableHead>
                    <TableHead scope="col" className="w-12">
                      <span className="sr-only">Aktionen</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.items.length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={4}
                        className="p-4 text-muted-foreground"
                      >
                        Keine Benutzer auf dieser Seite vorhanden.
                      </TableCell>
                    </TableRow>
                  )}
                  {data.items.map((user) => (
                    <TableRow key={user.userId}>
                      <TableCell className="pl-4 font-medium">
                        {`${user.firstName} ${user.lastName}`.trim() || "–"}
                      </TableCell>
                      <TableCell>{user.email}</TableCell>
                      <TableCell>
                        {user.missingInKeycloak ? (
                          <Badge
                            variant="destructive"
                            title="Der Benutzer existiert nur noch in der Datenbank, nicht mehr in Keycloak. Seine Rolle kann nicht geändert werden."
                          >
                            Nicht in Keycloak vorhanden
                          </Badge>
                        ) : user.roles.length === 0 ? (
                          <span className="text-muted-foreground">
                            Keine Rolle zugewiesen
                          </span>
                        ) : (
                          <div className="flex flex-wrap gap-1">
                            {user.roles.map((assigned) => (
                              <Badge key={assigned} variant="outline">
                                {roleLabels[assigned]}
                              </Badge>
                            ))}
                          </div>
                        )}
                      </TableCell>
                      <TableCell className="pr-4 text-right">
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={<Button variant="ghost" size="icon-sm" />}
                            aria-label={`Aktionen für ${user.email}`}
                          >
                            <EllipsisIcon />
                          </DropdownMenuTrigger>
                          <DropdownMenuContent
                            align="end"
                            className="w-max min-w-48 whitespace-nowrap"
                          >
                            <DropdownMenuItem
                              disabled={user.missingInKeycloak}
                              onClick={() => openEditor(user)}
                            >
                              <PencilIcon />
                              Rolle bearbeiten
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {(lastPage > 1 || page > 1) && (
              <nav
                className="flex items-center justify-end gap-2 text-sm"
                aria-label="Benutzerseiten"
              >
                <Button
                  variant="outline"
                  disabled={page <= 1 || users.isFetching}
                  onClick={() => setPage(page - 1)}
                >
                  Zurück
                </Button>
                <span className="tabular-nums">
                  {page} / {lastPage}
                </span>
                <Button
                  variant="outline"
                  disabled={page >= lastPage || users.isFetching}
                  onClick={() => setPage(page + 1)}
                >
                  Weiter
                </Button>
              </nav>
            )}
          </>
        )}
      </section>

      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open && !update.isPending) setEditing(null)
        }}
      >
        <DialogContent showCloseButton={!update.isPending}>
          {editing && (
            <form className="flex flex-col gap-4" onSubmit={submit}>
              <DialogHeader>
                <DialogTitle>Rolle bearbeiten</DialogTitle>
                <DialogDescription>
                  Globale Rolle für{" "}
                  <span className="wrap-anywhere">{editing.email}</span>{" "}
                  festlegen.
                </DialogDescription>
              </DialogHeader>
              <div className="flex flex-col gap-2">
                <Label htmlFor="user-global-role">Rolle</Label>
                <Select
                  items={roleLabels}
                  value={role}
                  onValueChange={setRole}
                  disabled={update.isPending}
                >
                  <SelectTrigger id="user-global-role" className="w-full">
                    <SelectValue placeholder="Bitte auswählen" />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    {globalRoles.map((option) => (
                      <SelectItem key={option} value={option}>
                        {roleLabels[option]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {update.isError && (
                <p role="alert" className="text-sm text-destructive">
                  {update.error.message}
                </p>
              )}
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  disabled={update.isPending}
                  onClick={() => setEditing(null)}
                >
                  Abbrechen
                </Button>
                <Button type="submit" disabled={!canSave || update.isPending}>
                  {update.isPending
                    ? "Wird überprüft …"
                    : "Änderung bestätigen"}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
