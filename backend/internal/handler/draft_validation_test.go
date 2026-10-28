package handler

import (
	"strings"
	"testing"
	"time"

	"backend/internal/model"

	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// validDraftRequest liefert einen Request, der alle Regeln erfuellt.
// Jeder Testfall veraendert daran genau eine Sache.
func validDraftRequest() model.CreateDraftRequest {
	return model.CreateDraftRequest{
		Title:      "Sommerfest 2026",
		StartTime:  time.Now().Add(48 * time.Hour),
		EndTime:    time.Now().Add(52 * time.Hour),
		Capacity:   100,
		Price:      decimal.NewFromInt(15),
		CategoryID: uuid.New(),
		Location:   "Stadthalle, Markt 1, 53111 Bonn",
	}
}

// EVENTHUB-77 / AK1: Eingabepruefung beim Speichern eines Entwurfs.
func TestCreateDraftRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(r *model.CreateDraftRequest)
		wantErr bool
	}{
		{name: "gueltiger request", mutate: func(r *model.CreateDraftRequest) {}},
		{name: "kostenlose veranstaltung", mutate: func(r *model.CreateDraftRequest) { r.Price = decimal.Zero }},
		{name: "titel zu kurz", mutate: func(r *model.CreateDraftRequest) { r.Title = "ab" }, wantErr: true},
		{name: "titel fehlt", mutate: func(r *model.CreateDraftRequest) { r.Title = "" }, wantErr: true},
		{name: "start in der vergangenheit", mutate: func(r *model.CreateDraftRequest) { r.StartTime = time.Now().Add(-time.Hour) }, wantErr: true},
		{name: "ende vor start", mutate: func(r *model.CreateDraftRequest) { r.EndTime = r.StartTime.Add(-time.Hour) }, wantErr: true},
		{name: "kapazitaet null", mutate: func(r *model.CreateDraftRequest) { r.Capacity = 0 }, wantErr: true},
		{name: "kapazitaet zu gross", mutate: func(r *model.CreateDraftRequest) { r.Capacity = 100001 }, wantErr: true},
		{name: "negativer preis", mutate: func(r *model.CreateDraftRequest) { r.Price = decimal.NewFromInt(-1) }, wantErr: true},
		{name: "preis zu hoch", mutate: func(r *model.CreateDraftRequest) { r.Price = decimal.NewFromInt(10001) }, wantErr: true},
		{name: "kategorie fehlt", mutate: func(r *model.CreateDraftRequest) { r.CategoryID = uuid.Nil }, wantErr: true},
		{name: "ort fehlt", mutate: func(r *model.CreateDraftRequest) { r.Location = "" }, wantErr: true},
		{name: "ort nur leerzeichen", mutate: func(r *model.CreateDraftRequest) { r.Location = " \t\n " }, wantErr: true},
		{name: "ort mit 200 zeichen", mutate: func(r *model.CreateDraftRequest) { r.Location = strings.Repeat("ä", 200) }},
		{name: "ort zu lang", mutate: func(r *model.CreateDraftRequest) { r.Location = strings.Repeat("ä", 201) }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validDraftRequest()
			tt.mutate(&req)

			err := binding.Validator.ValidateStruct(&req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr=%v, bekommen: %v", tt.wantErr, err)
			}
		})
	}
}
