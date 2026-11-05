/**
 * This file has been claimed for ownership from @oussemasahbeni/keycloakify-login-shadcn version 250004.0.24.
 * To relinquish ownership and restore this file to its original content, run the following command:
 *
 * $ npx keycloakify own --path "login/components/Template/TemplateTopBar.tsx" --revert
 */

import { Button } from "@/components/ui/button";
import { ModeToggle } from "@/login/components/ui/ThemeToggle";
import { FiHome } from "react-icons/fi";
import { useI18n } from "../../i18n";
import { useKcContext } from "../../KcContext";
import { Languages } from "../ui/Langauges";

const SESSION_STORAGE_KEY = "redirectUrlOrigin";

/**
 * Resolves where the home button should take the user back to, or undefined when
 * no destination is known.
 *
 * Never render `href="#"` or `href=""`: the Keycloakify template injects
 * `<base href=".../login/<theme name>/dist/">` into the page, so such an href makes
 * the browser navigate to the theme resources directory. Keycloak serves that as an
 * empty `application/octet-stream` response, meaning the user gets a 0-byte
 * "Download" file instead of a navigation. When no destination can be determined,
 * the button is simply not rendered.
 */
function getHomeUrl(clientBaseUrl: string | undefined): string | undefined {
    // The realm can define where the client lives (Keycloak "rootUrl"/"baseUrl").
    if (clientBaseUrl) {
        return clientBaseUrl;
    }

    // Otherwise, send the user back to the app that initiated the login. The
    // `redirect_uri` param is only present on the first page of the flow, so the
    // origin is cached in sessionStorage for the pages that follow.
    from_url: {
        const url = new URL(window.location.href);

        const value = url.searchParams.get("redirect_uri") ?? url.searchParams.get("redirect_url");

        if (value === null) {
            break from_url;
        }

        try {
            const { origin } = new URL(value, url);

            // Opaque origin, e.g. a redirect to a native app deep link.
            if (origin === "null") {
                break from_url;
            }

            sessionStorage.setItem(SESSION_STORAGE_KEY, origin);

            return origin;
        } catch {
            break from_url;
        }
    }

    return sessionStorage.getItem(SESSION_STORAGE_KEY) ?? undefined;
}

export function TemplateTopBar() {
    const { kcContext } = useKcContext();
    const { enabledLanguages } = useI18n();

    const homeUrl = getHomeUrl(kcContext.client?.baseUrl);

    return (
        <div className="absolute inset-x-4 top-4 z-20 flex items-center gap-2">
            {homeUrl !== undefined && (
                <Button type="button" variant="outline" size="icon" asChild>
                    <a href={homeUrl} aria-label="Home">
                        <FiHome />
                    </a>
                </Button>
            )}

            {kcContext.darkMode !== false && <ModeToggle />}

            {enabledLanguages.length > 1 && <Languages />}
        </div>
    );
}
