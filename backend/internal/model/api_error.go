package model

type APIError struct {
	Error APIErrorDetail `json:"error"`
}

type APIErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
