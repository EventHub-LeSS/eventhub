/**
 * This file has been claimed for ownership from @oussemasahbeni/keycloakify-login-shadcn version 250004.0.24.
 * To relinquish ownership and restore this file to its original content, run the following command:
 *
 * $ npx keycloakify own --path "login/components/Template/layouts/CenteredCardLayout.tsx" --revert
 */

import type { ReactNode } from "react";
import { TemplateTopBar } from "../TemplateTopBar";

export function CenteredCardLayout(props: { content: ReactNode }) {
    const { content } = props;

    return (
        <div className="flex min-h-svh w-full flex-col">
            <TemplateTopBar />
            <div className="flex flex-1 items-start justify-center px-4 py-8 sm:items-center md:py-12">
                <main className="w-full max-w-md">{content}</main>
            </div>
        </div>
    );
}
