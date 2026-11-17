package seed

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/google/uuid"
	"github.com/henning-kln/gocloak"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// The live seeder builds the content of the live deployment through the real user flow where
// one exists: the three organizations are created by approving organizer applications that the
// seeded users "submitted" through the contact form, so applications, audit entries and the
// approval notifications look exactly like production data. Everything else (events, bookings,
// payments) is seeded like the mock data, with deterministic IDs, so the seeder is idempotent.

// liveAdminEmail is the global admin of the live realm (core/realms/live-realm.json), who
// reviews the organizer applications. Resolved by email, because the realm stores the username
// independently of it.
const liveAdminEmail = "admin@eventhub.de"

// liveDefaultPassword is the password of every seeded live account.
const liveDefaultPassword = "password"

// liveUsers are the accounts of the live deployment; they all get the password "password".
// Three of them applied for their organization through the contact form and are its
// administrators after the approval; the rest book events as visitors.
var liveUsers = []mockUser{
	{Username: "sabine.vogt@provadis-hochschule.de", Email: "sabine.vogt@provadis-hochschule.de", FirstName: "Sabine", LastName: "Vogt", Phone: "+49 69 3107221"},
	{Username: "jens.krueger@telekom.de", Email: "jens.krueger@telekom.de", FirstName: "Jens", LastName: "Krüger", Phone: "+49 228 181222"},
	{Username: "petra.sommer@stadthalle-bremen.de", Email: "petra.sommer@stadthalle-bremen.de", FirstName: "Petra", LastName: "Sommer", Phone: "+49 421 350511"},
	{Username: "flora.hartmann@gmx.de", Email: "flora.hartmann@gmx.de", FirstName: "Flora", LastName: "Hartmann", Phone: "+49 171 454510"},
	{Username: "timo.becker@web.de", Email: "timo.becker@web.de", FirstName: "Timo", LastName: "Becker", Phone: "+49 172 454521"},
	{Username: "marie.lange@gmail.com", Email: "marie.lange@gmail.com", FirstName: "Marie", LastName: "Lange", Phone: "+49 173 454532"},
	{Username: "felix.braun@outlook.de", Email: "felix.braun@outlook.de", FirstName: "Felix", LastName: "Braun", Phone: "+49 174 454543"},
	{Username: "nina.wolf@web.de", Email: "nina.wolf@web.de", FirstName: "Nina", LastName: "Wolf", Phone: "+49 175 454554"},
	{Username: "leon.berg@freenet.de", Email: "leon.berg@freenet.de", FirstName: "Leon", LastName: "Berg", Phone: "+49 176 454565"},
	{Username: "emil.weiss@gmail.com", Email: "emil.weiss@gmail.com", FirstName: "Emil", LastName: "Weiss", Phone: "+49 177 454576"},
}

// liveApplications are the organizer applications the three organization admins submitted
// through the contact form. Approval creates the organization with the applicant as org_admin,
// exactly like the review by a global admin in the UI.
type liveApplication struct {
	applicantEmail string
	orgName        string
	orgAlias       string
	contactEmail   string
	phone          string
	street         string
	houseNumber    string
	postalCode     string
	city           string
	message        string
}

var liveApplications = []liveApplication{
	{
		applicantEmail: "sabine.vogt@provadis-hochschule.de",
		orgName:        "Provadis Hochschule",
		orgAlias:       "provadis-hochschule",
		contactEmail:   "info@provadis-hochschule.de",
		phone:          "+49 69 98980",
		street:         "Ginnheimer Landstraße",
		houseNumber:    "133",
		postalCode:     "65760",
		city:           "Frankfurt",
		message: "Hallo EventHub-Team, wir sind das Veranstalterteam der Provadis Hochschule und planen " +
			"regelmäßig Campus-Open-Airs, Workshops und Netzwerktreffen in Frankfurt. Wir möchten unsere " +
			"Veranstaltungen künftig über EventHub veröffentlichen und verwalten. Viele Grüße, Sabine Vogt",
	},
	{
		applicantEmail: "jens.krueger@telekom.de",
		orgName:        "Telekom",
		orgAlias:       "telekom",
		contactEmail:   "events@telekom.de",
		phone:          "+49 228 1810",
		street:         "Friedrich-Ebert-Allee",
		houseNumber:    "140",
		postalCode:     "53113",
		city:           "Bonn",
		message: "Guten Tag, ich koordiniere Konzerte, Messen und Sportevents der Telekom in Bonn und " +
			"München. Wir möchten EventHub als Ticketplattform für unsere Unternehmens- und " +
			"Kundenveranstaltungen nutzen. Mit freundlichen Grüßen, Jens Krüger",
	},
	{
		applicantEmail: "petra.sommer@stadthalle-bremen.de",
		orgName:        "Stadthalle Bremen",
		orgAlias:       "stadthalle-bremen",
		contactEmail:   "tickets@stadthalle-bremen.de",
		phone:          "+49 421 98980",
		street:         "Findorffstraße",
		houseNumber:    "105",
		postalCode:     "28215",
		city:           "Bremen",
		message: "Hallo, die Stadthalle Bremen betreibt den Veranstaltungsort mitten in Bremen und richtet " +
			"jährlich über 100 Konzerte, Comedy-Abende und Ausstellungen aus. Wir möchten unsere " +
			"Veranstaltungen über EventHub verkaufen. Viele Grüße, Petra Sommer",
	},
}

// liveEvents are the events of the live deployment: ten upcoming, published events within the
// next 30 days and three completed events per organization in the past. The locations reuse the
// seeded venue rows, the categories the seeded category rows.
type liveEvent struct {
	tag           string
	title         string
	desc          string
	startDays     int // relative to the seed run; negative means the past
	startHour     int
	durationHours int
	capacity      int
	status        model.EventStatus
	price         string
	category      string
	orgAlias      string
	location      string
	image         string
}

var liveEvents = []liveEvent{
	// Upcoming: 4x Provadis, 3x Telekom, 3x Stadthalle Bremen.
	{"live-evt-1", "Campus Open Air", "Open-Air-Sommerfest der Provadis Hochschule mit Live-Bands, Foodstands und Campus-Führungen.", 5, 16, 9, 2000, model.EventStatusPublished, "29.90", "Music", "provadis-hochschule", "Frankfurt Festplatz", "festival"},
	{"live-evt-2", "Data Science Workshop", "Eintägiger Praxis-Workshop zu Datenanalyse und Machine Learning für Studierende.", 12, 9, 8, 80, model.EventStatusPublished, "15.00", "Exhibition", "provadis-hochschule", "Frankfurt Festplatz", "art-gallery"},
	{"live-evt-3", "Hochschul-Kickerturnier", "Das jährliche Fußballturnier der Hochschul-Fakultäten mit Zuschussverpflegung und Siegerehrung.", 19, 10, 10, 300, model.EventStatusPublished, "5.00", "Sports", "provadis-hochschule", "Frankfurt Festplatz", "festival"},
	{"live-evt-4", "Comedy Club Frankfurt", "Stand-up-Abend mit vier Comedians aus dem deutschen TV und Nachwuchstalenten.", 26, 20, 3, 250, model.EventStatusPublished, "19.90", "Comedy", "provadis-hochschule", "Frankfurt Festplatz", "stadthalle-bremen"},
	{"live-evt-5", "Telekom Sommerkonzert", "Open-Air-Konzert der Telekom Sommerbühne mit wechselnden Top-Acts und Gästen.", 9, 18, 5, 5000, model.EventStatusPublished, "49.50", "Music", "telekom", "Olympiahalle München", "festival"},
	{"live-evt-6", "Gaming Expo Bonn", "Zweitägige Messe mit Retro-Gaming-Zone, Turnieren und Studios neuere Titel.", 15, 10, 9, 3000, model.EventStatusPublished, "22.00", "Exhibition", "telekom", "Olympiahalle München", "art-gallery"},
	{"live-evt-7", "Food Truck Rallye Bonn", "Über 40 Food Trucks, Live-Musik und Picknick-Wiesen auf dem Rheinauen-Gelände.", 22, 11, 10, 4000, model.EventStatusPublished, "9.90", "Food & Drink", "telekom", "Frankfurt Festplatz", "festival"},
	{"live-evt-8", "Kino unterm Sternenhimmel", "Open-Air-Kino auf dem Stadthallen-Vorplatz mit Kultklassikern und Kurzfilm-Premieren.", 7, 21, 4, 600, model.EventStatusPublished, "11.50", "Film", "stadthalle-bremen", "Stadthalle Bremen", "festival"},
	{"live-evt-9", "Bremen Oldie Night", "Eine Nacht mit den größten Oldies der 60er bis 90er Jahre und sommerlichem Barbetrieb.", 17, 19, 5, 1500, model.EventStatusPublished, "34.90", "Music", "stadthalle-bremen", "Stadthalle Bremen", "stadthalle-bremen"},
	{"live-evt-10", "Comedy Slam Bremen", "Das Bremen Comedy Slam: acht Comedians, das Publikum kürt den Abendssieger.", 28, 19, 3, 400, model.EventStatusPublished, "24.90", "Comedy", "stadthalle-bremen", "Stadthalle Bremen", "stadthalle-bremen"},

	// Past: three completed events per organization.
	{"live-pevt-1", "Emerging Talents Konzert", "Jahreskonzert der Hochschulensembles mit Orchester, Chor und Jazz-Trio.", -21, 20, 4, 600, model.EventStatusCompleted, "45.00", "Music", "provadis-hochschule", "Musikhalle Hamburg", "stadthalle-bremen"},
	{"live-pevt-2", "Data Science Summit", "Fachtagung mit Vorträgen aus Wirtschaft und Forschung sowie Poster-Session.", -14, 9, 8, 150, model.EventStatusCompleted, "25.00", "Exhibition", "provadis-hochschule", "Frankfurt Festplatz", "art-gallery"},
	{"live-pevt-3", "Curry & Craft Party", "Semesterabschluss-Party mit Currywurst-Station und regionalen Craft-Bieren.", -7, 17, 6, 400, model.EventStatusCompleted, "12.00", "Food & Drink", "provadis-hochschule", "Frankfurt Festplatz", "festival"},
	{"live-pevt-4", "Telekom Jazz Nacht", "Abend mit der Telekom Corporate Jazz Band und Gästen in der Olympiahalle.", -20, 19, 4, 1200, model.EventStatusCompleted, "40.00", "Music", "telekom", "Olympiahalle München", "festival"},
	{"live-pevt-5", "Bonn Lauf Cup", "Firmenlauf der Telekom mit 5-km- und 10-km-Strecken durch die Rheinauen.", -13, 10, 6, 2500, model.EventStatusCompleted, "18.00", "Sports", "telekom", "Frankfurt Festplatz", "festival"},
	{"live-pevt-6", "Street Food Markt Bonn", "Street-Food-Markt mit regionalen und internationalen Ständen am Rheinufer.", -6, 11, 8, 3000, model.EventStatusCompleted, "8.50", "Food & Drink", "telekom", "Frankfurt Festplatz", "festival"},
	{"live-pevt-7", "Rock Legenden Bremen", "Konzertabend mit Cover-Hits der großen Rockklassiker und Lightshow.", -22, 20, 4, 1800, model.EventStatusCompleted, "55.00", "Music", "stadthalle-bremen", "Stadthalle Bremen", "festival"},
	{"live-pevt-8", "Kulinarische Nacht Bremen", "Gang-für-Gang-Menü mit Weinbegleitung über die Bühne der Stadthalle.", -15, 18, 5, 200, model.EventStatusCompleted, "39.00", "Food & Drink", "stadthalle-bremen", "Stadthalle Bremen", "stadthalle-bremen"},
	{"live-pevt-9", "Lachattacke Bremen", "Comedy-Gala mit fünf Comedians aus TV und Clubszene.", -8, 19, 3, 350, model.EventStatusCompleted, "27.50", "Comedy", "stadthalle-bremen", "Stadthalle Bremen", "stadthalle-bremen"},
}

// liveAdminBookings are the bookings of the admin account on past events; each carries its paid
// payment (transaction). One booking per organization's event.
var liveAdminBookings = []liveBooking{
	{"live-bkg-admin-1", liveAdminEmail, "live-pevt-1", 2, 35},
	{"live-bkg-admin-2", liveAdminEmail, "live-pevt-4", 2, 30},
	{"live-bkg-admin-3", liveAdminEmail, "live-pevt-7", 2, 36},
}

// liveUserBookings distribute the ten users over all upcoming events; each booking carries its
// paid payment.
var liveUserBookings = []liveBooking{
	{"live-bkg-1", "sabine.vogt@provadis-hochschule.de", "live-evt-1", 2, 3},
	{"live-bkg-2", "jens.krueger@telekom.de", "live-evt-5", 2, 4},
	{"live-bkg-3", "petra.sommer@stadthalle-bremen.de", "live-evt-8", 2, 2},
	{"live-bkg-4", "flora.hartmann@gmx.de", "live-evt-1", 2, 3},
	{"live-bkg-5", "flora.hartmann@gmx.de", "live-evt-8", 3, 3},
	{"live-bkg-6", "timo.becker@web.de", "live-evt-2", 1, 4},
	{"live-bkg-7", "timo.becker@web.de", "live-evt-7", 4, 7},
	{"live-bkg-8", "marie.lange@gmail.com", "live-evt-3", 2, 6},
	{"live-bkg-9", "felix.braun@outlook.de", "live-evt-5", 4, 4},
	{"live-bkg-10", "nina.wolf@web.de", "live-evt-6", 2, 8},
	{"live-bkg-11", "nina.wolf@web.de", "live-evt-9", 1, 5},
	{"live-bkg-12", "leon.berg@freenet.de", "live-evt-7", 2, 10},
	{"live-bkg-13", "emil.weiss@gmail.com", "live-evt-10", 2, 9},
	{"live-bkg-14", "emil.weiss@gmail.com", "live-evt-4", 3, 5},
}

type liveBooking struct {
	tag       string
	userEmail string
	eventTag  string
	tickets   int
	// bookedDaysAgo backdates CreatedAt of booking and payment; the value must be large enough
	// that the booking exists before its event starts.
	bookedDaysAgo int
}

// LiveSeeder builds the content of the live deployment (see the package comment of the data
// definitions above). It is idempotent: re-runs skip everything that already exists.
type LiveSeeder struct {
	db    *gorm.DB
	kc    *service.KeycloakService
	tx    repository.Transactor
	realm string
	token string

	userByEmail map[string]uuid.UUID
	orgByAlias  map[string]uuid.UUID
	eventByTag  map[string]model.EventModel

	adminSubject  string
	adminUsername string
}

func NewLiveSeeder(db *gorm.DB, kc *service.KeycloakService, tx repository.Transactor, realm, token string) *LiveSeeder {
	return &LiveSeeder{db: db, kc: kc, tx: tx, realm: realm, token: token}
}

func (s *LiveSeeder) SeedAll(ctx context.Context) error {
	// The repairs keep older realms working; on a freshly realm-synced live realm they are no-ops.
	if err := NewKeycloakSeeder(s.kc, s.realm, s.token).EnsureRealmRepairs(ctx); err != nil {
		return fmt.Errorf("realm repairs: %w", err)
	}

	if err := s.seedUsers(ctx); err != nil {
		return err
	}

	// The API database mirrors the Keycloak users (and the default global role grant runs here).
	seeder := NewSeeder(s.db, s.kc)
	if err := seeder.SyncDB(ctx, s.realm, s.token); err != nil {
		return fmt.Errorf("db sync: %w", err)
	}

	if err := s.resolveUsers(ctx); err != nil {
		return err
	}
	if err := s.resolveAdmin(ctx); err != nil {
		return err
	}
	if err := s.seedOrganizations(ctx); err != nil {
		return err
	}
	if err := s.seedCategoriesAndLocations(ctx); err != nil {
		return err
	}
	if err := s.seedEvents(ctx); err != nil {
		return err
	}
	return s.seedBookings(ctx)
}

// seedUsers creates the ten live accounts in Keycloak with the password "password".
func (s *LiveSeeder) seedUsers(ctx context.Context) error {
	log.Println("live seed: seeding users...")
	for _, mu := range liveUsers {
		existing, err := s.kc.GetUserByUsername(ctx, s.token, s.realm, mu.Username)
		if err != nil {
			return fmt.Errorf("lookup user %q: %w", mu.Username, err)
		}
		if existing != nil {
			continue
		}
		enabled := true
		emailVerified := true
		temporary := false
		credType := "password"
		userRep := gocloak.User{
			Username:      ptrString(mu.Username),
			Email:         ptrString(mu.Email),
			FirstName:     ptrString(mu.FirstName),
			LastName:      ptrString(mu.LastName),
			Enabled:       &enabled,
			EmailVerified: &emailVerified,
			Credentials:   &[]gocloak.CredentialRepresentation{{Type: &credType, Value: ptrString(liveDefaultPassword), Temporary: &temporary}},
		}
		if _, err := s.kc.CreateUserWithToken(ctx, s.token, s.realm, userRep); err != nil {
			return fmt.Errorf("create user %q: %w", mu.Username, err)
		}
		log.Printf("  created user %s", mu.Username)
	}
	return nil
}

// resolveAdmin finds the global admin by email, whose identity the approvals are recorded
// against (same fields the audit middleware takes from the access token).
func (s *LiveSeeder) resolveAdmin(ctx context.Context) error {
	admin, err := s.kc.GetUserByEmail(ctx, s.token, s.realm, liveAdminEmail)
	if err != nil {
		return err
	}
	if admin == nil || admin.ID == nil {
		return fmt.Errorf("admin %q not found in Keycloak; apply live-realm.json first (realm-sync)", liveAdminEmail)
	}
	s.adminSubject = *admin.ID
	s.adminUsername = liveAdminEmail
	if admin.Username != nil && *admin.Username != "" {
		s.adminUsername = *admin.Username
	}
	if _, ok := s.userByEmail[liveAdminEmail]; !ok {
		return fmt.Errorf("admin %q has no row in the API database; the db sync should have created it", liveAdminEmail)
	}
	return nil
}

func (s *LiveSeeder) resolveUsers(_ context.Context) error {
	var users []model.UserModel
	if err := s.db.Find(&users).Error; err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	s.userByEmail = make(map[string]uuid.UUID, len(users))
	for _, u := range users {
		if u.Email != "" {
			s.userByEmail[u.Email] = u.UserID
		}
	}
	for _, mu := range liveUsers {
		if _, ok := s.userByEmail[mu.Email]; !ok {
			return fmt.Errorf("user %q missing from the API database after sync", mu.Email)
		}
	}
	return nil
}

// seedOrganizations runs the three organizer applications through the real user flow: submit as
// the applicant, approve as the global admin. Approval creates the organization in Keycloak and
// the API database, activates it and records the decision with audit entry and notification.
func (s *LiveSeeder) seedOrganizations(ctx context.Context) error {
	log.Println("live seed: seeding organizer applications and approvals...")
	orgRepo := repository.NewOrganizationRepository(s.db)
	userRepo := repository.NewUserRepository(s.db)
	appRepo := repository.NewOrganizerApplicationRepository(s.db)
	orgService := service.NewOrganizationService(s.kc, orgRepo, userRepo)
	appService := service.NewOrganizerApplicationService(s.tx, appRepo, userRepo, orgRepo, orgService)

	for _, la := range liveApplications {
		if existing, err := orgRepo.GetByAnyRef("", la.orgAlias, la.orgName); err != nil {
			return fmt.Errorf("look up organization %q: %w", la.orgAlias, err)
		} else if existing != nil {
			log.Printf("  organization %q exists, application already approved", la.orgAlias)
			continue
		}

		userID, ok := s.userByEmail[la.applicantEmail]
		if !ok {
			return fmt.Errorf("applicant %q not found", la.applicantEmail)
		}

		// An interrupted earlier run may have left a pending application; reuse it instead of
		// failing on the one-pending-per-user constraint.
		applicationID, err := s.pendingApplicationID(appRepo, userID, la.orgAlias)
		if err != nil {
			return err
		}
		if applicationID == uuid.Nil {
			req := model.SubmitOrganizerApplicationRequest{
				OrganizationName:   la.orgName,
				OrganizationAlias:  la.orgAlias,
				ContactEmail:       la.contactEmail,
				ContactPhoneNumber: ptrString(la.phone),
				Street:             ptrString(la.street),
				HouseNumber:        ptrString(la.houseNumber),
				PostalCode:         ptrString(la.postalCode),
				City:               ptrString(la.city),
				CountryCode:        ptrString("DE"),
				Message:            la.message,
			}
			application, err := appService.Submit(userID, req)
			if err != nil {
				return fmt.Errorf("submit organizer application of %q: %w", la.orgAlias, err)
			}
			applicationID = application.ApplicationID
			log.Printf("  submitted organizer application of %q by %s", la.orgName, la.applicantEmail)
		}

		// The approval is recorded against the admin account, exactly like the audit middleware
		// would record a decision made in the UI.
		meta := audit.Meta{
			OperationID:   uuid.New(),
			Action:        audit.OrganizerApplicationApproved,
			ActorSubject:  s.adminSubject,
			ActorUsername: s.adminUsername,
		}
		if _, err := appService.Approve(audit.WithMeta(ctx, meta), applicationID, nil); err != nil {
			switch {
			case errors.Is(err, service.ErrApplicationNotPending), errors.Is(err, service.ErrOrganizationExists):
				log.Printf("  application of %q already decided (%v), skipping", la.orgAlias, err)
				continue
			default:
				return fmt.Errorf("approve organizer application of %q: %w", la.orgAlias, err)
			}
		}
		log.Printf("  approved organizer application of %q (admin %s)", la.orgName, s.adminUsername)
	}

	var orgs []model.OrganizationModel
	if err := s.db.Find(&orgs).Error; err != nil {
		return fmt.Errorf("load organizations: %w", err)
	}
	s.orgByAlias = make(map[string]uuid.UUID, len(orgs))
	for _, o := range orgs {
		s.orgByAlias[o.Alias] = o.OrganizationID
	}
	for _, la := range liveApplications {
		if _, ok := s.orgByAlias[la.orgAlias]; !ok {
			return fmt.Errorf("organization %q missing after approval", la.orgAlias)
		}
	}
	return nil
}

func (s *LiveSeeder) pendingApplicationID(appRepo repository.OrganizerApplicationRepository, userID uuid.UUID, orgAlias string) (uuid.UUID, error) {
	apps, err := appRepo.ListForUser(userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("list applications of user %s: %w", userID, err)
	}
	for _, app := range apps {
		if app.OrganizationAlias == orgAlias && app.Status == model.OrganizerApplicationPending {
			return app.ApplicationID, nil
		}
	}
	return uuid.Nil, nil
}

// seedCategoriesAndLocations reuses the seeded categories and venues so the live events use the
// same reference rows as the mock data.
func (s *LiveSeeder) seedCategoriesAndLocations(ctx context.Context) error {
	log.Println("live seed: seeding categories and locations...")
	dbSeeder := NewDBSeeder(s.db)
	if err := dbSeeder.seedCategories(ctx); err != nil {
		return err
	}
	return dbSeeder.seedLocations(ctx)
}

func (s *LiveSeeder) seedEvents(ctx context.Context) error {
	log.Println("live seed: seeding events...")
	images, err := seedEventImages()
	if err != nil {
		return err
	}
	now := time.Now()
	for _, e := range liveEvents {
		id := seedUUID(e.tag)
		var existing model.EventModel
		if err := s.db.Where("event_id = ?", id).First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			catID := seedUUID("cat-" + e.category)
			orgID, ok := s.orgByAlias[e.orgAlias]
			if !ok {
				return fmt.Errorf("organization %q not found for event %q", e.orgAlias, e.title)
			}
			locID := seedUUID("loc-" + e.location)
			price, err := decimal.NewFromString(e.price)
			if err != nil {
				return fmt.Errorf("invalid price %q: %w", e.price, err)
			}
			start := time.Date(now.Year(), now.Month(), now.Day(), e.startHour, 0, 0, 0, now.Location()).AddDate(0, 0, e.startDays)
			end := start.Add(time.Duration(e.durationHours) * time.Hour)
			ev := model.EventModel{
				EventID:     id,
				Title:       e.title,
				Description: ptrString(e.desc),
				StartTime:   start,
				EndTime:     end,
				Capacity:    e.capacity,
				Status:      e.status,
				Price:       price,
				CategoryID:  &catID,
				OrganizerID: &orgID,
				LocationID:  &locID,
			}
			if image, ok := eventImageFor(images, e.image); ok {
				ev.ImageBase64 = image.base64
				ev.ImageContentType = image.contentType
			}
			if err := s.db.Create(&ev).Error; err != nil {
				return fmt.Errorf("create event %q: %w", e.title, err)
			}
		} else if err != nil {
			return fmt.Errorf("find event %q: %w", e.title, err)
		}
	}

	var events []model.EventModel
	if err := s.db.Find(&events).Error; err != nil {
		return fmt.Errorf("load events: %w", err)
	}
	s.eventByTag = make(map[string]model.EventModel, len(liveEvents))
	for _, e := range liveEvents {
		for _, loaded := range events {
			if loaded.EventID == seedUUID(e.tag) {
				s.eventByTag[e.tag] = loaded
				break
			}
		}
	}
	return nil
}

// seedBookings books the admin onto past events and the users onto upcoming events; every
// booking carries its own paid payment (the transaction) whose amount is tickets times the
// event price, exactly like a checkout stores it.
func (s *LiveSeeder) seedBookings(_ context.Context) error {
	log.Println("live seed: seeding bookings with payments...")
	all := make([]liveBooking, 0, len(liveAdminBookings)+len(liveUserBookings))
	all = append(all, liveAdminBookings...)
	all = append(all, liveUserBookings...)

	now := time.Now()
	for _, b := range all {
		id := seedUUID(b.tag)
		var existing model.BookingModel
		if err := s.db.Where("booking_id = ?", id).First(&existing).Error; err == nil {
			continue
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find booking %q: %w", b.tag, err)
		}

		userID, ok := s.userByEmail[b.userEmail]
		if !ok {
			return fmt.Errorf("user %q not found for booking %q", b.userEmail, b.tag)
		}
		event, ok := s.eventByTag[b.eventTag]
		if !ok {
			return fmt.Errorf("event %q not found for booking %q", b.eventTag, b.tag)
		}
		line := event.Price.Mul(decimal.NewFromInt(int64(b.tickets)))

		bookedAt := now.AddDate(0, 0, -b.bookedDaysAgo)
		// Defensiv gegen Datenfehler: die Buchung liegt immer vor dem Eventstart.
		if bookedAt.After(event.StartTime) {
			bookedAt = event.StartTime.Add(-24 * time.Hour)
		}

		payID := seedUUID(b.tag + "-pay")
		var payment model.PaymentModel
		if err := s.db.Where("payment_id = ?", payID).First(&payment).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			pm := model.PaymentModel{
				PaymentID: payID,
				Amount:    line,
				Status:    model.PaymentStatusPaid,
				CreatedAt: bookedAt,
			}
			if err := s.db.Create(&pm).Error; err != nil {
				return fmt.Errorf("create payment for booking %q: %w", b.tag, err)
			}
		} else if err != nil {
			return fmt.Errorf("find payment for booking %q: %w", b.tag, err)
		}

		bk := model.BookingModel{
			BookingID:       id,
			UserID:          &userID,
			EventID:         &event.EventID,
			PaymentID:       &payID,
			NumberOfTickets: b.tickets,
			Status:          model.BookingStatusConfirmed,
			LineAmount:      &line,
			CreatedAt:       bookedAt,
		}
		if err := s.db.Create(&bk).Error; err != nil {
			return fmt.Errorf("create booking %q: %w", b.tag, err)
		}
	}
	return nil
}
