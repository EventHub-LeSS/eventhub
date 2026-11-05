package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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
	err   error
	items []model.EventRating
	total int64
	// listed records the arguments of the last ListEventRatings call.
	listed *ratingListCall
}

type ratingListCall struct {
	eventID     uuid.UUID
	page, limit int
}

func (s *ratingRepoStub) GetEventRatingSummary(uuid.UUID) (*float64, int64, error) {
	return nil, 0, s.err
}

func (s *ratingRepoStub) GetOrganizerRatingSummary(uuid.UUID) (*float64, int64, error) {
	return nil, 0, s.err
}

func (s *ratingRepoStub) ListEventRatings(eventID uuid.UUID, page, limit int) ([]model.EventRating, int64, error) {
	s.listed = &ratingListCall{eventID: eventID, page: page, limit: limit}
	if s.err != nil {
		return nil, 0, s.err
	}
	items := s.items
	if items == nil {
		items = []model.EventRating{}
	}
	return items, s.total, nil
}

func (s *ratingRepoStub) CreateEventRating(context.Context, uuid.UUID, uuid.UUID, int, string) (*model.RatingModel, error) {
	return nil, s.err
}

func ratingRouter(s *service.RatingService) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewRatingHandler(s, nil)
	r := gin.New()
	// Include the existing wildcard siblings to catch Gin parameter-name conflicts.
	r.GET("/api/v1/events/:eventId", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/api/v1/organizations/:organizationID/audit-logs", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/api/v1/events/:eventId/rating-summary", h.GetEventRatingSummaryHandler)
	r.GET("/api/v1/events/:eventId/ratings", h.ListEventRatingsHandler)
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

func getEventRatings(router http.Handler, id, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events/"+id+"/ratings"+query, nil))
	return rec
}

func TestListEventRatingsRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct{ id, query string }{
		{"invalid", ""},
		{uuid.NewString(), "?page=0"},
		{uuid.NewString(), "?page=-1"},
		{uuid.NewString(), "?page=x"},
		{uuid.NewString(), "?limit=0"},
		{uuid.NewString(), "?limit=101"},
		{uuid.NewString(), "?limit=x"},
		{uuid.NewString(), "?page=" + strconv.Itoa(math.MaxInt)},
	} {
		t.Run(tc.id+tc.query, func(t *testing.T) {
			ratings := &ratingRepoStub{}
			router := ratingRouter(service.NewRatingService(ratings,
				&ratingEventStub{event: &model.EventModel{Status: model.EventStatusPublished}}, nil))
			assertOrganizationEventsProblem(t, getEventRatings(router, tc.id, tc.query), http.StatusBadRequest)
			if ratings.listed != nil {
				t.Fatal("ratings were loaded for invalid input")
			}
		})
	}
}

func TestListEventRatingsErrors(t *testing.T) {
	privateErr := errors.New("private database error")
	for _, tc := range []struct {
		name   string
		event  *model.EventModel
		events error
		list   error
		status int
	}{
		{name: "unknown", status: http.StatusNotFound},
		{name: "draft", event: &model.EventModel{Status: model.EventStatusDraft}, status: http.StatusNotFound},
		{name: "cancelled", event: &model.EventModel{Status: model.EventStatusCancelled}, status: http.StatusNotFound},
		{name: "completed but not ended", event: &model.EventModel{Status: model.EventStatusCompleted, EndTime: time.Now().Add(time.Hour)}, status: http.StatusNotFound},
		{name: "lookup failure", events: privateErr, status: http.StatusInternalServerError},
		{name: "list failure", event: &model.EventModel{Status: model.EventStatusPublished}, list: privateErr, status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ratings := &ratingRepoStub{err: tc.list}
			router := ratingRouter(service.NewRatingService(ratings, &ratingEventStub{event: tc.event, err: tc.events}, nil))
			rec := getEventRatings(router, uuid.NewString(), "")
			assertOrganizationEventsProblem(t, rec, tc.status)
			if strings.Contains(rec.Body.String(), privateErr.Error()) {
				t.Fatal("database error leaked")
			}
			if tc.list == nil && ratings.listed != nil {
				t.Fatal("ratings of a private event were loaded")
			}
		})
	}
}

func TestListEventRatingsJSON(t *testing.T) {
	name := "Max"
	created := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, query string
		event       *model.EventModel
		items       []model.EventRating
		page, limit int
	}{
		{name: "empty defaults", event: &model.EventModel{Status: model.EventStatusPublished}, page: 1, limit: 50},
		{
			name:  "ended completed event",
			query: "?page=3&limit=2",
			event: &model.EventModel{Status: model.EventStatusCompleted, EndTime: time.Now().Add(-time.Hour)},
			items: []model.EventRating{
				{RatingID: uuid.New(), Score: 5, Text: "Super", AuthorName: &name, CreatedAt: created},
				{RatingID: uuid.New(), Score: 2, CreatedAt: created},
			},
			page: 3, limit: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eventID := uuid.New()
			ratings := &ratingRepoStub{items: tc.items, total: 7}
			router := ratingRouter(service.NewRatingService(ratings, &ratingEventStub{event: tc.event}, nil))
			rec := getEventRatings(router, eventID.String(), tc.query)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if *ratings.listed != (ratingListCall{eventID: eventID, page: tc.page, limit: tc.limit}) {
				t.Fatalf("unexpected repository call: %+v", *ratings.listed)
			}
			var body struct {
				Items []map[string]json.RawMessage `json:"items"`
				Page  int                          `json:"page"`
				Limit int                          `json:"limit"`
				Total int64                        `json:"total"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Items == nil || len(body.Items) != len(tc.items) || body.Page != tc.page || body.Limit != tc.limit || body.Total != 7 {
				t.Fatalf("unexpected page: %s", rec.Body.String())
			}
			for i, item := range body.Items {
				if len(item) != 5 {
					t.Errorf("item %d exposes unexpected fields: %v", i, item)
				}
				want := "null"
				if tc.items[i].AuthorName != nil {
					want = `"` + *tc.items[i].AuthorName + `"`
				}
				if string(item["authorName"]) != want {
					t.Errorf("item %d authorName=%s, want %s", i, item["authorName"], want)
				}
			}
		})
	}
}

func TestCreateEventRatingRejectsInvalidInputBeforeUserLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewRatingHandler(service.NewRatingService(&ratingRepoStub{}, nil, nil), nil)
	noop := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	r := gin.New()
	// Register the POST siblings from main.go; Gin panics on wildcard name conflicts.
	r.POST("/api/v1/events/draft", noop)
	r.POST("/api/v1/events/:id/publish", noop)
	r.POST("/api/v1/events/:id/withdraw", noop)
	r.POST("/api/v1/events/:id/ratings", func(c *gin.Context) {
		middleware.SetPrincipalForRequest(c, &middleware.Principal{Subject: "visitor"})
		c.Next()
	}, h.CreateEventRatingHandler)

	for _, tc := range []struct {
		eventID string
		body    string
	}{
		{"invalid", `{"score":4}`},
		{uuid.NewString(), `{"score":0}`},
		{uuid.NewString(), `{"score":6}`},
		{uuid.NewString(), `{}`},
		{uuid.NewString(), `{"score":4,"text":"` + strings.Repeat("ä", 2001) + `"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+tc.eventID+"/ratings", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("input %s/%.40s: status=%d body=%.200s", tc.eventID, tc.body, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	r = gin.New()
	r.POST("/api/v1/events/:id/ratings", h.CreateEventRatingHandler)
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/events/"+uuid.NewString()+"/ratings", strings.NewReader(`{"score":4}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
