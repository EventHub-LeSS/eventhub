package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestPublishedEventsDateValidation(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
		hours       int
	}{
		{"empty", "", true, 0},
		{"reset", "  ", true, 0},
		{"trimmed", " 2026-10-06 ", true, 24},
		{"leap day", "2028-02-29", true, 24},
		{"spring DST", "2026-03-29", true, 23},
		{"autumn DST", "2026-10-25", true, 25},
		{"invalid leap day", "2026-02-29", false, 0},
		{"invalid month", "2026-13-01", false, 0},
		{"wrong format", "06.10.2026", false, 0},
		{"timestamp", "2026-10-06T00:00:00Z", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &publishedEventsStub{}
			router := gin.New()
			router.GET("/api/v1/events", NewEventHandler(service.NewEventService(repo, nil, nil)).ListPublishedEventsHandler)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events?date="+url.QueryEscape(tc.input), nil))
			if !tc.valid {
				if rec.Code != http.StatusBadRequest || repo.called {
					t.Fatalf("status=%d called=%v", rec.Code, repo.called)
				}
				return
			}
			if rec.Code != http.StatusOK || !repo.called {
				t.Fatalf("status=%d called=%v", rec.Code, repo.called)
			}
			date := repo.filter.Date
			if tc.hours == 0 {
				if date != nil {
					t.Fatal("empty date must reset filter")
				}
			} else {
				if date == nil || date.Location().String() != "Europe/Berlin" || date.Hour() != 0 ||
					date.AddDate(0, 0, 1).Sub(*date) != time.Duration(tc.hours)*time.Hour {
					t.Fatalf("date=%v", date)
				}
			}
		})
	}
}

func TestPublishedEventsDateDatabase(t *testing.T) {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, calendarDay := range []string{"2030-10-08", "2030-03-31", "2030-10-27"} {
		t.Run(calendarDay, func(t *testing.T) {
			db := testdb.Open(t)
			day, err := time.ParseInLocation(time.DateOnly, calendarDay, location)
			if err != nil {
				t.Fatal(err)
			}
			next := day.AddDate(0, 0, 1)
			var ids []uuid.UUID
			var category uuid.UUID
			for _, instant := range []time.Time{day.Add(-time.Microsecond), day, next.Add(-time.Microsecond), next} {
				event := seedEventForOrg(t, db, "date", model.EventStatusPublished)
				if category == uuid.Nil {
					category = event.categoryID
				}
				if err := db.Exec("UPDATE events SET start_time = ?, end_time = ?, category_id = ? WHERE event_id = ?",
					instant.UTC(), instant.Add(time.Hour).UTC(), category, event.eventID).Error; err != nil {
					t.Fatal(err)
				}
				ids = append(ids, event.eventID)
			}
			for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusCancelled, model.EventStatusCompleted} {
				event := seedEventForOrg(t, db, "date", status)
				if err := db.Exec("UPDATE events SET start_time = ?, end_time = ? WHERE event_id = ?",
					day.UTC(), day.Add(time.Hour).UTC(), event.eventID).Error; err != nil {
					t.Fatal(err)
				}
			}
			router := newEventRouter(db, nil)
			for _, tc := range []struct {
				name, date, city, cat string
				want                  []uuid.UUID
			}{
				// Events overlapping the day are matched (EVENTHUB-269): ids[0] starts one
				// microsecond before midnight and runs one hour into the day, ids[2] starts
				// one microsecond before the end of the day.
				{"selected day", calendarDay, "", "", ids[:3]},
				{"all filters", calendarDay, "Bonn", category.String(), ids[:3]},
				{"other city", calendarDay, "Berlin", "", nil},
				{"other category", calendarDay, "", uuid.NewString(), nil},
				{"no results", "2020-01-01", "", "", nil},
				// ids[2] starts one microsecond before the next day and runs into it.
				{"change day", next.Format(time.DateOnly), "", "", ids[2:]},
				{"reset", "", "", "", ids},
			} {
				t.Run(tc.name, func(t *testing.T) {
					query := url.Values{"date": {tc.date}, "location": {tc.city}, "categoryId": {tc.cat}}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events?"+query.Encode(), nil))
					if rec.Code != http.StatusOK {
						t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
					}
					var events []model.PublishedEventResponse
					if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
						t.Fatal(err)
					}
					if events == nil || len(events) != len(tc.want) {
						t.Fatalf("body=%s want=%v", rec.Body.String(), tc.want)
					}
					for i, event := range events {
						if event.EventID != tc.want[i] || event.Status != model.EventStatusPublished {
							t.Fatalf("unexpected event: %+v", event)
						}
					}
				})
			}
		})
	}
}

func TestPublishedEventsDateMultiDay(t *testing.T) {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	db := testdb.Open(t)
	day, err := time.ParseInLocation(time.DateOnly, "2030-06-10", location)
	if err != nil {
		t.Fatal(err)
	}
	before := day.AddDate(0, 0, -1)
	after := day.AddDate(0, 0, 1)
	afterAfter := day.AddDate(0, 0, 2)

	single := seedEventForOrg(t, db, "multiday", model.EventStatusPublished)
	spanning := seedEventForOrg(t, db, "multiday", model.EventStatusPublished)
	// Ends exactly at the start of the selected day: the half-open interval
	// [start, end) means its last moment is on the day before, so it is not
	// matched on the selected day.
	endsAtMidnight := seedEventForOrg(t, db, "multiday", model.EventStatusPublished)
	startsNextMidnight := seedEventForOrg(t, db, "multiday", model.EventStatusPublished)
	for _, u := range []struct {
		id         uuid.UUID
		start, end time.Time
	}{
		{single.eventID, day.Add(10 * time.Hour), day.Add(12 * time.Hour)},
		{spanning.eventID, before.Add(8 * time.Hour), after.Add(18 * time.Hour)},
		{endsAtMidnight.eventID, before.AddDate(0, 0, -1), day},
		{startsNextMidnight.eventID, after, afterAfter},
	} {
		if err := db.Exec("UPDATE events SET start_time = ?, end_time = ? WHERE event_id = ?",
			u.start.UTC(), u.end.UTC(), u.id).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := newEventRouter(db, nil)
	for _, tc := range []struct {
		date string
		want []uuid.UUID
	}{
		{day.AddDate(0, 0, -3).Format(time.DateOnly), nil},
		// endsAtMidnight starts exactly at midnight of this day, so it takes place here.
		{day.AddDate(0, 0, -2).Format(time.DateOnly), []uuid.UUID{endsAtMidnight.eventID}},
		// endsAtMidnight runs until the start of the selected day, spanning covers it.
		{before.Format(time.DateOnly), []uuid.UUID{endsAtMidnight.eventID, spanning.eventID}},
		// A multi-day event must be found on every day it spans, not only its start day.
		{day.Format(time.DateOnly), []uuid.UUID{spanning.eventID, single.eventID}},
		{after.Format(time.DateOnly), []uuid.UUID{spanning.eventID, startsNextMidnight.eventID}},
		{afterAfter.Format(time.DateOnly), nil},
	} {
		t.Run(tc.date, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events?date="+url.QueryEscape(tc.date), nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var events []model.PublishedEventResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
				t.Fatal(err)
			}
			if events == nil || len(events) != len(tc.want) {
				t.Fatalf("body=%s want=%v", rec.Body.String(), tc.want)
			}
			for i, event := range events {
				if event.EventID != tc.want[i] || event.Status != model.EventStatusPublished {
					t.Fatalf("unexpected event: %+v", event)
				}
			}
		})
	}
}
