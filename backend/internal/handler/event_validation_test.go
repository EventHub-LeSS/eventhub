package handler

import (
	"backend/internal/model"
	"testing"
	"time"

	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func validUpdateEventRequest() model.UpdateEventRequest {
	return model.UpdateEventRequest{
		Title:      "Sommerkonzert",
		StartTime:  time.Now().Add(48 * time.Hour),
		EndTime:    time.Now().Add(52 * time.Hour),
		Capacity:   100,
		Price:      decimal.NewFromInt(20),
		CategoryID: uuid.New(),
		LocationID: uuid.New(),
	}
}

func TestUpdateEventRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(r *model.UpdateEventRequest)
		wantErr bool
	}{
		{name: "valid request", mutate: func(r *model.UpdateEventRequest) {}},
		{name: "free event", mutate: func(r *model.UpdateEventRequest) { r.Price = decimal.Zero }},
		{name: "title too short", mutate: func(r *model.UpdateEventRequest) { r.Title = "ab" }, wantErr: true},
		{name: "start in the past", mutate: func(r *model.UpdateEventRequest) { r.StartTime = time.Now().Add(-time.Hour) }, wantErr: true},
		{name: "end before start", mutate: func(r *model.UpdateEventRequest) { r.EndTime = r.StartTime.Add(-time.Hour) }, wantErr: true},
		{name: "zero capacity", mutate: func(r *model.UpdateEventRequest) { r.Capacity = 0 }, wantErr: true},
		{name: "negative price", mutate: func(r *model.UpdateEventRequest) { r.Price = decimal.NewFromInt(-1) }, wantErr: true},
		{name: "missing category", mutate: func(r *model.UpdateEventRequest) { r.CategoryID = uuid.Nil }, wantErr: true},
		{name: "missing location", mutate: func(r *model.UpdateEventRequest) { r.LocationID = uuid.Nil }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validUpdateEventRequest()
			tt.mutate(&req)
			err := binding.Validator.ValidateStruct(&req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr=%v, got %v", tt.wantErr, err)
			}
		})
	}
}
