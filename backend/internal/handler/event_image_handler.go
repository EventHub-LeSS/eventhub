package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"backend/internal/model"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EVENTHUB image upload: raw PNG or JPEG bytes are sent as the request body, the Content-Type
// header declares the type. The image is stored base64 encoded like a regular database field
// and served again as real image bytes at GET /events/{eventId}/image.

// PutEventImageHandler stores the image of an event.
//
//	@Summary		Upload event image
//	@Description	Replaces the image of an event with the PNG or JPEG bytes sent as the raw request body; the Content-Type header must declare image/png or image/jpeg. The image is served at GET /events/{eventId}/image. Requires the event_manager role in the organization that owns the event. Only events in status draft or published can be edited. A body exceeding 2 MiB is refused with 413, a body that is empty or does not start with the magic bytes of the declared image type with 400 or 415.
//	@Tags			events
//	@Security		BearerAuth
//	@Accept			image/png
//	@Accept			image/jpeg
//	@Produce		json
//	@Param			eventId	path		string	true	"Event ID"
//	@Param			image	body		string	true	"Raw PNG or JPEG image bytes"
//	@Success		200		{object}	model.EventActionResponse
//	@Failure		400		{object}	model.ErrorResponse
//	@Failure		401		{object}	model.APIError
//	@Failure		403		{object}	model.ErrorResponse
//	@Failure		404		{object}	model.ErrorResponse
//	@Failure		413		{object}	model.ErrorResponse	"Image exceeds 2 MiB"
//	@Failure		415		{object}	model.ErrorResponse	"Content-Type must be image/png or image/jpeg"
//	@Failure		500		{object}	model.ErrorResponse
//	@Router			/events/{eventId}/image [put]
func (h *EventHandler) PutEventImageHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}

	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	// The type is taken from the verified header only; media type parameters such as a
	// charset are ignored.
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || !model.IsValidEventImageContentType(mediaType) {
		writeProblem(c, http.StatusUnsupportedMediaType, "Content-Type must be image/png or image/jpeg")
		return
	}

	var maxBytesErr *http.MaxBytesError
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, model.MaxEventImageBytes+1))
	if errors.As(err, &maxBytesErr) || (err == nil && len(body) > model.MaxEventImageBytes) {
		writeProblem(c, http.StatusRequestEntityTooLarge, "the image must not exceed 2 MiB")
		return
	}
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "the image could not be read")
		return
	}
	if len(body) == 0 {
		writeProblem(c, http.StatusBadRequest, "the image body must not be empty")
		return
	}

	if err := h.eventService.SetEventImage(c.Request.Context(), eventID, managedOrgIDs, mediaType, body); err != nil {
		writeEventImageError(c, err)
		return
	}

	c.JSON(http.StatusOK, model.EventActionResponse{Message: "event image stored"})
}

// GetEventImageHandler serves the image of a published event.
//
//	@Summary		Get event image
//	@Description	Serves the image of a published event as binary data with the Content-Type it was uploaded with (image/png or image/jpeg). Public like the event details; unpublished events and events without an image return 404.
//	@Tags			events
//	@Produce		image/png
//	@Produce		image/jpeg
//	@Param			eventId	path		string	true	"Event UUID"
//	@Success		200		{file}		binary
//	@Failure		400		{object}	model.ErrorResponse
//	@Failure		404		{object}	model.ErrorResponse
//	@Failure		500		{object}	model.ErrorResponse
//	@Router			/events/{eventId}/image [get]
func (h *EventHandler) GetEventImageHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "eventId must be a valid UUID")
		return
	}

	contentType, data, err := h.eventService.GetEventImage(eventID)
	if err != nil {
		writeEventImageError(c, err)
		return
	}

	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, contentType, data)
}

func writeEventImageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEventNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrNoImage):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrForbidden):
		writeProblem(c, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrInvalidStatus):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUnsupportedImage):
		writeProblem(c, http.StatusUnsupportedMediaType, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
