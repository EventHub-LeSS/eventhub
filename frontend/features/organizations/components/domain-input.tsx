"use client";

import { PlusIcon, XIcon } from "lucide-react";
import { useState } from "react";

import { isValidDomain } from "@/features/organizations/lib/organization";
import { Button } from "@/features/shared/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/features/shared/components/ui/field";
import { Input } from "@/features/shared/components/ui/input";

interface DomainInputProps {
  domains: string[];
  onChange: (domains: string[]) => void;
}

export function DomainInput({ domains, onChange }: DomainInputProps) {
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  function add() {
    const domain = draft.trim().toLowerCase().replace(/^@/, "");

    if (!domain) {
      return;
    }

    if (!isValidDomain(domain)) {
      setError("Enter a domain like fs-inf.uni-example.de.");
      return;
    }

    if (domains.includes(domain)) {
      setError("That domain is already listed.");
      return;
    }

    onChange([...domains, domain]);
    setDraft("");
    setError(null);
  }

  return (
    <Field data-invalid={Boolean(error)}>
      <FieldLabel htmlFor="organization-domains">Email domains</FieldLabel>
      <div className="flex gap-2">
        <Input
          id="organization-domains"
          value={draft}
          placeholder="fs-inf.uni-example.de"
          aria-invalid={Boolean(error)}
          onChange={(event) => {
            setDraft(event.target.value);
            setError(null);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              add();
            }
          }}
        />
        <Button type="button" variant="outline" onClick={add}>
          <PlusIcon />
          Add
        </Button>
      </div>
      <FieldDescription>
        Members signing in with an address at these domains are routed to this
        organization. Optional.
      </FieldDescription>
      <FieldError>{error}</FieldError>
      {domains.length > 0 ? (
        <ul className="flex flex-wrap gap-1.5 pt-1">
          {domains.map((domain) => (
            <li key={domain}>
              <span className="inline-flex items-center gap-1 rounded-lg border border-border bg-muted/50 py-0.5 pr-1 pl-2 text-xs">
                {domain}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`Remove ${domain}`}
                  onClick={() =>
                    onChange(domains.filter((entry) => entry !== domain))
                  }
                >
                  <XIcon />
                </Button>
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </Field>
  );
}
