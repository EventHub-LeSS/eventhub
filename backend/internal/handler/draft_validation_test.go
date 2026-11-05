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
		Location:   model.LocationInput{Street: "Markt", HouseNumber: textPtr("1"), PostalCode: "53111", City: "Bonn"},
	}
}

func textPtr(s string) *string { return &s }

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
		{name: "ort fehlt", mutate: func(r *model.CreateDraftRequest) { r.Location = model.LocationInput{} }, wantErr: true},
		{name: "strasse nur leerzeichen", mutate: func(r *model.CreateDraftRequest) { r.Location.Street = " \t " }, wantErr: true},
		{name: "strasse zu lang", mutate: func(r *model.CreateDraftRequest) { r.Location.Street = strings.Repeat("ß", 201) }, wantErr: true},
		{name: "ohne hausnummer", mutate: func(r *model.CreateDraftRequest) { r.Location.HouseNumber = nil }},
		{name: "hausnummer zu lang", mutate: func(r *model.CreateDraftRequest) { r.Location.HouseNumber = textPtr(strings.Repeat("1", 21)) }, wantErr: true},
		{name: "plz fehlt", mutate: func(r *model.CreateDraftRequest) { r.Location.PostalCode = "" }, wantErr: true},
		{name: "plz zu lang", mutate: func(r *model.CreateDraftRequest) { r.Location.PostalCode = "531111" }, wantErr: true},
		{name: "plz mit vorzeichen", mutate: func(r *model.CreateDraftRequest) { r.Location.PostalCode = "-5311" }, wantErr: true},
		{name: "ort nur leerzeichen", mutate: func(r *model.CreateDraftRequest) { r.Location.City = "   " }, wantErr: true},
		{name: "mit adresszusatz", mutate: func(r *model.CreateDraftRequest) { r.Location.AddressAddition = textPtr("Halle 3, 2. Etage") }},
		{name: "adresszusatz zu lang", mutate: func(r *model.CreateDraftRequest) { r.Location.AddressAddition = textPtr(strings.Repeat("ä", 201)) }, wantErr: true},
		{name: "barrierefrei", mutate: func(r *model.CreateDraftRequest) { r.Location.Accessible = true }},
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
