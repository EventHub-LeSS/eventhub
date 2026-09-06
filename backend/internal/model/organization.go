package model

type CreateOrganizationRequest struct {
	InternalName string `json:"name" binding:"required"`
	DisplayName  string `json:"alias" binding:"required"`
	OrgAdmin     string `json:"orgAdmin" binding:"required"`
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
