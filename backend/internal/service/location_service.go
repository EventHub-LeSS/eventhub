package service

import (
	"backend/internal/model"
	"backend/internal/repository"

	"github.com/google/uuid"
)

// LocationService lists the stored locations organizers can pick from when entering the address of
// an event. Locations are created by saving events, see EventService.CreateDraft and UpdateEvent.
type LocationService struct {
	orgRepo      repository.OrganizationRepository
	locationRepo repository.LocationRepository
}

func NewLocationService(orgRepo repository.OrganizationRepository, locationRepo repository.LocationRepository) *LocationService {
	return &LocationService{orgRepo: orgRepo, locationRepo: locationRepo}
}

// ListSelectable returns the locations of all events that are no drafts and of every event of the
// organizations in keycloakOrgIDs, the organizations in which the caller may manage events.
func (s *LocationService) ListSelectable(keycloakOrgIDs []string) ([]model.PublishedEventLocation, error) {
	orgs, err := s.orgRepo.ListByKeycloakOrgIDsOrAliases(keycloakOrgIDs)
	if err != nil {
		return nil, err
	}
	orgIDs := make([]uuid.UUID, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.OrganizationID)
	}
	locations, err := s.locationRepo.ListSelectable(orgIDs)
	if err != nil {
		return nil, err
	}
	selectable := make([]model.PublishedEventLocation, 0, len(locations))
	for _, location := range locations {
		selectable = append(selectable, model.LocationResponse(location))
	}
	return selectable, nil
}
