package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ratingEventStub struct {
	repository.EventRepository
	event *model.EventModel
	err   error
}

func (s *ratingEventStub) GetEventByID(uuid.UUID) (*model.EventModel, error) {
	return s.event, s.err
}

type ratingOrgStub struct {
	repository.OrganizationRepository
	org *model.OrganizationModel
	err error
}

func (s *ratingOrgStub) GetByID(uuid.UUID) (*model.OrganizationModel, error) {
	return s.org, s.err
}

type ratingRepoStub struct {
	err error
}

func (s *ratingRepoStub) GetEventRatingSummary(uuid.UUID) (*float64, int64, error) {
	return nil, 0, s.err
}

func (s *ratingRepoStub) GetOrganizerRatingSummary(uuid.UUID) (*float64, int64, error) {
	return nil, 0, s.err
}

func ratingRouter(s *service.RatingService) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewRatingHandler(s)
	r := gin.New()
	// Include the existing wildcard siblings to catch Gin parameter-name conflicts.
	r.GET("/api/v1/events/:eventId", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/api/v1/organizations/:organizationID/audit-logs", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/api/v1/events/:eventId/rating-summary", h.GetEventRatingSummaryHandler)
	r.GET("/api/v1/organizations/:organizationID/rating-summary", h.GetOrganizerRatingSummaryHandler)
	return r
}

func getRatingSummary(router http.Handler, resource, id string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/"+resource+"/"+id+"/rating-summary", nil))
	return rec
}

func TestRatingSummaryErrors(t *testing.T) {
	privateErr := errors.New("private database error")
	for _, resource := range []string{"events", "organizations"} {
		for _, stage := range []string{"invalid ID", "unknown", "lookup failure", "aggregate failure"} {
			t.Run(resource+"/"+stage, func(t *testing.T) {
				id := uuid.NewString()
				events := &ratingEventStub{event: &model.EventModel{Status: model.EventStatusPublished}}
				orgs := &ratingOrgStub{org: &model.OrganizationModel{}}
				ratings := &ratingRepoStub{}
				status := http.StatusInternalServerError
				switch stage {
				case "invalid ID":
					id, status = "invalid", http.StatusBadRequest
				case "unknown":
					events.event, orgs.org, status = nil, nil, http.StatusNotFound
				case "lookup failure":
					events.err, orgs.err = privateErr, privateErr
				case "aggregate failure":
					ratings.err = privateErr
				}
				rec := getRatingSummary(ratingRouter(service.NewRatingService(ratings, events, orgs)), resource, id)
				assertOrganizationEventsProblem(t, rec, status)
				if strings.Contains(rec.Body.String(), privateErr.Error()) {
					t.Fatal("database error leaked")
				}
			})
		}
	}
}

func TestRatingSummaryEmptyJSON(t *testing.T) {
	router := ratingRouter(service.NewRatingService(&ratingRepoStub{},
		&ratingEventStub{event: &model.EventModel{Status: model.EventStatusPublished}},
		&ratingOrgStub{org: &model.OrganizationModel{}}))
	for _, resource := range []string{"events", "organizations"} {
		t.Run(resource, func(t *testing.T) {
			rec := getRatingSummary(router, resource, uuid.NewString())
			var body map[string]json.RawMessage
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || string(body["averageRating"]) != "null" {
				t.Fatalf("expected explicit null average: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
