/**
 * This file has been claimed for ownership from @oussemasahbeni/keycloakify-login-shadcn version 250004.0.24.
 * To relinquish ownership and restore this file to its original content, run the following command:
 * 
 * $ npx keycloakify own --path "components/ui/alert.tsx" --revert
 */

import { cn } from "@/components/lib/utils";
import { cva, type VariantProps } from "class-variance-authority";
import { AlertTriangle, Info, XCircle } from "lucide-react";
import * as React from "react";
import { MdCheckCircle } from "react-icons/md";

const alertVariants = cva(
    "relative w-full rounded-lg border px-3 py-2.5 text-sm flex items-start gap-2.5 transition-all",
    {
        variants: {
            variant: {
                // Only info is muted. Errors, warnings and success keep their color in the text, like the frontend shows them.
                info: "bg-card text-card-foreground **:data-[slot=alert-description]:text-muted-foreground",
                success: "bg-emerald-50/50 border-emerald-200 text-emerald-900 dark:bg-emerald-500/5 dark:border-emerald-500/20 dark:text-emerald-400",
                warning: "bg-amber-50/50 border-amber-200 text-amber-900 dark:bg-amber-500/5 dark:border-amber-500/20 dark:text-amber-400",
                error: "bg-destructive/10 border-destructive/30 text-destructive dark:bg-destructive/20",
            },
        },
        defaultVariants: {
            variant: "info",
        },
    }
);

interface AlertProps
    extends React.ComponentProps<"div">,
    VariantProps<typeof alertVariants> {
    showIcon?: boolean;
}

function Alert({
    className,
    variant = "info",
    showIcon = true,
    children,
    ...props
}: AlertProps) {
    const Icon = {
        info: Info,
        error: XCircle,
        warning: AlertTriangle,
        success: MdCheckCircle,
    }[variant as string] || Info;

    return (
        <div
            data-slot="alert"
            role="alert"
            className={cn(alertVariants({ variant }), className)}
            {...props}
        >
            {showIcon && <Icon className="mt-0.5 size-4 shrink-0" data-slot="alert-icon" />}
            {/* We use a div wrapper for children to ensure Title and Description stack vertically */}
            <div className="flex flex-col gap-0.5 flex-1">
                {children}
            </div>
        </div>
    );
}

function AlertTitle({ className, ...props }: React.ComponentProps<"div">) {
    return (
        <div
            data-slot="alert-title"
            className={cn(
                "font-medium leading-none tracking-tight",
                className
            )}
            {...props}
        />
    );
}

function AlertDescription({
    className,
    ...props
}: React.ComponentProps<"div">) {
    return (
        <div
            data-slot="alert-description"
            className={cn(
                "text-sm [&_p]:leading-relaxed",
                className
            )}
            {...props}
        />
    );
}

export { Alert, AlertDescription, AlertTitle };
