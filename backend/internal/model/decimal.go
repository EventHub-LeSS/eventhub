package model

import "github.com/shopspring/decimal"

// MaxDecimalExponent bounds the exponent of decimal input such as prices and payment amounts.
// Converting or comparing a decimal materializes 10^|exponent|, so a value like 1e2000000000
// would stall a request for minutes; no price or amount needs an exponent beyond a few digits.
const MaxDecimalExponent = 32

// IsBoundedDecimal reports whether d can be converted and compared without excessive work.
func IsBoundedDecimal(d decimal.Decimal) bool {
	exp := d.Exponent()
	return exp >= -MaxDecimalExponent && exp <= MaxDecimalExponent
}
