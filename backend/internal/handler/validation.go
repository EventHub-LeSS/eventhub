package handler

import (
	"reflect"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"
)

// Lets gin's validator apply numeric rules such as gte=0 to decimal.Decimal fields.
func init() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	v.RegisterCustomTypeFunc(func(field reflect.Value) any {
		if d, ok := field.Interface().(decimal.Decimal); ok {
			f, _ := d.Float64()
			return f
		}
		return nil
	}, decimal.Decimal{})
}
