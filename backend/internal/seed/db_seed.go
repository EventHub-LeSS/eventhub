package seed

import (
	"context"
	"fmt"
	"log"
	"time"

	"backend/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var seedNamespace = uuid.MustParse("00000000-0000-0000-0000-000000000000")

func seedUUID(tag string) uuid.UUID {
	return uuid.NewSHA1(seedNamespace, []byte(tag))
}

type DBSeeder struct {
	db        *gorm.DB
	orgByKey  map[string]uuid.UUID
	userByKey map[string]uuid.UUID
}

func NewDBSeeder(db *gorm.DB) *DBSeeder {
	return &DBSeeder{
		db:        db,
		orgByKey:  make(map[string]uuid.UUID),
		userByKey: make(map[string]uuid.UUID),
	}
}

func (s *DBSeeder) Seed(ctx context.Context) error {
	if err := s.resolveIDs(ctx); err != nil {
		return err
	}
	if err := s.enrichUsersAndOrgs(ctx); err != nil {
		return err
	}
	if err := s.seedCategories(ctx); err != nil {
		return err
	}
	if err := s.seedLocations(ctx); err != nil {
		return err
	}
	if err := s.seedEvents(ctx); err != nil {
		return err
	}
	if err := s.seedPayments(ctx); err != nil {
		return err
	}
	if err := s.seedBookings(ctx); err != nil {
		return err
	}
	if err := s.seedRatings(ctx); err != nil {
		return err
	}
	return s.seedNotifications(ctx)
}

func (s *DBSeeder) resolveIDs(_ context.Context) error {
	var users []model.UserModel
	if err := s.db.Find(&users).Error; err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	for _, u := range users {
		s.userByKey[u.Email] = u.UserID
	}
	var orgs []model.OrganizationModel
	if err := s.db.Find(&orgs).Error; err != nil {
		return fmt.Errorf("load organizations: %w", err)
	}
	for _, o := range orgs {
		s.orgByKey[o.Name] = o.OrganizationID
	}
	log.Printf("db: resolved %d users, %d organizations", len(s.userByKey), len(s.orgByKey))
	return nil
}

func (s *DBSeeder) enrichUsersAndOrgs(_ context.Context) error {
	log.Println("db: enriching users and organizations with mock contact data...")
	phoneByEmail := make(map[string]string)
	for _, mu := range mockUsers {
		phoneByEmail[mu.Email] = mu.Phone
	}
	for email, userID := range s.userByKey {
		phone, ok := phoneByEmail[email]
		if !ok {
			continue
		}
		if err := s.db.Model(&model.UserModel{}).Where("user_id = ?", userID).Update("phone_number", phone).Error; err != nil {
			return fmt.Errorf("update phone for user %q: %w", email, err)
		}
	}
	orgByName := make(map[string]mockOrg)
	for _, mo := range mockOrgs {
		orgByName[mo.Name] = mo
	}
	for name, orgID := range s.orgByKey {
		mo, ok := orgByName[name]
		if !ok {
			continue
		}
		updates := map[string]interface{}{
			"contact_email":        mo.ContactEmail,
			"contact_phone_number": mo.ContactPhone,
			"street":               mo.Street,
			"house_number":         mo.HouseNumber,
			"postal_code":          mo.PostalCode,
			"city":                 mo.City,
			"country_code":         mo.CountryCode,
		}
		if err := s.db.Model(&model.OrganizationModel{}).Where("organization_id = ?", orgID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update org %q: %w", name, err)
		}
	}
	return nil
}

func (s *DBSeeder) seedCategories(_ context.Context) error {
	log.Println("db: seeding categories...")
	cats := []model.CategoryModel{
		{CategoryID: seedUUID("cat-Music"), Category: "Music", Description: ptrString("Concerts, festivals and live performances")},
		{CategoryID: seedUUID("cat-Sports"), Category: "Sports", Description: ptrString("Sporting events and competitions")},
		{CategoryID: seedUUID("cat-Food & Drink"), Category: "Food & Drink", Description: ptrString("Food festivals, street food and culinary events")},
		{CategoryID: seedUUID("cat-Film"), Category: "Film", Description: ptrString("Open-air cinema, film screenings and premieres")},
		{CategoryID: seedUUID("cat-Exhibition"), Category: "Exhibition", Description: ptrString("Art, trade and cultural exhibitions")},
		{CategoryID: seedUUID("cat-Comedy"), Category: "Comedy", Description: ptrString("Stand-up and comedy shows")},
	}
	for _, c := range cats {
		var existing model.CategoryModel
		if err := s.db.Where("category = ?", c.Category).First(&existing).Error; err == gorm.ErrRecordNotFound {
			if err := s.db.Create(&c).Error; err != nil {
				return fmt.Errorf("create category %q: %w", c.Category, err)
			}
		}
	}
	return nil
}

func (s *DBSeeder) seedLocations(_ context.Context) error {
	log.Println("db: seeding locations...")
	locs := []model.LocationModel{
		{LocationID: seedUUID("loc-Musikhalle Hamburg"), Name: "Musikhalle Hamburg", City: "Hamburg", PostalCode: "20354", Street: "Lombardsbrücke", HouseNumber: ptrString("1")},
		{LocationID: seedUUID("loc-Olympiahalle München"), Name: "Olympiahalle München", City: "München", PostalCode: "80809", Street: "Spiridon-Louis-Ring", HouseNumber: ptrString("21")},
		{LocationID: seedUUID("loc-Treptower Park Berlin"), Name: "Treptower Park Berlin", City: "Berlin", PostalCode: "12435", Street: "Alt-Treptow", HouseNumber: ptrString("6")},
		{LocationID: seedUUID("loc-Stadthalle Bremen"), Name: "Stadthalle Bremen", City: "Bremen", PostalCode: "28215", Street: "Findorffstraße", HouseNumber: ptrString("105")},
		{LocationID: seedUUID("loc-Frankfurt Festplatz"), Name: "Frankfurt Festplatz", City: "Frankfurt", PostalCode: "60386", Street: "Ratsweg", HouseNumber: ptrString("1")},
	}
	for _, l := range locs {
		var existing model.LocationModel
		if err := s.db.Where("name = ?", l.Name).First(&existing).Error; err == gorm.ErrRecordNotFound {
			if err := s.db.Create(&l).Error; err != nil {
				return fmt.Errorf("create location %q: %w", l.Name, err)
			}
		}
	}
	return nil
}

type eventDef struct {
	tag       string
	title     string
	desc      string
	startDays int
	endDays   int
	capacity  int
	status    model.EventStatus
	price     string
	category  string
	orgName   string
	location  string
}

func (s *DBSeeder) seedEvents(_ context.Context) error {
	log.Println("db: seeding events...")
	now := time.Now()
	events := []eventDef{
		{"evt-1", "Summer Beats Festival", "Open-air music festival featuring top DJs and live acts.", 30, 31, 5000, model.EventStatusPublished, "49.90", "Music", "Provadis Hochschule", "Musikhalle Hamburg"},
		{"evt-2", "Rock am Ring Preview", "Preview night with legendary rock bands before the main festival.", 45, 45, 30000, model.EventStatusPublished, "89.00", "Music", "Telekom", "Olympiahalle München"},
		{"evt-3", "Bremen Comedy Night", "An evening of stand-up comedy with national and local talent.", 14, 14, 300, model.EventStatusPublished, "24.50", "Comedy", "Stadthalle Bremen", "Stadthalle Bremen"},
		{"evt-4", "Electronic Dance Festival", "Two-day open-air electronic music festival.", 60, 61, 8000, model.EventStatusDraft, "39.90", "Music", "ACME Events", "Treptower Park Berlin"},
		{"evt-5", "Food Truck Festival Frankfurt", "Over 50 food trucks serving street food from around the world.", 20, 22, 5000, model.EventStatusPublished, "12.00", "Food & Drink", "Telekom", "Frankfurt Festplatz"},
		{"evt-6", "Art Exhibition: Modern Visions", "Contemporary art from emerging European artists.", 10, 24, 200, model.EventStatusPublished, "12.00", "Exhibition", "Stadthalle Bremen", "Stadthalle Bremen"},
		{"evt-7", "Indie Night Live", "Up-and-coming indie bands in an intimate setting.", 35, 35, 400, model.EventStatusPublished, "18.00", "Music", "ACME Events", "Musikhalle Hamburg"},
		{"evt-8", "Open Air Cinema Berlin", "Outdoor film screenings under the stars.", 50, 50, 1500, model.EventStatusDraft, "15.00", "Film", "Telekom", "Treptower Park Berlin"},
		{"evt-9", "Winter Gala Concert", "Classical music gala with full symphony orchestra.", -5, -5, 600, model.EventStatusCompleted, "65.00", "Music", "Provadis Hochschule", "Musikhalle Hamburg"},
		{"evt-10", "Street Food Festival Bremen", "A weekend of street food, live cooking and local craft beer.", 28, 29, 3000, model.EventStatusCancelled, "8.00", "Food & Drink", "Stadthalle Bremen", "Stadthalle Bremen"},
		{"evt-11", "Jazz & Blues Night", "An evening of smooth jazz and blues in a relaxed atmosphere.", 40, 40, 800, model.EventStatusPublished, "32.00", "Music", "Telekom", "Olympiahalle München"},
		{"evt-12", "Comedy Gala Hamburg", "A night of laughter with five top comedians.", 55, 55, 500, model.EventStatusPublished, "28.00", "Comedy", "Provadis Hochschule", "Musikhalle Hamburg"},
		{"evt-13", "München Marathon Expo", "Expo and registration event ahead of the city marathon.", 18, 19, 10000, model.EventStatusPublished, "25.00", "Sports", "Telekom", "Olympiahalle München"},
		{"evt-14", "Berlin Beer Festival", "A weekend of craft beer, local breweries and live music.", 25, 27, 6000, model.EventStatusPublished, "14.00", "Food & Drink", "ACME Events", "Treptower Park Berlin"},
		{"evt-15", "Classical Nights München", "An elegant evening of Mozart and Beethoven with a full orchestra.", 38, 38, 1200, model.EventStatusPublished, "55.00", "Music", "Telekom", "Olympiahalle München"},
		{"evt-16", "Football Fan Fest Frankfurt", "Live screening of the finals with food trucks and fan zones.", 33, 33, 20000, model.EventStatusPublished, "0.00", "Sports", "Telekom", "Frankfurt Festplatz"},
		{"evt-17", "Digital Art Expo Bremen", "Interactive digital art installations from emerging creators.", 12, 26, 350, model.EventStatusPublished, "10.00", "Exhibition", "Provadis Hochschule", "Stadthalle Bremen"},
		{"evt-18", "Outdoor Cinema Hamburg", "Open-air screenings of cult classics by the river.", 48, 48, 800, model.EventStatusDraft, "12.50", "Film", "ACME Events", "Musikhalle Hamburg"},
		{"evt-19", "Stand-up Showcase Berlin", "A showcase of the funniest new stand-up talent in Germany.", 22, 22, 500, model.EventStatusPublished, "20.00", "Comedy", "ACME Events", "Treptower Park Berlin"},
		{"evt-20", "Oktoberfest Preview München", "A two-day taste of Oktoberfest with traditional food and beer.", 42, 44, 15000, model.EventStatusPublished, "35.00", "Food & Drink", "Telekom", "Olympiahalle München"},
		{"evt-21", "Indie Film Marathon Hamburg", "Back-to-back indie film screenings with director Q&As.", 52, 52, 300, model.EventStatusPublished, "9.00", "Film", "Provadis Hochschule", "Musikhalle Hamburg"},
		{"evt-22", "Frankfurt Wine Tasting", "Sample wines from the Rheingau paired with regional specialties.", 16, 16, 250, model.EventStatusPublished, "45.00", "Food & Drink", "Provadis Hochschule", "Frankfurt Festplatz"},
		{"evt-23", "Esports Championship Berlin", "Two-day esports tournament with international teams and a big prize pool.", 70, 72, 5000, model.EventStatusPublished, "30.00", "Sports", "ACME Events", "Treptower Park Berlin"},
		{"evt-24", "New Year's Eve Gala Hamburg", "Ring in the new year with a gala dinner, live band and fireworks.", 112, 113, 800, model.EventStatusPublished, "75.00", "Music", "Stadthalle Bremen", "Musikhalle Hamburg"},
		{"evt-25", "Spring Awakening Concert", "A fresh season opener with orchestral and choral pieces.", 120, 120, 1000, model.EventStatusDraft, "22.00", "Music", "Provadis Hochschule", "Musikhalle Hamburg"},
		{"evt-26", "Vintage Car Exhibition Bremen", "A weekend exhibition of classic cars and restoration workshops.", 8, 9, 600, model.EventStatusCancelled, "18.00", "Exhibition", "Stadthalle Bremen", "Stadthalle Bremen"},
	}
	for _, e := range events {
		id := seedUUID(e.tag)
		var existing model.EventModel
		if err := s.db.Where("event_id = ?", id).First(&existing).Error; err == gorm.ErrRecordNotFound {
			catID := seedUUID("cat-" + e.category)
			orgID, ok := s.orgByKey[e.orgName]
			if !ok {
				return fmt.Errorf("org %q not found for event %q", e.orgName, e.title)
			}
			locID := seedUUID("loc-" + e.location)
			price, err := decimal.NewFromString(e.price)
			if err != nil {
				return fmt.Errorf("invalid price %q: %w", e.price, err)
			}
			ev := model.EventModel{
				EventID:     id,
				Title:       e.title,
				Description: ptrString(e.desc),
				StartTime:   now.AddDate(0, 0, e.startDays),
				EndTime:     now.AddDate(0, 0, e.endDays),
				Capacity:    e.capacity,
				Status:      e.status,
				Price:       price,
				CategoryID:  &catID,
				OrganizerID: &orgID,
				LocationID:  &locID,
			}
			if err := s.db.Create(&ev).Error; err != nil {
				return fmt.Errorf("create event %q: %w", e.title, err)
			}
		}
	}
	return nil
}

type paymentDef struct {
	tag         string
	amount      string
	status      model.PaymentStatus
	refundAmt   string
}

func (s *DBSeeder) seedPayments(_ context.Context) error {
	log.Println("db: seeding payments...")
	payments := []paymentDef{
		{"pay-1", "249.50", model.PaymentStatusPaid, "0.00"},
		{"pay-2", "89.00", model.PaymentStatusPaid, "0.00"},
		{"pay-3", "24.50", model.PaymentStatusPaid, "0.00"},
		{"pay-4", "36.00", model.PaymentStatusPaid, "0.00"},
		{"pay-5", "48.00", model.PaymentStatusPaid, "0.00"},
		{"pay-6", "18.00", model.PaymentStatusPending, "0.00"},
		{"pay-7", "16.00", model.PaymentStatusFailed, "0.00"},
		{"pay-8", "65.00", model.PaymentStatusRefunded, "65.00"},
		{"pay-9", "64.00", model.PaymentStatusPaid, "0.00"},
		{"pay-10", "28.00", model.PaymentStatusPaid, "0.00"},
		{"pay-11", "50.00", model.PaymentStatusPaid, "0.00"},
		{"pay-12", "42.00", model.PaymentStatusPaid, "0.00"},
		{"pay-13", "55.00", model.PaymentStatusPaid, "0.00"},
		{"pay-14", "0.00", model.PaymentStatusPaid, "0.00"},
		{"pay-15", "20.00", model.PaymentStatusPaid, "0.00"},
		{"pay-16", "20.00", model.PaymentStatusPaid, "0.00"},
		{"pay-17", "105.00", model.PaymentStatusPaid, "0.00"},
		{"pay-18", "18.00", model.PaymentStatusPaid, "0.00"},
		{"pay-19", "45.00", model.PaymentStatusPaid, "0.00"},
		{"pay-20", "60.00", model.PaymentStatusPaid, "0.00"},
		{"pay-21", "150.00", model.PaymentStatusPending, "0.00"},
		{"pay-22", "32.00", model.PaymentStatusPaid, "0.00"},
		{"pay-23", "99.80", model.PaymentStatusPaid, "0.00"},
		{"pay-24", "56.00", model.PaymentStatusPaid, "0.00"},
		{"pay-25", "12.00", model.PaymentStatusPaid, "0.00"},
		{"pay-26", "89.00", model.PaymentStatusPending, "0.00"},
		{"pay-27", "49.00", model.PaymentStatusPaid, "0.00"},
		{"pay-28", "18.00", model.PaymentStatusPaid, "0.00"},
		{"pay-29", "36.00", model.PaymentStatusFailed, "0.00"},
		{"pay-30", "65.00", model.PaymentStatusRefunded, "65.00"},
	}
	for _, p := range payments {
		id := seedUUID(p.tag)
		var existing model.PaymentModel
		if err := s.db.Where("payment_id = ?", id).First(&existing).Error; err == gorm.ErrRecordNotFound {
			amt, _ := decimal.NewFromString(p.amount)
			refund, _ := decimal.NewFromString(p.refundAmt)
			pm := model.PaymentModel{
				PaymentID:    id,
				Amount:       amt,
				Status:       p.status,
				RefundAmount: refund,
			}
			if err := s.db.Create(&pm).Error; err != nil {
				return fmt.Errorf("create payment %q: %w", p.tag, err)
			}
		}
	}
	return nil
}

type bookingDef struct {
	tag       string
	userEmail string
	eventTag  string
	payTag    string
	numTix    int
	status    model.BookingStatus
}

func (s *DBSeeder) seedBookings(_ context.Context) error {
	log.Println("db: seeding bookings...")
	bookings := []bookingDef{
		{"bkg-1", "visitor@eventhub.de", "evt-1", "pay-1", 5, model.BookingStatusConfirmed},
		{"bkg-2", "visitor@eventhub.de", "evt-2", "pay-2", 1, model.BookingStatusConfirmed},
		{"bkg-3", "visitor2@eventhub.de", "evt-3", "pay-3", 1, model.BookingStatusConfirmed},
		{"bkg-4", "visitor3@eventhub.de", "evt-5", "pay-4", 3, model.BookingStatusConfirmed},
		{"bkg-5", "visitor@eventhub.de", "evt-6", "pay-5", 4, model.BookingStatusConfirmed},
		{"bkg-6", "visitor2@eventhub.de", "evt-7", "pay-6", 1, model.BookingStatusReserved},
		{"bkg-7", "visitor3@eventhub.de", "evt-10", "pay-7", 2, model.BookingStatusFailed},
		{"bkg-8", "visitor@eventhub.de", "evt-9", "pay-8", 1, model.BookingStatusCancelled},
		{"bkg-9", "visitor2@eventhub.de", "evt-11", "pay-9", 2, model.BookingStatusConfirmed},
		{"bkg-10", "visitor3@eventhub.de", "evt-12", "pay-10", 1, model.BookingStatusConfirmed},
		{"bkg-11", "visitor@eventhub.de", "evt-13", "pay-11", 2, model.BookingStatusConfirmed},
		{"bkg-12", "visitor2@eventhub.de", "evt-14", "pay-12", 3, model.BookingStatusConfirmed},
		{"bkg-13", "visitor3@eventhub.de", "evt-15", "pay-13", 1, model.BookingStatusConfirmed},
		{"bkg-14", "visitor4@eventhub.de", "evt-16", "pay-14", 4, model.BookingStatusConfirmed},
		{"bkg-15", "visitor5@eventhub.de", "evt-17", "pay-15", 2, model.BookingStatusConfirmed},
		{"bkg-16", "visitor6@eventhub.de", "evt-19", "pay-16", 1, model.BookingStatusConfirmed},
		{"bkg-17", "visitor@eventhub.de", "evt-20", "pay-17", 3, model.BookingStatusConfirmed},
		{"bkg-18", "visitor2@eventhub.de", "evt-21", "pay-18", 2, model.BookingStatusConfirmed},
		{"bkg-19", "visitor3@eventhub.de", "evt-22", "pay-19", 1, model.BookingStatusConfirmed},
		{"bkg-20", "visitor4@eventhub.de", "evt-23", "pay-20", 2, model.BookingStatusConfirmed},
		{"bkg-21", "visitor5@eventhub.de", "evt-24", "pay-21", 2, model.BookingStatusReserved},
		{"bkg-22", "visitor6@eventhub.de", "evt-11", "pay-22", 1, model.BookingStatusConfirmed},
		{"bkg-23", "max.multi@eventhub.de", "evt-1", "pay-23", 2, model.BookingStatusConfirmed},
		{"bkg-24", "eva.manager@provadis-hochschule.de", "evt-12", "pay-24", 2, model.BookingStatusConfirmed},
		{"bkg-25", "tom.finance@provadis-hochschule.de", "evt-5", "pay-25", 1, model.BookingStatusConfirmed},
		{"bkg-26", "lena.admin@telekom.de", "evt-2", "pay-26", 1, model.BookingStatusReserved},
		{"bkg-27", "visitor@eventhub.de", "evt-3", "pay-27", 2, model.BookingStatusConfirmed},
		{"bkg-28", "visitor2@eventhub.de", "evt-7", "pay-28", 1, model.BookingStatusConfirmed},
		{"bkg-29", "visitor3@eventhub.de", "evt-26", "pay-29", 2, model.BookingStatusFailed},
		{"bkg-30", "visitor4@eventhub.de", "evt-9", "pay-30", 1, model.BookingStatusCancelled},
	}
	for _, b := range bookings {
		id := seedUUID(b.tag)
		var existing model.BookingModel
		if err := s.db.Where("booking_id = ?", id).First(&existing).Error; err == gorm.ErrRecordNotFound {
			userID, ok := s.userByKey[b.userEmail]
			if !ok {
				return fmt.Errorf("user %q not found for booking %q", b.userEmail, b.tag)
			}
			eventID := seedUUID(b.eventTag)
			payID := seedUUID(b.payTag)
			bk := model.BookingModel{
				BookingID:       id,
				UserID:          &userID,
				EventID:         &eventID,
				PaymentID:       &payID,
				NumberOfTickets: b.numTix,
				Status:          b.status,
			}
			if err := s.db.Create(&bk).Error; err != nil {
				return fmt.Errorf("create booking %q: %w", b.tag, err)
			}
		}
	}
	return nil
}

type ratingDef struct {
	tag        string
	bookingTag string
	score      int
	text       string
	visible    bool
}

func (s *DBSeeder) seedRatings(_ context.Context) error {
	log.Println("db: seeding ratings...")
	ratings := []ratingDef{
		{"rat-1", "bkg-1", 5, "Amazing festival, great vibes and top DJs!", true},
		{"rat-2", "bkg-2", 5, "Epic night of rock music, worth every cent.", true},
		{"rat-3", "bkg-3", 4, "Hilarious show, laughed all night.", true},
		{"rat-4", "bkg-9", 4, "Smooth jazz and great atmosphere.", false},
		{"rat-5", "bkg-13", 5, "World-class classical performance, stunning venue.", true},
		{"rat-6", "bkg-17", 4, "Great Oktoberfest vibe, plenty of beer and food.", true},
		{"rat-7", "bkg-20", 5, "Insane esports atmosphere, would go again!", true},
		{"rat-8", "bkg-27", 4, "Comedy night was a blast, highly recommend.", false},
		{"rat-9", "bkg-23", 3, "Good festival but a bit overcrowded.", true},
		{"rat-10", "bkg-11", 5, "Well-organized marathon expo, lots of stalls.", true},
	}
	for _, r := range ratings {
		id := seedUUID(r.tag)
		var existing model.RatingModel
		if err := s.db.Where("rating_id = ?", id).First(&existing).Error; err == gorm.ErrRecordNotFound {
			bkID := seedUUID(r.bookingTag)
			rt := model.RatingModel{
				RatingID:  id,
				BookingID: &bkID,
				Score:     r.score,
				Text:      r.text,
				IsVisible: r.visible,
			}
			if err := s.db.Create(&rt).Error; err != nil {
				return fmt.Errorf("create rating %q: %w", r.tag, err)
			}
		}
	}
	return nil
}

type notificationDef struct {
	tag       string
	userEmail string
	subject   string
	content   string
}

func (s *DBSeeder) seedNotifications(_ context.Context) error {
	log.Println("db: seeding notifications...")
	notifs := []notificationDef{
		{"not-1", "visitor@eventhub.de", "Booking confirmed", "Your booking for Summer Beats Festival is confirmed. 5 tickets reserved."},
		{"not-2", "visitor@eventhub.de", "Event reminder", "Rock am Ring Preview starts in 2 days at Olympiahalle München."},
		{"not-3", "visitor2@eventhub.de", "Booking confirmed", "Your booking for Bremen Comedy Night is confirmed."},
		{"not-4", "visitor3@eventhub.de", "Payment failed", "Your payment for Street Food Festival Bremen failed. Please try again."},
		{"not-5", "organizer@telekom.de", "New booking on your event", "A visitor just booked 2 tickets for Jazz & Blues Night."},
		{"not-6", "visitor4@eventhub.de", "Booking confirmed", "Your booking for Football Fan Fest Frankfurt is confirmed. 4 tickets."},
		{"not-7", "visitor5@eventhub.de", "Payment pending", "Your payment for New Year's Eve Gala Hamburg is still pending. Please complete it soon."},
		{"not-8", "visitor2@eventhub.de", "Event reminder", "Berlin Beer Festival starts tomorrow at Treptower Park Berlin."},
		{"not-9", "organizer@telekom.de", "New booking on your event", "A visitor just booked 3 tickets for Oktoberfest Preview München."},
		{"not-10", "visitor3@eventhub.de", "Refund processed", "Your refund for Winter Gala Concert has been processed."},
	}
	for _, n := range notifs {
		id := seedUUID(n.tag)
		var existing model.NotificationData
		if err := s.db.Where("notification_id = ?", id).First(&existing).Error; err == gorm.ErrRecordNotFound {
			userID, ok := s.userByKey[n.userEmail]
			if !ok {
				return fmt.Errorf("user %q not found for notification %q", n.userEmail, n.tag)
			}
			nd := model.NotificationData{
				NotificationID: id,
				UserID:         &userID,
				Subject:        n.subject,
				Content:        n.content,
			}
			if err := s.db.Create(&nd).Error; err != nil {
				return fmt.Errorf("create notification %q: %w", n.tag, err)
			}
		}
	}
	return nil
}
