package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LocationRepository stores the addresses of events. Locations are only ever added, never changed.
type LocationRepository interface {
	// FindOrCreate returns the stored location with the address of location, compared
	// case-insensitively, and inserts location first if there is none.
	FindOrCreate(location *model.LocationModel) (*model.LocationModel, error)
	// ListSelectable returns the locations organizers may pick from: those of events that are no
	// drafts and those of any event of the given organizations. Drafts stay private to their organization.
	ListSelectable(organizationIDs []uuid.UUID) ([]*model.LocationModel, error)
}

type locationRepository struct {
	db *gorm.DB
}

func NewLocationRepository(db *gorm.DB) LocationRepository {
	return &locationRepository{db: db}
}

// sameAddress matches the expressions of the unique index locations_address_key.
const sameAddress = `lower(street) = lower(?::text) AND lower(house_number) IS NOT DISTINCT FROM lower(?::text) AND
	postal_code = ? AND lower(city) = lower(?::text) AND
	lower(address_addition) IS NOT DISTINCT FROM lower(?::text) AND accessible = ?`

func (r *locationRepository) FindOrCreate(location *model.LocationModel) (*model.LocationModel, error) {
	// The unique index turns a concurrent insert of the same address into a no-op, so both requests
	// read the one stored row afterwards.
	if err := r.db.Exec(`INSERT INTO locations (location_id, street, house_number, postal_code, city, address_addition, accessible)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		uuid.New(), location.Street, location.HouseNumber, location.PostalCode, location.City,
		location.AddressAddition, location.Accessible).Error; err != nil {
		return nil, err
	}
	var stored model.LocationModel
	err := r.db.Where(sameAddress, location.Street, location.HouseNumber, location.PostalCode, location.City,
		location.AddressAddition, location.Accessible).First(&stored).Error
	if err != nil {
		return nil, err
	}
	return &stored, nil
}

func (r *locationRepository) ListSelectable(organizationIDs []uuid.UUID) ([]*model.LocationModel, error) {
	locations := make([]*model.LocationModel, 0)
	used := r.db.Table("events").Select("1").
		Where("events.location_id = locations.location_id AND (events.status <> ? OR events.organizer_id IN ?)",
			model.EventStatusDraft, organizationIDs)
	err := r.db.Where("EXISTS (?)", used).
		Order("city, street, house_number NULLS FIRST, address_addition NULLS FIRST, accessible, location_id").
		Find(&locations).Error
	return locations, err
}
