package model

type CreateOrganizationRequest struct {
	InternalName       string  `json:"name" binding:"required"`
	DisplayName        string  `json:"alias" binding:"required"`
	OrgAdmin           string  `json:"orgAdmin" binding:"required"`
	ContactEmail       *string `json:"contactEmail,omitempty"`
	ContactPhoneNumber *string `json:"contactPhoneNumber,omitempty"`
	Street             *string `json:"street,omitempty"`
	HouseNumber        *string `json:"houseNumber,omitempty"`
	PostalCode         *string `json:"postalCode,omitempty"`
	City               *string `json:"city,omitempty"`
	CountryCode        *string `json:"countryCode,omitempty"`
}

type CreateOrganizationResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	Message string `json:"message,omitempty"`
}

type ErrorResponse struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}
