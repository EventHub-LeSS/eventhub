package service

import (
	"backend/internal/model"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The simulated payment provider. EventHub connects to no real payment provider; this one checks
// the payment details the way a provider would, charges valid ones and declines the provider's
// test data below. Invalid details are rejected before anything changes, so the payer can correct
// them, whereas a decline completes the payment as failed.

var (
	ErrInvalidPaymentMethod  = errors.New("paymentMethod must be credit_card, direct_debit or paypal")
	ErrPaymentMethodRequired = errors.New("paymentMethod is required when payment details are sent")
	ErrInvalidPaymentDetails = errors.New("invalid payment details")
)

// Test data the simulated payment provider declines, like the test data of real providers'
// sandboxes. Every other valid card, IBAN or PayPal account is charged.
const (
	declinedTestCardNumber  = "4000000000000002"
	declinedTestIBAN        = "DE62370400440532013001"
	declinedTestPayPalEmail = "paypal-declined@example.com"
)

// sepaIBANLengths is the IBAN length per country of the SEPA area.
var sepaIBANLengths = map[string]int{
	"AD": 24, "AT": 20, "BE": 16, "BG": 22, "CH": 21, "CY": 28, "CZ": 24, "DE": 22, "DK": 18,
	"EE": 20, "ES": 24, "FI": 18, "FR": 27, "GB": 22, "GI": 23, "GR": 27, "HR": 21, "HU": 28,
	"IE": 22, "IS": 26, "IT": 27, "LI": 21, "LT": 20, "LU": 20, "LV": 21, "MC": 27, "MT": 31,
	"NL": 18, "NO": 15, "PL": 28, "PT": 25, "RO": 24, "SE": 24, "SI": 19, "SK": 24, "SM": 27,
	"VA": 22,
}

var (
	cardNumberPattern = regexp.MustCompile(`^\d{12,19}$`)
	cardExpiryPattern = regexp.MustCompile(`^(0?[1-9]|1[0-2])/(\d{2})$`)
	ibanPattern       = regexp.MustCompile(`^[A-Z]{2}\d{2}[A-Z0-9]+$`)
	cardSeparators    = strings.NewReplacer(" ", "", "-", "")
)

// maxPayPalEmailLength is the longest e-mail address SMTP allows.
const maxPayPalEmailLength = 254

// simulatedCharge is a payment method whose details passed validation, ready to be charged by the
// simulated payment provider.
type simulatedCharge struct {
	method model.PaymentMethod
	// account is the masked account; nil when the payer sent no details, e.g. for a simulated
	// decline or PayPal Express without account.
	account *string
	brand   *model.CardBrand
	// declinedByProvider reports whether the details are test data the provider declines.
	declinedByProvider bool
	transactionID      string
}

// declines reports whether the simulated provider declines the charge: the payer chose the
// failure outcome, or the details are test data the provider declines.
func (c *simulatedCharge) declines(outcome model.SimulatedPaymentOutcome) bool {
	return outcome == model.SimulatedPaymentFailure || c.declinedByProvider
}

// record stores on the payment how it was charged or declined.
func (c *simulatedCharge) record(payment *model.PaymentModel) {
	method, transactionID := c.method, c.transactionID
	payment.PaymentMethod = &method
	payment.PaymentAccount = c.account
	payment.CardBrand = c.brand
	payment.TransactionID = &transactionID
}

// prepareCharge validates the payment method and its details as the payment provider would before
// charging them. It returns nil without a payment method. Card and account data are required to
// pay; a simulated decline needs none, but details that are sent are validated either way. Only the
// details of the chosen method may be sent. now decides whether a card has expired.
func prepareCharge(in *model.PaymentMethodInput, outcome model.SimulatedPaymentOutcome, now time.Time) (*simulatedCharge, error) {
	if in == nil {
		return nil, nil
	}
	if in.Method == "" {
		return nil, ErrPaymentMethodRequired
	}
	if !in.Method.Valid() {
		return nil, ErrInvalidPaymentMethod
	}
	if (in.CreditCard != nil && in.Method != model.PaymentMethodCreditCard) ||
		(in.DirectDebit != nil && in.Method != model.PaymentMethodDirectDebit) ||
		(in.PayPal != nil && in.Method != model.PaymentMethodPayPal) {
		return nil, fmt.Errorf("%w: only the details of %s may be sent", ErrInvalidPaymentDetails, in.Method)
	}

	charge := &simulatedCharge{method: in.Method}
	var err error
	switch in.Method {
	case model.PaymentMethodCreditCard:
		if in.CreditCard == nil {
			if outcome == model.SimulatedPaymentSuccess {
				return nil, fmt.Errorf("%w: creditCard is required to pay by credit card", ErrInvalidPaymentDetails)
			}
			break
		}
		err = charge.prepareCard(in.CreditCard, now)
	case model.PaymentMethodDirectDebit:
		if in.DirectDebit == nil {
			if outcome == model.SimulatedPaymentSuccess {
				return nil, fmt.Errorf("%w: directDebit is required to pay by direct debit", ErrInvalidPaymentDetails)
			}
			break
		}
		err = charge.prepareDirectDebit(in.DirectDebit)
	case model.PaymentMethodPayPal:
		// PayPal Express: the buyer approves the payment at PayPal, so the account is optional.
		if in.PayPal != nil && in.PayPal.Email != "" {
			err = charge.preparePayPal(in.PayPal)
		}
	}
	if err != nil {
		return nil, err
	}
	charge.transactionID = newTransactionID(in.Method)
	return charge, nil
}

func (c *simulatedCharge) prepareCard(card *model.CreditCardDetails, now time.Time) error {
	if utf8.RuneCountInString(strings.TrimSpace(card.Cardholder)) < 2 {
		return fmt.Errorf("%w: cardholder is required", ErrInvalidPaymentDetails)
	}
	number := cardSeparators.Replace(strings.TrimSpace(card.Number))
	if !cardNumberPattern.MatchString(number) {
		return fmt.Errorf("%w: card number must have 12 to 19 digits", ErrInvalidPaymentDetails)
	}
	if !luhnValid(number) {
		return fmt.Errorf("%w: card number is invalid", ErrInvalidPaymentDetails)
	}
	brand, ok := cardBrandOf(number)
	if !ok {
		return fmt.Errorf("%w: only Visa, Mastercard and American Express cards are accepted", ErrInvalidPaymentDetails)
	}
	if err := checkCardExpiry(card.Expiry, now); err != nil {
		return err
	}
	if !cvcValid(strings.TrimSpace(card.CVC), brand) {
		if brand == model.CardBrandAmex {
			return fmt.Errorf("%w: CVC of an American Express card must have 4 digits", ErrInvalidPaymentDetails)
		}
		return fmt.Errorf("%w: CVC must have 3 digits", ErrInvalidPaymentDetails)
	}

	account := "**** " + number[len(number)-4:]
	c.account, c.brand = &account, &brand
	c.declinedByProvider = number == declinedTestCardNumber
	return nil
}

func (c *simulatedCharge) prepareDirectDebit(debit *model.DirectDebitDetails) error {
	if utf8.RuneCountInString(strings.TrimSpace(debit.AccountHolder)) < 2 {
		return fmt.Errorf("%w: account holder is required", ErrInvalidPaymentDetails)
	}
	iban, err := normalizeIBAN(debit.IBAN)
	if err != nil {
		return err
	}
	if !debit.MandateAccepted {
		return fmt.Errorf("%w: the SEPA direct debit mandate must be accepted", ErrInvalidPaymentDetails)
	}

	account := iban[:4] + " **** " + iban[len(iban)-4:]
	c.account = &account
	c.declinedByProvider = iban == declinedTestIBAN
	return nil
}

func (c *simulatedCharge) preparePayPal(payPal *model.PayPalDetails) error {
	email := strings.TrimSpace(payPal.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" || len(email) > maxPayPalEmailLength {
		return fmt.Errorf("%w: PayPal e-mail address is invalid", ErrInvalidPaymentDetails)
	}

	account := maskEmail(email)
	c.account = &account
	c.declinedByProvider = strings.EqualFold(email, declinedTestPayPalEmail)
	return nil
}

// luhnValid reports whether the digits pass the Luhn check of card numbers.
func luhnValid(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// cardBrandOf derives the card network from the number's prefix and length.
func cardBrandOf(number string) (model.CardBrand, bool) {
	n := len(number)
	switch {
	case number[0] == '4' && (n == 13 || n == 16 || n == 19):
		return model.CardBrandVisa, true
	case (strings.HasPrefix(number, "34") || strings.HasPrefix(number, "37")) && n == 15:
		return model.CardBrandAmex, true
	case n == 16 && mastercardPrefix(number):
		return model.CardBrandMastercard, true
	}
	return "", false
}

// mastercardPrefix reports whether the number starts with 51-55 or 2221-2720.
func mastercardPrefix(number string) bool {
	if two := number[:2]; two >= "51" && two <= "55" {
		return true
	}
	four := number[:4]
	return four >= "2221" && four <= "2720"
}

// checkCardExpiry parses an expiry date MM/YY. A card is valid through the end of its expiry month.
func checkCardExpiry(raw string, now time.Time) error {
	match := cardExpiryPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return fmt.Errorf("%w: expiry must be MM/YY", ErrInvalidPaymentDetails)
	}
	// The pattern guarantees one or two digits each.
	month, _ := strconv.Atoi(match[1])
	year, _ := strconv.Atoi(match[2])
	expiresAt := time.Date(2000+year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC)
	if !now.UTC().Before(expiresAt) {
		return fmt.Errorf("%w: card has expired", ErrInvalidPaymentDetails)
	}
	return nil
}

func cvcValid(cvc string, brand model.CardBrand) bool {
	want := 3
	if brand == model.CardBrandAmex {
		want = 4
	}
	if len(cvc) != want {
		return false
	}
	for _, r := range cvc {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// normalizeIBAN removes the spaces of an IBAN, upper-cases it and checks its country, length and
// check digits (ISO 13616, mod 97).
func normalizeIBAN(raw string) (string, error) {
	iban := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if len(iban) < 2 {
		return "", fmt.Errorf("%w: IBAN is required", ErrInvalidPaymentDetails)
	}
	want, ok := sepaIBANLengths[iban[:2]]
	if !ok {
		return "", fmt.Errorf("%w: IBAN must be from the SEPA area", ErrInvalidPaymentDetails)
	}
	if len(iban) != want || !ibanPattern.MatchString(iban) {
		return "", fmt.Errorf("%w: an IBAN from %s has %d characters", ErrInvalidPaymentDetails, iban[:2], want)
	}
	if ibanMod97(iban) != 1 {
		return "", fmt.Errorf("%w: IBAN check digits are invalid", ErrInvalidPaymentDetails)
	}
	return iban, nil
}

// ibanMod97 computes the ISO 7064 remainder of an IBAN: the first four characters move to the
// end and every letter counts as two digits, A = 10 to Z = 35.
func ibanMod97(iban string) int {
	remainder := 0
	for _, r := range iban[4:] + iban[:4] {
		if r >= 'A' && r <= 'Z' {
			remainder = (remainder*100 + int(r-'A') + 10) % 97
		} else {
			remainder = (remainder*10 + int(r-'0')) % 97
		}
	}
	return remainder
}

// maskEmail keeps the first character of the local part and the domain: m***@example.com.
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	first, _ := utf8.DecodeRuneInString(email)
	return string(first) + "***" + email[at:]
}

// newTransactionID returns the reference of a payment attempt, e.g. SIM-CC-4F2A9C1E7B3D. It fits
// a SEPA mandate reference, which allows at most 35 characters.
func newTransactionID(method model.PaymentMethod) string {
	prefix := map[model.PaymentMethod]string{
		model.PaymentMethodCreditCard:  "SIM-CC-",
		model.PaymentMethodDirectDebit: "SIM-DD-",
		model.PaymentMethodPayPal:      "SIM-PP-",
	}[method]
	id := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	return prefix + id[:12]
}
