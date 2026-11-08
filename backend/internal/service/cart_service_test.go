package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/testdb"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// The cart tests run against a real Postgres because the reservation and overbooking guarantees
// depend on database transactions and locking; see testdb.Open for how to enable them.

// seedUserRow inserts a user and returns their ID.
func seedUserRow(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	err := db.Exec("INSERT INTO users (user_id, keycloak_user_id, first_name, last_name, email) VALUES (?, ?, ?, ?, ?)",
		userID, "kc-"+userID.String(), "Max", "Muster", userID.String()+"@test.de").Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

// seedEventFor seeds an event with the given price for the given user.
func seedEventFor(t *testing.T, db *gorm.DB, capacity int, status model.EventStatus, price string, userID uuid.UUID) uuid.UUID {
	t.Helper()
	categoryID, locationID, orgID, eventID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO categories (category_id, category) VALUES (?, ?)", []any{categoryID, "cat-" + categoryID.String()}},
		{"INSERT INTO locations (location_id, street, postal_code, city, address_addition) VALUES (?, ?, ?, ?, ?)", []any{locationID, "Weg", "53111", "Bonn", "loc-" + locationID.String()}},
		{"INSERT INTO organizations (organization_id, keycloak_org_id, name, is_active) VALUES (?, ?, ?, true)", []any{orgID, "kc-org-" + orgID.String(), "Org " + orgID.String()}},
		{`INSERT INTO events (event_id, title, start_time, end_time, capacity, status, price, category_id, organizer_id, location_id)
		  VALUES (?, ?, NOW() + interval '1 day', NOW() + interval '2 day', ?, ?, ?, ?, ?, ?)`,
			[]any{eventID, "Konzert", capacity, string(status), price, categoryID, orgID, locationID}},
	}
	for _, st := range statements {
		if err := db.Exec(st.sql, st.args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return eventID
}

// wantSeededLocation fails unless location is the address that seedEventFor stores.
func wantSeededLocation(t *testing.T, location *model.PublishedEventLocation) {
	t.Helper()
	if location == nil || location.Street != "Weg" || location.HouseNumber != nil || location.PostalCode != "53111" ||
		location.City != "Bonn" || location.AddressAddition == nil || !strings.HasPrefix(*location.AddressAddition, "loc-") ||
		location.Accessible {
		t.Errorf("unexpected location: %+v", location)
	}
}

// seedEvent seeds an event and its user; the event costs nothing.
func seedEvent(t *testing.T, db *gorm.DB, capacity int, status model.EventStatus) (eventID, userID uuid.UUID) {
	t.Helper()
	userID = seedUserRow(t, db)
	return seedEventFor(t, db, capacity, status, "0", userID), userID
}

func reservedTickets(t *testing.T, db *gorm.DB, eventID uuid.UUID) int64 {
	t.Helper()
	var sum int64
	err := db.Raw("SELECT COALESCE(SUM(number_of_tickets), 0) FROM bookings WHERE event_id = ? AND status = 'reserved'", eventID).Scan(&sum).Error
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return sum
}

// newCartService builds the cart service over the real database.
func newCartService(db *gorm.DB) *CartService {
	return NewCartService(repository.NewTransactor(db), repository.NewBookingRepository(db),
		repository.NewCartRepository(db), repository.NewPaymentRepository(db), 0)
}

// cartItemFor finds the cart item of the event, or nil.
func cartItemFor(cart *model.CartResponse, eventID uuid.UUID) *model.CartItemResponse {
	for i := range cart.Items {
		if item := cart.Items[i]; item.Event != nil && item.Event.EventID == eventID {
			return &cart.Items[i]
		}
	}
	return nil
}

// addToCart adds tickets of the fixture event and returns the booking ID of the item.
func (f paymentFixture) addToCart(t *testing.T, userID uuid.UUID, tickets int) uuid.UUID {
	t.Helper()
	cart, err := f.carts.AddToCart(context.Background(), userID, f.eventID, tickets)
	if err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	item := cartItemFor(cart, f.eventID)
	if item == nil {
		t.Fatalf("no cart item for the event: %+v", cart)
	}
	return item.BookingID
}

func TestAddToCart_ParallelRequestsNeverOverbook(t *testing.T) {
	db := testdb.Open(t)
	eventID, _ := seedEvent(t, db, 1, model.EventStatusPublished)
	svc := newCartService(db)

	const attempts = 50
	users := make([]uuid.UUID, attempts)
	for i := range users {
		users[i] = seedUserRow(t, db)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(attempts)
	sqlDB.SetMaxIdleConns(attempts)

	// Every goroutine warms up its own pooled connection first, so that the
	// actual reservations hit the database at the same moment.
	start := make(chan struct{})
	results := make(chan error, attempts)
	var ready, wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		ready.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Exec("SELECT 1")
			ready.Done()
			<-start
			_, err := svc.AddToCart(context.Background(), users[i], eventID, 1)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	close(results)

	successes, rejected := 0, 0
	for err := range results {
		var capErr *CapacityExceededError
		switch {
		case err == nil:
			successes++
		case errors.As(err, &capErr):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successes != 1 || rejected != attempts-1 {
		t.Errorf("expected exactly 1 success and %d rejections, got %d successes and %d rejections", attempts-1, successes, rejected)
	}
	if sum := reservedTickets(t, db, eventID); sum != 1 {
		t.Errorf("overbooked: %d tickets reserved for capacity 1", sum)
	}
}

func TestAddToCart_CapacityMath(t *testing.T) {
	db := testdb.Open(t)
	eventID, _ := seedEvent(t, db, 3, model.EventStatusPublished)
	svc := newCartService(db)
	ctx := context.Background()
	first, second, third := seedUserRow(t, db), seedUserRow(t, db), seedUserRow(t, db)

	if _, err := svc.AddToCart(ctx, first, eventID, 2); err != nil {
		t.Fatalf("first reservation: %v", err)
	}

	var capErr *CapacityExceededError
	_, err := svc.AddToCart(ctx, second, eventID, 2)
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapacityExceededError, got %v", err)
	}
	if capErr.Available != 1 || capErr.Requested != 2 {
		t.Errorf("expected available=1 requested=2, got available=%d requested=%d", capErr.Available, capErr.Requested)
	}

	if _, err := svc.AddToCart(ctx, second, eventID, 1); err != nil {
		t.Fatalf("last seat: %v", err)
	}
	_, err = svc.AddToCart(ctx, third, eventID, 1)
	if !errors.As(err, &capErr) || capErr.Available != 0 {
		t.Errorf("expected sold out (available=0), got %v", err)
	}
	if sum := reservedTickets(t, db, eventID); sum != 3 {
		t.Errorf("expected 3 reserved tickets, got %d", sum)
	}
}

// Adding to an item that already exists raises its quantity and refreshes the reservation, up to
// the per-event limit; the item stays one reservation.
func TestAddToCart_MergesSameEventUpToTheTicketLimit(t *testing.T) {
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, 50, model.EventStatusPublished)
	svc := newCartService(db)
	ctx := context.Background()

	first, err := svc.AddToCart(ctx, userID, eventID, 2)
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	second, err := svc.AddToCart(ctx, userID, eventID, 3)
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if len(first.Items) != 1 || len(second.Items) != 1 || first.Items[0].BookingID != second.Items[0].BookingID {
		t.Fatalf("the item must stay one reservation: %+v then %+v", first, second)
	}
	if item := second.Items[0]; item.Event.EventID != eventID || second.TotalTickets != 5 || second.TotalAmount.String() != "0" {
		t.Fatalf("unexpected cart: %+v", second)
	}
	if item := second.Items[0]; item.ExpiresAt.Before(time.Now().Add(14 * time.Minute)) {
		t.Errorf("adding must refresh the reservation, got expiry %v", item.ExpiresAt)
	}
	if _, err := svc.AddToCart(ctx, userID, eventID, MaxTicketsPerEvent); err == nil || !errors.Is(err, ErrTicketLimitExceeded) {
		t.Fatalf("expected ErrTicketLimitExceeded, got %v", err)
	}
	if sum := reservedTickets(t, db, eventID); sum != 5 {
		t.Errorf("expected 5 reserved tickets in one row, got %d", sum)
	}
}

func TestAddToCart_ExpiredReservationIsReplaced(t *testing.T) {
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, 1, model.EventStatusPublished)
	svc := newCartService(db)

	err := db.Exec(`INSERT INTO bookings (booking_id, user_id, event_id, number_of_tickets, status, expires_at)
	                VALUES (?, ?, ?, 1, 'reserved', NOW() - interval '1 hour')`, uuid.New(), userID, eventID).Error
	if err != nil {
		t.Fatalf("seed expired booking: %v", err)
	}

	cart, err := svc.AddToCart(context.Background(), userID, eventID, 1)
	if err != nil {
		t.Fatalf("expected expired reservation to free the seat, got %v", err)
	}
	item := cartItemFor(cart, eventID)
	if item == nil {
		t.Fatalf("no item for the event: %+v", cart)
	}
	if item.ExpiresAt.Before(time.Now().Add(14 * time.Minute)) {
		t.Errorf("expected expiry about 15 minutes ahead, got %v", item.ExpiresAt)
	}
	var rows int64
	if err := db.Raw("SELECT COUNT(*) FROM bookings WHERE user_id = ? AND event_id = ? AND payment_id IS NULL", userID, eventID).Scan(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("the expired reservation must be replaced, got %d rows (%v)", rows, err)
	}
}

func TestAddToCart_RejectsUnpublishedEvent(t *testing.T) {
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, 10, model.EventStatusDraft)
	svc := newCartService(db)

	_, err := svc.AddToCart(context.Background(), userID, eventID, 1)
	if !errors.Is(err, ErrEventNotPublished) {
		t.Errorf("expected ErrEventNotPublished, got %v", err)
	}
	if sum := reservedTickets(t, db, eventID); sum != 0 {
		t.Errorf("no reservation must be created, got %d tickets", sum)
	}
}

func TestAddToCart_UnknownEvent(t *testing.T) {
	db := testdb.Open(t)
	userID := seedUserRow(t, db)
	svc := newCartService(db)

	_, err := svc.AddToCart(context.Background(), userID, uuid.New(), 1)
	if !errors.Is(err, ErrEventNotFound) {
		t.Errorf("expected ErrEventNotFound, got %v", err)
	}
}

func TestAddToCart_RejectsInvalidCounts(t *testing.T) {
	svc := NewCartService(nil, nil, nil, nil, 0)
	for _, n := range []int{0, -1} {
		if _, err := svc.AddToCart(context.Background(), uuid.New(), uuid.New(), n); !errors.Is(err, ErrInvalidTicketCount) {
			t.Errorf("tickets=%d: expected ErrInvalidTicketCount, got %v", n, err)
		}
	}
	if _, err := svc.AddToCart(context.Background(), uuid.New(), uuid.New(), MaxTicketsPerEvent+1); !errors.Is(err, ErrTicketLimitExceeded) {
		t.Errorf("tickets=%d: expected ErrTicketLimitExceeded, got %v", MaxTicketsPerEvent+1, err)
	}
}

// The cart view shows the current prices and availability, leaves expired reservations out and
// flags items whose event can no longer be checked out.
func TestGetCart_ShowsCurrentPricesAvailabilityAndBookability(t *testing.T) {
	db := testdb.Open(t)
	userID := seedUserRow(t, db)
	music := seedEventFor(t, db, 10, model.EventStatusPublished, "20", userID)
	food := seedEventFor(t, db, 5, model.EventStatusPublished, "5", userID)
	withdrawn := seedEventFor(t, db, 8, model.EventStatusPublished, "9", userID)
	svc := newCartService(db)
	ctx := context.Background()

	if _, err := svc.AddToCart(ctx, userID, music, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddToCart(ctx, userID, food, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddToCart(ctx, userID, withdrawn, 3); err != nil {
		t.Fatal(err)
	}
	// An expired reservation holds no seats and is not part of the cart.
	if err := db.Exec(`INSERT INTO bookings (booking_id, user_id, event_id, number_of_tickets, status, expires_at)
	                VALUES (?, ?, ?, 1, 'reserved', NOW() - interval '1 hour')`, uuid.New(), userID, music).Error; err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", withdrawn)

	cart, err := svc.GetCart(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 3 || cart.TotalTickets != 6 || !cart.TotalAmount.Equal(decimal.RequireFromString("72")) || cart.CheckoutReady {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	musicItem, foodItem, withdrawnItem := cartItemFor(cart, music), cartItemFor(cart, food), cartItemFor(cart, withdrawn)
	if musicItem == nil || foodItem == nil || withdrawnItem == nil {
		t.Fatalf("missing items: %+v", cart)
	}
	// AvailableSeats counts the seats free for others plus the item's own reserved seats.
	if musicItem.AvailableSeats != 10 || foodItem.AvailableSeats != 5 || withdrawnItem.AvailableSeats != 8 {
		t.Errorf("unexpected availability: music=%d food=%d withdrawn=%d", musicItem.AvailableSeats, foodItem.AvailableSeats, withdrawnItem.AvailableSeats)
	}
	if !musicItem.UnitPrice.Equal(decimal.RequireFromString("20")) || !musicItem.LineTotal.Equal(decimal.RequireFromString("40")) {
		t.Errorf("unexpected music line: %+v", musicItem)
	}
	if !foodItem.LineTotal.Equal(decimal.RequireFromString("5")) {
		t.Errorf("unexpected food line: %+v", foodItem)
	}
	if musicItem.Bookable && !foodItem.Bookable {
		t.Error("published events must be bookable")
	}
	if withdrawnItem.Bookable || cart.CheckoutReady {
		t.Errorf("a withdrawn event must block the checkout: %+v", withdrawnItem)
	}
	wantSeededLocation(t, musicItem.Event.Location)
}

func TestGetCart_ShowsTheOpenCheckout(t *testing.T) {
	f := newPaymentFixture(t, 20, "20")
	ctx := context.Background()

	// Two items, one checkout: the items leave the cart together.
	other := seedEventFor(t, f.db, 10, model.EventStatusPublished, "5", f.userID)
	f.addToCart(t, f.userID, 2)
	if _, err := f.carts.AddToCart(ctx, f.userID, other, 1); err != nil {
		t.Fatal(err)
	}
	pending := f.checkout(t)

	// The items left the cart with the checkout; the open payment takes their place with the
	// stored sums, so a return from the payment view does not look like lost items.
	cart, err := f.carts.GetCart(ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 0 {
		t.Fatalf("checked-out items must leave the cart: %+v", cart.Items)
	}
	if cart.OpenPayment == nil {
		t.Fatal("the open checkout must be reported")
	}
	if cart.OpenPayment.PaymentID != pending.PaymentID || !cart.OpenPayment.Amount.Equal(pending.Amount) ||
		cart.OpenPayment.TotalTickets != 3 {
		t.Fatalf("unexpected open payment: %+v, want payment %s (%s, 3 tickets)", cart.OpenPayment, pending.PaymentID, pending.Amount)
	}
	if !cart.OpenPayment.ExpiresAt.After(time.Now().Add(14 * time.Minute)) {
		t.Errorf("the shared deadline must be about 15 minutes ahead, got %v", cart.OpenPayment.ExpiresAt)
	}

	// New items coexist with the open checkout; it stays unchanged.
	f.addToCart(t, f.userID, 1)
	if cart, err = f.carts.GetCart(ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 1 || cart.OpenPayment == nil || cart.OpenPayment.PaymentID != pending.PaymentID {
		t.Fatalf("the open checkout must stay while items coexist: %+v", cart)
	}

	// The checkout of one user never shows in the cart of another.
	stranger, err := f.carts.GetCart(ctx, f.seedUser(t))
	if err != nil {
		t.Fatal(err)
	}
	if stranger.OpenPayment != nil {
		t.Fatalf("a foreign checkout must not show: %+v", stranger.OpenPayment)
	}

	// Once the payment completes, the checkout is no longer open.
	if _, err := f.payments.SimulatePayment(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount); err != nil {
		t.Fatal(err)
	}
	if cart, err = f.carts.GetCart(ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if cart.OpenPayment != nil {
		t.Fatalf("a completed payment must not show as open: %+v", cart.OpenPayment)
	}

	// An expired reservation holds no seats; its payment is no longer the open checkout.
	expired := f.checkout(t)
	if err := f.db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE payment_id = ?", expired.PaymentID).Error; err != nil {
		t.Fatal(err)
	}
	if cart, err = f.carts.GetCart(ctx, f.userID); err != nil {
		t.Fatal(err)
	}
	if cart.OpenPayment != nil {
		t.Fatalf("an expired checkout holds no seats: %+v", cart.OpenPayment)
	}
}

func TestUpdateQuantity(t *testing.T) {
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, 3, model.EventStatusPublished)
	other := seedUserRow(t, db)
	svc := newCartService(db)
	ctx := context.Background()

	bookingID := addToCartEvent(t, svc, userID, eventID, 2)

	// Keeping the tickets always works: the item holds its own seats.
	if _, err := svc.UpdateQuantity(ctx, userID, bookingID, 2); err != nil {
		t.Fatalf("keep quantity: %v", err)
	}
	// Growing needs free seats besides the item's own: one seat is free.
	var capErr *CapacityExceededError
	_, err := svc.UpdateQuantity(ctx, userID, bookingID, 4)
	if !errors.As(err, &capErr) || capErr.Requested != 4 || capErr.Available != 3 {
		t.Fatalf("expected available=3 (own included), got %v", err)
	}
	// Someone else takes the last seat: the item keeps its own tickets but cannot grow.
	if _, err := svc.AddToCart(ctx, other, eventID, 1); err != nil {
		t.Fatal(err)
	}
	_, err = svc.UpdateQuantity(ctx, userID, bookingID, 3)
	if !errors.As(err, &capErr) || capErr.Available != 2 {
		t.Fatalf("expected available=2 (own included), got %v", err)
	}
	if cart, err := svc.UpdateQuantity(ctx, userID, bookingID, 1); err != nil || cartItemFor(cart, eventID).BookingID != bookingID {
		t.Fatalf("shrinking must keep the item: %v", err)
	}

	if _, err := svc.UpdateQuantity(ctx, userID, bookingID, MaxTicketsPerEvent+1); !errors.Is(err, ErrTicketLimitExceeded) {
		t.Errorf("expected ErrTicketLimitExceeded, got %v", err)
	}
	if _, err := svc.UpdateQuantity(ctx, other, bookingID, 1); !errors.Is(err, ErrCartItemNotFound) {
		t.Errorf("a foreign item must read as not found, got %v", err)
	}
	if _, err := svc.UpdateQuantity(ctx, userID, uuid.New(), 1); !errors.Is(err, ErrCartItemNotFound) {
		t.Errorf("an unknown item must read as not found, got %v", err)
	}
}

func TestUpdateQuantity_Rejections(t *testing.T) {
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, 10, model.EventStatusPublished)
	svc := newCartService(db)
	ctx := context.Background()

	for _, n := range []int{0, -1, MaxTicketsPerEvent + 1} {
		if _, err := svc.UpdateQuantity(ctx, userID, uuid.New(), n); !errors.Is(err, ErrInvalidTicketCount) && !errors.Is(err, ErrTicketLimitExceeded) {
			t.Fatalf("quantity=%d: unexpected error: %v", n, err)
		}
	}

	expired := addToCartEvent(t, svc, userID, eventID, 1)
	expireReservation(t, db, expired)
	if _, err := svc.UpdateQuantity(ctx, userID, expired, 2); !errors.Is(err, ErrReservationExpired) {
		t.Errorf("expected ErrReservationExpired, got %v", err)
	}

	checkedOut := addToCartEvent(t, svc, userID, eventID, 1)
	payment, _, err := svc.Checkout(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(payment.Items) != 1 || payment.Items[0].BookingID != checkedOut {
		t.Fatalf("unexpected checkout: %+v", payment)
	}
	// A checked out item is no cart item anymore.
	if _, err := svc.UpdateQuantity(ctx, userID, checkedOut, 2); !errors.Is(err, ErrCartItemNotFound) {
		t.Errorf("expected ErrCartItemNotFound for a checked out item, got %v", err)
	}
}

func TestRemoveFromCart(t *testing.T) {
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, 2, model.EventStatusPublished)
	other := seedUserRow(t, db)
	svc := newCartService(db)
	ctx := context.Background()

	bookingID := addToCartEvent(t, svc, userID, eventID, 2)
	var capErr *CapacityExceededError
	if _, err := svc.AddToCart(ctx, other, eventID, 1); !errors.As(err, &capErr) {
		t.Fatalf("expected the item to hold its seats, got %v", err)
	}

	if _, err := svc.RemoveFromCart(ctx, other, bookingID); !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("a foreign item must read as not found, got %v", err)
	}
	cart, err := svc.RemoveFromCart(ctx, userID, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 0 || cart.TotalTickets != 0 {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	if _, err := svc.RemoveFromCart(ctx, userID, bookingID); !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("a removed item must read as not found, got %v", err)
	}
	// The seats are free again.
	if _, err := svc.AddToCart(ctx, other, eventID, 2); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}

func TestClearCart(t *testing.T) {
	db := testdb.Open(t)
	userID := seedUserRow(t, db)
	music := seedEventFor(t, db, 5, model.EventStatusPublished, "20", userID)
	food := seedEventFor(t, db, 5, model.EventStatusPublished, "5", userID)
	other := seedUserRow(t, db)
	svc := newCartService(db)
	ctx := context.Background()

	addToCartEvent(t, svc, userID, music, 1)
	addToCartEvent(t, svc, userID, food, 2)
	// A reservation with a payment belongs to an open checkout, not to the cart.
	checkedOut := addToCartEvent(t, svc, other, music, 1)
	if _, _, err := svc.Checkout(ctx, other); err != nil {
		t.Fatal(err)
	}

	cart, err := svc.ClearCart(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 0 || cart.TotalTickets != 0 || !cart.TotalAmount.IsZero() || cart.CheckoutReady {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	var open int64
	if err := db.Raw("SELECT COUNT(*) FROM bookings WHERE user_id = ? AND payment_id IS NULL AND status = 'reserved'", userID).Scan(&open).Error; err != nil || open != 0 {
		t.Fatalf("the cart items must be gone, got %d (%v)", open, err)
	}
	stored, err := repository.NewBookingRepository(db).GetBookingByID(checkedOut)
	if err != nil || stored == nil || stored.PaymentID == nil {
		t.Fatalf("the checked out reservation must stay: %+v (%v)", stored, err)
	}
}

// addToCartEvent adds tickets of an event and returns the booking ID of the item.
func addToCartEvent(t *testing.T, svc *CartService, userID, eventID uuid.UUID, tickets int) uuid.UUID {
	t.Helper()
	cart, err := svc.AddToCart(context.Background(), userID, eventID, tickets)
	if err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	item := cartItemFor(cart, eventID)
	if item == nil {
		t.Fatalf("no cart item for the event: %+v", cart)
	}
	return item.BookingID
}
