package model

import "testing"

func text(s string) *string { return &s }

func TestLocationInputLocation_TrimsAndDropsEmptyOptionalParts(t *testing.T) {
	location := LocationInput{
		Street: "  Lombardsbrücke ", HouseNumber: text(" "), PostalCode: " 20354", City: "Hamburg\n",
		AddressAddition: text("  Musikhalle Hamburg "), Accessible: true,
	}.Location()

	if location.Street != "Lombardsbrücke" || location.PostalCode != "20354" || location.City != "Hamburg" || !location.Accessible {
		t.Errorf("fields not trimmed: %+v", location)
	}
	if location.HouseNumber != nil {
		t.Errorf("blank house number = %q, want nil", *location.HouseNumber)
	}
	if location.AddressAddition == nil || *location.AddressAddition != "Musikhalle Hamburg" {
		t.Errorf("address addition = %v, want the trimmed text", location.AddressAddition)
	}
	if (LocationInput{Street: "Weg", PostalCode: "53111", City: "Bonn"}).Location().AddressAddition != nil {
		t.Error("omitted address addition must stay nil")
	}
}

func TestLocationAddress(t *testing.T) {
	tests := []struct {
		name     string
		location LocationModel
		want     string
	}{
		{"all parts", LocationModel{Street: "Lombardsbrücke", HouseNumber: text("1"), PostalCode: "20354", City: "Hamburg", AddressAddition: text("Musikhalle Hamburg")},
			"Musikhalle Hamburg, Lombardsbrücke 1, 20354 Hamburg"},
		{"without optional parts", LocationModel{Street: "Ratsweg", PostalCode: "60386", City: "Frankfurt"}, "Ratsweg, 60386 Frankfurt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.location.Address(); got != tt.want {
				t.Errorf("Address() = %q, want %q", got, tt.want)
			}
		})
	}
}
