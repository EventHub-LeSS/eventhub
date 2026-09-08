package seed

const defaultPassword = "password"

type mockUser struct {
	Username  string
	Email     string
	FirstName string
	LastName  string
	Phone     string
}

type mockMembership struct {
	UserUsername string
	OrgAlias     string
	Role         string // org_admin | event_manager | finance_viewer
}

type mockOrg struct {
	Name            string
	Alias           string
	Description     string
	AdminUser       string
	ContactEmail    string
	ContactPhone    string
	Street          string
	HouseNumber     string
	PostalCode      string
	City            string
	CountryCode     string
}

var mockUsers = []mockUser{
	{Username: "großmeister_finn", Email: "finn.betz@grossmeister.de", FirstName: "Großmeister", LastName: "Finn", Phone: "+49 151 12345678"},
	{Username: "organizer@provadis-hochschule.de", Email: "organizer@provadis-hochschule.de", FirstName: "Merz", LastName: "Leck Eier2", Phone: "+49 69 12345679"},
	{Username: "organizer@telekom.de", Email: "organizer@telekom.de", FirstName: "Merz", LastName: "Leck Eier3", Phone: "+49 30 12345680"},
	{Username: "eva.manager@provadis-hochschule.de", Email: "eva.manager@provadis-hochschule.de", FirstName: "Eva", LastName: "Manager", Phone: "+49 69 12345681"},
	{Username: "tom.finance@provadis-hochschule.de", Email: "tom.finance@provadis-hochschule.de", FirstName: "Tom", LastName: "Finance", Phone: "+49 69 12345682"},
	{Username: "lena.admin@telekom.de", Email: "lena.admin@telekom.de", FirstName: "Lena", LastName: "Admin", Phone: "+49 30 12345683"},
	{Username: "max.multi@eventhub.de", Email: "max.multi@eventhub.de", FirstName: "Max", LastName: "Multi", Phone: "+49 151 98765432"},
	{Username: "visitor@eventhub.de", Email: "visitor@eventhub.de", FirstName: "Merz", LastName: "Leck Eier1", Phone: "+49 221 11111111"},
	{Username: "visitor2@eventhub.de", Email: "visitor2@eventhub.de", FirstName: "Anna", LastName: "Besuch", Phone: "+49 221 22222222"},
	{Username: "visitor3@eventhub.de", Email: "visitor3@eventhub.de", FirstName: "Lukas", LastName: "Gast", Phone: "+49 221 33333333"},
}

var mockOrgs = []mockOrg{
	{Name: "Provadis Hochschule", Alias: "provadis-hochschule", Description: "Demo organization", AdminUser: "großmeister_finn",
		ContactEmail: "info@provadis-hochschule.de", ContactPhone: "+49 69 98980", Street: "Ginnheimer Landstraße", HouseNumber: "133", PostalCode: "65760", City: "Frankfurt", CountryCode: "DE"},
	{Name: "Telekom", Alias: "telekom", Description: "Demo organization", AdminUser: "großmeister_finn",
		ContactEmail: "events@telekom.de", ContactPhone: "+49 228 1810", Street: "Friedrich-Ebert-Allee", HouseNumber: "140", PostalCode: "53113", City: "Bonn", CountryCode: "DE"},
	{Name: "ACME Events", Alias: "acme-events", Description: "Mock organization for demos", AdminUser: "großmeister_finn",
		ContactEmail: "hello@acme-events.de", ContactPhone: "+49 40 227090", Street: "Große Freiheit", HouseNumber: "7", PostalCode: "22767", City: "Hamburg", CountryCode: "DE"},
	{Name: "Stadthalle Bremen", Alias: "stadthalle-bremen", Description: "Mock organization for demos", AdminUser: "großmeister_finn",
		ContactEmail: "tickets@stadthalle-bremen.de", ContactPhone: "+49 421 98980", Street: "Findorffstraße", HouseNumber: "105", PostalCode: "28215", City: "Bremen", CountryCode: "DE"},
}

var mockMemberships = []mockMembership{
	{"großmeister_finn", "provadis-hochschule", "org_admin"},
	{"organizer@provadis-hochschule.de", "provadis-hochschule", "event_manager"},
	{"eva.manager@provadis-hochschule.de", "provadis-hochschule", "event_manager"},
	{"tom.finance@provadis-hochschule.de", "provadis-hochschule", "finance_viewer"},
	{"max.multi@eventhub.de", "provadis-hochschule", "org_admin"},

	{"großmeister_finn", "telekom", "org_admin"},
	{"organizer@telekom.de", "telekom", "event_manager"},
	{"lena.admin@telekom.de", "telekom", "org_admin"},
	{"max.multi@eventhub.de", "telekom", "event_manager"},

	{"großmeister_finn", "acme-events", "org_admin"},
	{"eva.manager@provadis-hochschule.de", "acme-events", "event_manager"},

	{"großmeister_finn", "stadthalle-bremen", "org_admin"},
	{"lena.admin@telekom.de", "stadthalle-bremen", "event_manager"},
}
