package handler

import (
	"math"
	"reflect"

	"backend/internal/model"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"
)

// Lets gin's validator apply numeric rules such as gte=0 to decimal.Decimal fields
// and validates organization role names in request models.
func init() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	v.RegisterCustomTypeFunc(func(field reflect.Value) any {
		if d, ok := field.Interface().(decimal.Decimal); ok {
			// Float64 would materialize 10^|exponent|; NaN fails every numeric rule instead.
			if !model.IsBoundedDecimal(d) {
				return math.NaN()
			}
			f, _ := d.Float64()
			return f
		}
		return nil
	}, decimal.Decimal{})
	if err := v.RegisterValidation("org_role", func(fl validator.FieldLevel) bool {
		return model.IsValidOrganizationRole(fl.Field().String())
	}); err != nil {
		panic("register org_role validation: " + err.Error())
	}
}
