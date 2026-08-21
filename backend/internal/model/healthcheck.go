package model

import "time"

type HealthcheckModel struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}
