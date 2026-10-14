package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type NotificationHandler struct {
	notifications *service.NotificationService
	users         repository.UserRepository
}

func NewNotificationHandler(notifications *service.NotificationService, users repository.UserRepository) *NotificationHandler {
	return &NotificationHandler{notifications: notifications, users: users}
}
func (h *NotificationHandler) user(c *gin.Context) (*model.UserModel, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return nil, false
	}
	return resolveAPIUser(c, h.users, principal)
}

// ListNotificationsHandler handles EVENTHUB-240.
// @Summary List own notifications
// @Description Returns only the authenticated user's notifications, newest first. No notifications returns []. Simulated reminders are immediately visible, even before scheduledAt; no email is sent.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.NotificationData
// @Failure 401 {object} model.ErrorResponse
// @Failure 403 {object} model.ErrorResponse
// @Failure 500 {object} model.ErrorResponse
// @Router /notifications [get]
func (h *NotificationHandler) ListNotificationsHandler(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	notifications, err := h.notifications.ListForUser(c.Request.Context(), user.UserID)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, "internal error")
		return
	}
	c.JSON(http.StatusOK, notifications)
}

// SimulateReminderHandler handles EVENTHUB-240.
// @Summary Simulate own booking reminder
// @Description Creates an in-app reminder for an own confirmed booking of an upcoming published event. scheduledAt is 24 hours before the event or now for short-notice bookings. Simulation displays it immediately. Repeated calls return the existing reminder. No email or external delivery service is used. Foreign bookings return 404.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param bookingId path string true "Booking UUID"
// @Success 201 {object} model.NotificationData
// @Success 200 {object} model.NotificationData "Existing reminder"
// @Failure 400 {object} model.ErrorResponse
// @Failure 401 {object} model.ErrorResponse
// @Failure 403 {object} model.ErrorResponse
// @Failure 404 {object} model.ErrorResponse
// @Failure 409 {object} model.ErrorResponse
// @Failure 500 {object} model.ErrorResponse
// @Router /bookings/{bookingId}/reminder [post]
func (h *NotificationHandler) SimulateReminderHandler(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	bookingID, err := uuid.Parse(c.Param("bookingId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "bookingId must be a valid UUID")
		return
	}
	notification, created, err := h.notifications.SimulateReminder(c.Request.Context(), bookingID, user.UserID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBookingNotFound):
			writeProblem(c, http.StatusNotFound, err.Error())
		case errors.Is(err, service.ErrReminderUnavailable):
			writeProblem(c, http.StatusConflict, err.Error())
		default:
			writeProblem(c, http.StatusInternalServerError, "internal error")
		}
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, notification)
}
