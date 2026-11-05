"use client"

import { CreditCardIcon, LandmarkIcon, LoaderCircleIcon } from "lucide-react"
import { useState } from "react"

import {
  accountHolderError,
  cardCvcError,
  cardExpiryError,
  cardNumberError,
  cardholderError,
  formatCardExpiry,
  formatIban,
  ibanError,
} from "@/features/cart/lib/validation"
import { Button } from "@/features/shared/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import { Input } from "@/features/shared/components/ui/input"
import { cn } from "@/features/shared/lib/utils"

/** The payment methods that need data entered before paying; PayPal Express does not. */
export type FormPaymentMethod = "card" | "sepa"

export type CardDetails = ReturnType<typeof useCardDetails>
export type SepaDetails = ReturnType<typeof useSepaDetails>

/**
 * AK4: the card data of the simulated payment, validated live before the
 * payment can be completed. It never leaves the browser. Untouched fields
 * stay silent; completing the payment simply stays disabled.
 */
export function useCardDetails() {
  const [cardholder, setCardholder] = useState("")
  const [cardNumber, setCardNumber] = useState("")
  const [cardExpiry, setCardExpiry] = useState("")
  const [cardCvc, setCardCvc] = useState("")

  const holderError = cardholderError(cardholder)
  const numberError = cardNumberError(cardNumber)
  const expiryError = cardExpiryError(cardExpiry)
  const cvcError = cardCvcError(cardCvc)
  const filled =
    cardholder.trim() !== "" &&
    cardNumber.trim() !== "" &&
    cardExpiry.trim() !== "" &&
    cardCvc.trim() !== ""

  return {
    cardholder,
    cardNumber,
    cardExpiry,
    cardCvc,
    setCardholder,
    setCardNumber,
    setCardExpiry: (raw: string) => setCardExpiry(formatCardExpiry(raw)),
    setCardCvc,
    holderError,
    numberError,
    expiryError,
    cvcError,
    valid: filled && !holderError && !numberError && !expiryError && !cvcError,
  }
}

/** The direct debit data of the simulated payment; like the card data it never leaves the browser. */
export function useSepaDetails() {
  const [accountHolder, setAccountHolder] = useState("")
  const [iban, setIban] = useState("")

  const holderError = accountHolderError(accountHolder)
  const ibanFieldError = ibanError(iban)
  const filled = accountHolder.trim() !== "" && iban.trim() !== ""

  return {
    accountHolder,
    iban,
    setAccountHolder,
    setIban: (raw: string) => setIban(formatIban(raw)),
    holderError,
    ibanError: ibanFieldError,
    valid: filled && !holderError && !ibanFieldError,
  }
}

const methodOptions: {
  value: FormPaymentMethod
  label: string
  icon: typeof CreditCardIcon
}[] = [
  { value: "card", label: "Kredit- oder Debitkarte", icon: CreditCardIcon },
  { value: "sepa", label: "SEPA-Lastschrift", icon: LandmarkIcon },
]

interface PaymentMethodsProps {
  method: FormPaymentMethod
  onMethodChange: (method: FormPaymentMethod) => void
  card: CardDetails
  sepa: SepaDetails
  disabled: boolean
  onPayPal: () => void
  payPalPending: boolean
}

export function PaymentMethods({
  method,
  onMethodChange,
  card,
  sepa,
  disabled,
  onPayPal,
  payPalPending,
}: PaymentMethodsProps) {
  return (
    <Card className="gap-4 shadow-md">
      <CardHeader>
        <CardTitle className="text-xl font-bold">Zahlungsart</CardTitle>
        <CardDescription>
          Bezahle per Karte, per Lastschrift oder schnell mit PayPal Express.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        <fieldset className="flex flex-col gap-3" disabled={disabled}>
          <legend className="sr-only">Zahlungsart wählen</legend>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {methodOptions.map(({ value, label, icon: Icon }) => (
              <label
                key={value}
                className={cn(
                  "flex cursor-pointer items-center gap-2 rounded-lg border border-border p-3 text-sm font-medium transition-colors hover:bg-muted/60",
                  "has-checked:border-primary has-checked:bg-primary/5 has-focus-visible:ring-2 has-focus-visible:ring-ring/50",
                  disabled && "cursor-not-allowed opacity-50"
                )}
              >
                <input
                  type="radio"
                  name="payment-method"
                  value={value}
                  checked={method === value}
                  onChange={() => onMethodChange(value)}
                  className="sr-only"
                />
                <Icon className="size-4 text-muted-foreground" />
                {label}
              </label>
            ))}
          </div>

          {method === "card" ? (
            <>
              <div className="flex flex-col gap-1.5">
                <Input
                  id="cardholder"
                  aria-label="Karteninhaber"
                  placeholder="Karteninhaber"
                  autoComplete="cc-name"
                  value={card.cardholder}
                  onChange={(changed) =>
                    card.setCardholder(changed.target.value)
                  }
                  aria-invalid={Boolean(card.holderError)}
                  disabled={disabled}
                  className="bg-muted"
                />
                {card.holderError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {card.holderError}
                  </p>
                ) : null}
              </div>
              <div className="flex flex-col gap-1.5">
                <Input
                  id="card-number"
                  aria-label="Kartennummer"
                  placeholder="Kartennummer"
                  inputMode="numeric"
                  autoComplete="cc-number"
                  value={card.cardNumber}
                  onChange={(changed) =>
                    card.setCardNumber(changed.target.value)
                  }
                  aria-invalid={Boolean(card.numberError)}
                  disabled={disabled}
                  className="bg-muted"
                />
                {card.numberError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {card.numberError}
                  </p>
                ) : null}
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="flex flex-col gap-1.5">
                  <Input
                    id="card-expiry"
                    aria-label="Ablaufdatum (MM/JJ)"
                    placeholder="MM/JJ"
                    autoComplete="cc-exp"
                    value={card.cardExpiry}
                    onChange={(changed) =>
                      card.setCardExpiry(changed.target.value)
                    }
                    aria-invalid={Boolean(card.expiryError)}
                    disabled={disabled}
                    className="bg-muted"
                  />
                  {card.expiryError ? (
                    <p className="text-xs text-destructive" role="alert">
                      {card.expiryError}
                    </p>
                  ) : null}
                </div>
                <div className="flex flex-col gap-1.5">
                  <Input
                    id="card-cvc"
                    aria-label="CVC"
                    placeholder="CVC"
                    inputMode="numeric"
                    autoComplete="cc-csc"
                    value={card.cardCvc}
                    onChange={(changed) =>
                      card.setCardCvc(changed.target.value)
                    }
                    aria-invalid={Boolean(card.cvcError)}
                    disabled={disabled}
                    className="bg-muted"
                  />
                  {card.cvcError ? (
                    <p className="text-xs text-destructive" role="alert">
                      {card.cvcError}
                    </p>
                  ) : null}
                </div>
              </div>
            </>
          ) : (
            <>
              <div className="flex flex-col gap-1.5">
                <Input
                  id="account-holder"
                  aria-label="Kontoinhaber"
                  placeholder="Kontoinhaber"
                  autoComplete="name"
                  value={sepa.accountHolder}
                  onChange={(changed) =>
                    sepa.setAccountHolder(changed.target.value)
                  }
                  aria-invalid={Boolean(sepa.holderError)}
                  disabled={disabled}
                  className="bg-muted"
                />
                {sepa.holderError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {sepa.holderError}
                  </p>
                ) : null}
              </div>
              <div className="flex flex-col gap-1.5">
                <Input
                  id="iban"
                  aria-label="IBAN"
                  placeholder="IBAN"
                  autoComplete="off"
                  spellCheck={false}
                  value={sepa.iban}
                  onChange={(changed) => sepa.setIban(changed.target.value)}
                  aria-invalid={Boolean(sepa.ibanError)}
                  disabled={disabled}
                  className="bg-muted font-mono"
                />
                {sepa.ibanError ? (
                  <p className="text-xs text-destructive" role="alert">
                    {sepa.ibanError}
                  </p>
                ) : null}
              </div>
              <p className="text-xs text-muted-foreground">
                Mit dem Bezahlen ermächtigst du EventHub, den Betrag einmalig
                per SEPA-Lastschrift von diesem Konto einzuziehen (Simulation).
              </p>
            </>
          )}
        </fieldset>

        <div
          className="flex items-center gap-3 text-xs text-muted-foreground"
          aria-hidden="true"
        >
          <span className="h-px flex-1 bg-border" />
          oder
          <span className="h-px flex-1 bg-border" />
        </div>

        <div className="flex flex-col gap-2">
          <Button
            type="button"
            onClick={onPayPal}
            disabled={disabled}
            className="h-10 w-full bg-[#ffc439] text-[#003087] hover:bg-[#f2b630]"
          >
            {payPalPending ? (
              <>
                <LoaderCircleIcon className="animate-spin" />
                Verarbeite …
              </>
            ) : (
              <>
                Express Checkout mit
                <span className="font-extrabold italic">PayPal</span>
              </>
            )}
          </Button>
          <p className="text-xs text-muted-foreground">
            Simulation: PayPal Express schließt die Zahlung sofort erfolgreich
            ab.
          </p>
        </div>
      </CardContent>
    </Card>
  )
}
