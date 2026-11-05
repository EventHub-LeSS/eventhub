package handler

import (
	"backend/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type LocationHandler struct {
	locationService *service.LocationService
}

func NewLocationHandler(locationService *service.LocationService) *LocationHandler {
	return &LocationHandler{locationService: locationService}
}

// @Summary      List selectable locations
// @Description  Returns the stored locations an organizer can pick to fill the address fields of an event: the locations of all events that are no drafts and of every event of the organizations in which the caller holds the event_manager role. Locations used only by drafts of other organizations are not listed. Saving an event with the same address, compared case-insensitively, reuses the location; any other address creates a new one. Locations are never changed. Sorted by city, street, house number and address addition.
// @Tags         locations
// @Security     BearerAuth
// @Produce      json
// @Success      200 {array}  model.PublishedEventLocation "Selectable locations; an empty array if there are none"
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /locations [get]
func (h *LocationHandler) ListLocationsHandler(c *gin.Context) {
	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	locations, err := h.locationService.ListSelectable(managedOrgIDs)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, "internal error")
		return
	}

	c.JSON(http.StatusOK, locations)
}
