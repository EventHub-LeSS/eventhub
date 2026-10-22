package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"math"
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
// @Description Returns only the authenticated user's notifications, newest first. The response contains items, page, limit and total; empty pages return items: []. readAt is null for unread notifications. Simulated reminder content and scheduledAt are refreshed from current event and location data before display; changed reminders become unread. Simulation is immediately visible, even before scheduledAt; no email is sent.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number (1-indexed)" default(1) minimum(1)
// @Param limit query int false "Page size (1-100)" default(50) minimum(1) maximum(100)
// @Success 200 {object} model.NotificationPage
// @Failure 400 {object} model.ErrorResponse
// @Failure 401 {object} model.ErrorResponse
// @Failure 403 {object} model.ErrorResponse
// @Failure 500 {object} model.ErrorResponse
// @Router /notifications [get]
func (h *NotificationHandler) ListNotificationsHandler(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	page, limit, err := parsePageLimit(c)
	if err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}
	if page-1 > math.MaxInt/limit {
		writeProblem(c, http.StatusBadRequest, "page is too large")
		return
	}
	notifications, err := h.notifications.ListForUser(c.Request.Context(), user.UserID, page, limit)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, "internal error")
		return
	}
	c.JSON(http.StatusOK, notifications)
}

// SimulateReminderHandler handles EVENTHUB-240.
// @Summary Simulate own booking reminder
// @Description Creates an in-app reminder for an own confirmed booking of an upcoming published event. scheduledAt is 24 hours before the event or now for short-notice bookings. Simulation displays it immediately. Repeated calls refresh the existing reminder from current event data. Changed reminders become unread. No email or external delivery service is used. Foreign bookings return 404.
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

// MarkNotificationReadHandler marks an own notification as read.
// @Summary Mark own notification as read
// @Description Idempotently sets readAt. Foreign and unknown notifications return 404. Changed simulated reminders become unread again on their next refresh.
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param notificationId path string true "Notification UUID"
// @Success 200 {object} model.NotificationData
// @Failure 400 {object} model.ErrorResponse
// @Failure 401 {object} model.ErrorResponse
// @Failure 403 {object} model.ErrorResponse
// @Failure 404 {object} model.ErrorResponse
// @Failure 500 {object} model.ErrorResponse
// @Router /notifications/{notificationId}/read [patch]
func (h *NotificationHandler) MarkNotificationReadHandler(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("notificationId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "notificationId must be a valid UUID")
		return
	}
	notification, err := h.notifications.MarkRead(c.Request.Context(), id, user.UserID)
	if err != nil {
		if errors.Is(err, service.ErrNotificationNotFound) {
			writeProblem(c, http.StatusNotFound, err.Error())
		} else {
			writeProblem(c, http.StatusInternalServerError, "internal error")
		}
		return
	}
	c.JSON(http.StatusOK, notification)
}
