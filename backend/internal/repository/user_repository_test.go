package repository

import (
	"backend/internal/model"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetPageQueries(t *testing.T) {
	for _, tt := range []struct {
		name       string
		page       int
		limit      int
		total      int64
		wantOffset int
		wantQuery  bool
	}{
		{"first", 1, 50, 1000, 0, true},
		{"second", 2, 10, 25, 10, true},
		{"last partial", 3, 10, 25, 20, true},
		{"empty", 1, 50, 0, 0, false},
		{"past last", 4, 10, 25, 0, false},
		{"exact boundary", 3, 10, 20, 0, false},
		{"max page", math.MaxInt, 100, 25, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			counts, pages := 0, 0
			db := userPageDryRunDB(t, func(tx *gorm.DB) {
				if tx.Statement.Context != ctx {
					t.Fatal("query did not use request context")
				}
				switch dest := tx.Statement.Dest.(type) {
				case *int64:
					counts++
					// DryRun assertions intentionally do not use an IDE data source.
					//noinspection SqlNoDataSourceInspection
					if got := tx.Statement.SQL.String(); got != `SELECT count(*) FROM "users"` {
						t.Fatalf("count SQL = %s", got)
					}
					*dest = tt.total
					tx.RowsAffected = 1
				case *[]*model.UserModel:
					pages++
					//noinspection SqlNoDataSourceInspection
					wantSQL := `SELECT * FROM "users" ORDER BY user_id ASC LIMIT $1`
					wantVars := []any{tt.limit}
					if tt.wantOffset > 0 {
						wantSQL += ` OFFSET $2`
						wantVars = append(wantVars, tt.wantOffset)
					}
					if tx.Statement.SQL.String() != wantSQL || !reflect.DeepEqual(tx.Statement.Vars, wantVars) {
						t.Fatalf("page query = %s %v, want %s %v", tx.Statement.SQL.String(), tx.Statement.Vars, wantSQL, wantVars)
					}
					*dest = []*model.UserModel{{KeycloakUserID: "page-user"}}
				default:
					t.Fatalf("unexpected query destination %T", dest)
				}
			})
			users, total, err := NewUserRepository(db).GetPage(ctx, tt.page, tt.limit)
			if err != nil || total != tt.total || counts != 1 {
				t.Fatalf("result total=%d, counts=%d, err=%v", total, counts, err)
			}
			if tt.wantQuery {
				if pages != 1 || len(users) != 1 || users[0].KeycloakUserID != "page-user" {
					t.Fatalf("missing page result: pages=%d, users=%v", pages, users)
				}
			} else if pages != 0 || users == nil || len(users) != 0 {
				t.Fatalf("empty page queried or returned nil: pages=%d, users=%v", pages, users)
			}
		})
	}
}

func TestGetPageErrors(t *testing.T) {
	for _, failCount := range []bool{true, false} {
		t.Run(map[bool]string{true: "count", false: "page"}[failCount], func(t *testing.T) {
			wantErr := errors.New("database query failed")
			queries := 0
			db := userPageDryRunDB(t, func(tx *gorm.DB) {
				queries++
				if count, ok := tx.Statement.Dest.(*int64); ok && !failCount {
					*count = 100
					tx.RowsAffected = 1
				} else {
					if err := tx.AddError(wantErr); !errors.Is(err, wantErr) {
						t.Fatalf("inject query error: %v", err)
					}
				}
			})
			users, _, err := NewUserRepository(db).GetPage(context.Background(), 1, 10)
			if !errors.Is(err, wantErr) || users != nil {
				t.Fatalf("users=%v, err=%v", users, err)
			}
			if failCount && queries != 1 {
				t.Fatalf("page fetched after count failed: %d queries", queries)
			}
		})
	}
}

func TestGetPageRejectsInvalidInput(t *testing.T) {
	for _, pair := range [][2]int{{0, 10}, {1, 0}, {-1, 10}, {1, -1}} {
		db := userPageDryRunDB(t, func(*gorm.DB) { t.Fatal("queried invalid page") })
		if _, _, err := NewUserRepository(db).GetPage(context.Background(), pair[0], pair[1]); err == nil {
			t.Fatalf("accepted %v", pair)
		}
	}
}

// DryRun builds real PostgreSQL queries without connecting; callbacks supply query results.
func userPageDryRunDB(t *testing.T, afterQuery func(*gorm.DB)) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=localhost user=test dbname=test sslmode=disable"}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := db.Callback().Query().After("gorm:query").Register("test:page-results", afterQuery); err != nil {
		t.Fatal(err)
	}
	return db
}
