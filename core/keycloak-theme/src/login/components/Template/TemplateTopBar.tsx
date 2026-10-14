/**
 * This file has been claimed for ownership from @oussemasahbeni/keycloakify-login-shadcn version 250004.0.24.
 * To relinquish ownership and restore this file to its original content, run the following command:
 *
 * $ npx keycloakify own --path "login/components/Template/TemplateTopBar.tsx" --revert
 */

import { ModeToggle } from "@/login/components/ui/ThemeToggle";
import { redirectUrlOrigin } from "@/login/shared/redirectUrlOrigin";
import { TicketsIcon } from "lucide-react";
import { useI18n } from "../../i18n";
import { useKcContext } from "../../KcContext";
import { Languages } from "../ui/Langauges";

// Mirrors the navbar of the frontend (frontend/features/app/components/navbar) so that
// switching between the app and the login does not feel like leaving EventHub.
export function TemplateTopBar() {
    const { kcContext } = useKcContext();
    const { enabledLanguages } = useI18n();

    return (
        <header className="sticky top-0 z-50 w-full border-b border-border bg-background">
            <div className="mx-auto flex h-14 max-w-screen-xl items-center gap-4 px-4">
                <a
                    href={kcContext.client?.baseUrl ?? redirectUrlOrigin}
                    className="flex shrink-0 items-center gap-2 rounded-lg outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
                >
                    <TicketsIcon className="size-5" />
                    <span className="text-sm font-semibold">
                        {kcContext.properties.SHADCN_THEME_APP_NAME}
                    </span>
                </a>

                <div className="ml-auto flex items-center gap-2">
                    {enabledLanguages.length > 1 && <Languages />}
                    {kcContext.darkMode !== false && <ModeToggle />}
                </div>
            </div>
        </header>
    );
}
