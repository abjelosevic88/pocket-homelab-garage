package store

// User is an account that can sign in. The first user is the admin; admins
// manage other users and the shared settings. Vehicles are shared with members.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Lists the UI offers; free text is always allowed where it makes sense.
var (
	VehicleKinds     = []string{"car", "motorcycle", "van", "truck", "camper", "bike", "boat", "tractor", "other"}
	FuelTypes        = []string{"petrol", "diesel", "electric", "hybrid", "plugin_hybrid", "lpg", "cng", "other"}
	RecordKinds      = []string{"service", "repair", "upgrade", "expense"}
	PlanStages       = []string{"backlog", "planned", "in_progress", "testing", "done"}
	Priorities       = []string{"low", "normal", "high", "critical"}
	DocumentKinds    = []string{"registration", "insurance", "invoice", "receipt", "manual", "inspection", "photo", "other"}
	ServiceCategory  = []string{"oil", "filters", "tyres", "brakes", "battery", "timing belt", "spark plugs", "coolant", "transmission", "suspension", "exhaust", "clutch", "electrics", "body", "glass", "ac", "inspection", "detailing", "other"}
	ExpenseCategory  = []string{"registration", "insurance", "tax", "inspection", "parking", "toll", "wash", "fine", "accessories", "financing", "other"}
	VehicleStatuses  = []string{"active", "sold", "archived"}
	ReminderStatuses = []string{"active", "done", "dismissed"}
)

func in(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ValidRecordKind reports whether k is a record kind.
func ValidRecordKind(k string) bool { return in(RecordKinds, k) }

// Vehicle is one thing with wheels (or a hull, or tracks).
type Vehicle struct {
	ID             int64   `json:"id"`
	OwnerID        int64   `json:"owner_id"`
	Name           string  `json:"name"`
	Year           int     `json:"year"`
	Make           string  `json:"make"`
	Model          string  `json:"model"`
	Trim           string  `json:"trim"`
	Plate          string  `json:"plate"`
	VIN            string  `json:"vin"`
	Kind           string  `json:"kind"`
	FuelType       string  `json:"fuel_type"`
	OdometerUnit   string  `json:"odometer_unit"`
	TankCapacity   float64 `json:"tank_capacity"`
	Currency       string  `json:"currency"`
	Color          string  `json:"color"`
	ImagePath      string  `json:"-"`
	HasImage       bool    `json:"has_image"`
	PurchaseDate   string  `json:"purchase_date"`
	PurchasePrice  int64   `json:"purchase_price"`
	PurchaseOdo    float64 `json:"purchase_odo"`
	SoldDate       string  `json:"sold_date"`
	SoldPrice      int64   `json:"sold_price"`
	Status         string  `json:"status"`
	OdometerOffset float64 `json:"odometer_offset"`
	Notes          string  `json:"notes"`
	Tags           string  `json:"tags"`
	FireflyTag     string  `json:"firefly_tag"`
	FireflyAccount string  `json:"firefly_account"`
	Dawarich       bool    `json:"dawarich"`
	SortOrder      int     `json:"sort_order"`
	ExternalID     string  `json:"external_id"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`

	// Derived
	DisplayName string          `json:"display_name"`
	Role        string          `json:"role,omitempty"`
	Members     []VehicleMember `json:"members,omitempty"`
	Summary     *VehicleSummary `json:"summary,omitempty"`
}

// Title is the nickname or "year make model".
func (v *Vehicle) Title() string {
	if v.Name != "" {
		return v.Name
	}
	s := ""
	if v.Year > 0 {
		s = itoa(v.Year) + " "
	}
	s += v.Make + " " + v.Model
	return trim(s)
}

// VehicleMember is a user with access to a vehicle.
type VehicleMember struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// VehicleSummary is what a list card and the dashboard need.
type VehicleSummary struct {
	Odometer      float64          `json:"odometer"` // latest confirmed reading (with offset)
	OdometerDate  string           `json:"odometer_date"`
	OdometerEst   float64          `json:"odometer_estimate"` // reading + Dawarich km since
	EstimateKm    float64          `json:"estimate_km"`       // Dawarich km since the reading
	CostTotal     int64            `json:"cost_total"`        // all time, vehicle currency
	CostYear      int64            `json:"cost_year"`
	CostMonth     int64            `json:"cost_month"`
	CostFuelTotal int64            `json:"cost_fuel_total"`
	CostByKind    map[string]int64 `json:"cost_by_kind"`
	Fills         int              `json:"fills"`
	Records       int              `json:"records"`
	Reminders     int              `json:"reminders"`     // active
	RemindersDue  int              `json:"reminders_due"` // due or overdue
	RemindersSoon int              `json:"reminders_soon"`
	NextReminder  *Reminder        `json:"next_reminder,omitempty"`
	Economy       float64          `json:"economy"`        // l/100km (or kWh/100km), 0 = unknown
	EconomyRecent float64          `json:"economy_recent"` // last 5 full tanks
	CostPerKm     float64          `json:"cost_per_km"`    // minor units per km, all costs
	FuelPerKm     float64          `json:"fuel_per_km"`    // minor units per km, fuel only
	KmPerYear     float64          `json:"km_per_year"`
	LastFuel      *Fuel            `json:"last_fuel,omitempty"`
	LastService   *Record          `json:"last_service,omitempty"`
	OwnedDays     int              `json:"owned_days"`
	Plans         int              `json:"plans"` // not done
}

// Record is a service, repair, upgrade or expense line.
type Record struct {
	ID          int64    `json:"id"`
	VehicleID   int64    `json:"vehicle_id"`
	Kind        string   `json:"kind"`
	Category    string   `json:"category"`
	Date        string   `json:"date"`
	Odometer    *float64 `json:"odometer"`
	Description string   `json:"description"`
	Notes       string   `json:"notes"`
	Vendor      string   `json:"vendor"`
	Cost        int64    `json:"cost"`
	Currency    string   `json:"currency"`
	CostHome    int64    `json:"cost_home"`
	FXRate      float64  `json:"fx_rate"`
	Parts       []Part   `json:"parts"`
	Tags        string   `json:"tags"`
	Source      string   `json:"source"`
	ExternalID  string   `json:"external_id"`
	FireflyID   string   `json:"firefly_id"`
	CreatedBy   *int64   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Documents   int      `json:"documents,omitempty"`
	MoneyTxID   *int64   `json:"money_tx_id"`
	MoneyUserID *int64   `json:"money_user_id"`
	MoneyError  string   `json:"money_error,omitempty"` // set on the response when booking in Pocket Money failed
	// InventoryError: parts picked from Pocket Inventory could not be moved (response only)
	InventoryError string `json:"inventory_error,omitempty"`
}

// Part is one line inside a record: what was fitted or used.
type Part struct {
	Name        string  `json:"name"`
	Qty         float64 `json:"qty"`
	Cost        int64   `json:"cost"` // minor units, record currency, for the quantity
	PartNo      string  `json:"part_no"`
	InventoryID int64   `json:"inventory_id,omitempty"`
	Link        string  `json:"link,omitempty"`
}

// Fuel is one fill-up or charge.
type Fuel struct {
	ID          int64    `json:"id"`
	VehicleID   int64    `json:"vehicle_id"`
	Date        string   `json:"date"`
	Odometer    float64  `json:"odometer"`
	Volume      float64  `json:"volume"`
	Cost        int64    `json:"cost"`
	Currency    string   `json:"currency"`
	CostHome    int64    `json:"cost_home"`
	FXRate      float64  `json:"fx_rate"`
	FullTank    bool     `json:"full_tank"`
	Missed      bool     `json:"missed"`
	Station     string   `json:"station"`
	Grade       string   `json:"grade"`
	SocStart    float64  `json:"soc_start"`
	SocEnd      float64  `json:"soc_end"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Notes       string   `json:"notes"`
	Tags        string   `json:"tags"`
	Source      string   `json:"source"`
	ExternalID  string   `json:"external_id"`
	FireflyID   string   `json:"firefly_id"`
	CreatedBy   *int64   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	MoneyTxID   *int64   `json:"money_tx_id"`
	MoneyUserID *int64   `json:"money_user_id"`
	MoneyError  string   `json:"money_error,omitempty"`

	// Derived on read (oldest → newest by odometer)
	Distance  float64 `json:"distance"`    // since the previous fill
	Economy   float64 `json:"economy"`     // volume per 100 distance units over the stretch ending here (0 = n/a)
	PricePer  float64 `json:"price_per"`   // minor units per volume unit
	CostPerKm float64 `json:"cost_per_km"` // minor units per distance unit over the stretch
}

// OdometerReading is one point on the odometer line, from any source.
type OdometerReading struct {
	ID         int64   `json:"id"`
	VehicleID  int64   `json:"vehicle_id"`
	Date       string  `json:"date"`
	Reading    float64 `json:"reading"`
	Notes      string  `json:"notes"`
	Source     string  `json:"source"` // manual | api | dawarich | lubelogger | fuel | record
	ExternalID string  `json:"external_id,omitempty"`
	LinkKind   string  `json:"link_kind,omitempty"`
	LinkID     int64   `json:"link_id,omitempty"`
	CreatedAt  string  `json:"created_at"`
	// Derived
	Delta float64 `json:"delta"` // since the previous reading
	Days  int     `json:"days"`
}

// Reminder is something to do by a date and/or an odometer.
type Reminder struct {
	ID            int64    `json:"id"`
	VehicleID     int64    `json:"vehicle_id"`
	Title         string   `json:"title"`
	Notes         string   `json:"notes"`
	Kind          string   `json:"kind"`
	Category      string   `json:"category"`
	DueDate       string   `json:"due_date"`
	DueOdometer   *float64 `json:"due_odometer"`
	IntervalDays  int      `json:"interval_days"`
	IntervalKm    float64  `json:"interval_km"`
	SoonDays      int      `json:"soon_days"`
	SoonKm        float64  `json:"soon_km"`
	Status        string   `json:"status"`
	LastDoneDate  string   `json:"last_done_date"`
	LastDoneOdo   *float64 `json:"last_done_odo"`
	NotifiedLevel string   `json:"notified_level"`
	NotifiedAt    string   `json:"notified_at"`
	ExternalID    string   `json:"external_id"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`

	// Derived against today and the current odometer estimate
	Urgency     string   `json:"urgency"` // ok | soon | due | overdue | done
	DaysLeft    *int     `json:"days_left"`
	KmLeft      *float64 `json:"km_left"`
	VehicleName string   `json:"vehicle_name,omitempty"`
}

// Plan is a card on the planner board.
type Plan struct {
	ID         int64  `json:"id"`
	VehicleID  int64  `json:"vehicle_id"`
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	Kind       string `json:"kind"`
	Category   string `json:"category"`
	Priority   string `json:"priority"`
	Stage      string `json:"stage"`
	Estimate   int64  `json:"estimate"`
	TargetDate string `json:"target_date"`
	RecordID   *int64 `json:"record_id"`
	SortOrder  int    `json:"sort_order"`
	ExternalID string `json:"external_id"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// Note is a pinned or plain note on a vehicle.
type Note struct {
	ID         int64  `json:"id"`
	VehicleID  int64  `json:"vehicle_id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Pinned     bool   `json:"pinned"`
	ExternalID string `json:"external_id"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// Document is an uploaded file.
type Document struct {
	ID        int64  `json:"id"`
	VehicleID int64  `json:"vehicle_id"`
	UserID    *int64 `json:"user_id"`
	Name      string `json:"name"`
	Filename  string `json:"-"`
	Mime      string `json:"mime"`
	Bytes     int64  `json:"bytes"`
	Kind      string `json:"kind"`
	ExpiresOn string `json:"expires_on"`
	LinkKind  string `json:"link_kind"`
	LinkID    int64  `json:"link_id"`
	CreatedAt string `json:"created_at"`
}

// Activity is one line of the vehicle's change log.
type Activity struct {
	ID        int64  `json:"id"`
	VehicleID int64  `json:"vehicle_id"`
	UserID    *int64 `json:"user_id"`
	UserName  string `json:"user_name"`
	Action    string `json:"action"`
	Subject   string `json:"subject"`
	CreatedAt string `json:"created_at"`
}

// DawarichDay is one day of recorded driving.
type DawarichDay struct {
	Date    string  `json:"date"`
	Km      float64 `json:"km"`
	TotalKm float64 `json:"total_km"`
}

// Settings are preferences stored as key/value pairs, shared by everyone.
type Settings struct {
	Title           string `json:"title"`
	HomeCurrency    string `json:"home_currency"`
	DateFormat      string `json:"date_format"`
	TimeFormat      string `json:"time_format"`
	DistanceUnit    string `json:"distance_unit"` // km | mi
	VolumeUnit      string `json:"volume_unit"`   // l | gal_us | gal_uk
	EconomyUnit     string `json:"economy_unit"`  // l100 | kml | mpg_us | mpg_uk
	FXAuto          bool   `json:"fx_auto"`
	FireflyURL      string `json:"firefly_url"`
	FireflyToken    string `json:"firefly_token"`
	FireflyCategory string `json:"firefly_category"` // category for pushed withdrawals
	FireflyAccount  string `json:"firefly_account"`  // default source account name/id
	FireflyAutoSync bool   `json:"firefly_auto_sync"`
	DawarichURL     string `json:"dawarich_url"`
	DawarichAPIKey  string `json:"dawarich_api_key"`
	NotifyURL       string `json:"notify_url"`
	NotifyFormat    string `json:"notify_format"` // ntfy | apprise | json
	NotifyHour      int    `json:"notify_hour"`
	NotifyDigest    bool   `json:"notify_digest"` // weekly Monday digest
	TravelURL       string `json:"travel_url"`
	TravelToken     string `json:"travel_token"`
	TravelPublicURL string `json:"travel_public_url"` // what the browser opens when travel_url is internal
	InventoryURL    string `json:"inventory_url"`
	InventoryToken  string `json:"inventory_token"`
	InventoryTake   bool   `json:"inventory_take"` // parts picked from Pocket Inventory come off the shelf
	// How Garage reaches each Pocket app: core | direct | off ("" = direct when a token is set, else core).
	TravelMode    string `json:"travel_mode"`
	InventoryMode string `json:"inventory_mode"`
	MoneyMode     string `json:"money_mode"`
	// How Garage reaches the person's Dawarich: core (URL + their key from
	// Pocket Core) | direct (dawarich_url + key above) | off; "" = direct when
	// a key is saved, else core when Garage is connected to Core, else direct.
	DawarichMode string `json:"dawarich_mode"`
	// Pocket Money: through a Pocket Core link, or a Money URL + token without Core.
	MoneyEnabled      bool   `json:"money_enabled"` // offer "Create a transaction" (ticked) when adding costs
	MoneyURL          string `json:"money_url"`
	MoneyToken        string `json:"money_token"`
	MoneyAccount      int64  `json:"money_account"`          // default account to book on
	MoneyCategoryFuel int64  `json:"money_category_fuel"`    // category for fill-ups (0 = by name)
	MoneyCategory     int64  `json:"money_category"`         // category for everything else (0 = by name)
	MoneyVehicleAccts string `json:"money_vehicle_accounts"` // JSON {vehicle id: account id}, the last account used per vehicle
	SetupComplete     bool   `json:"setup_complete"`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func trim(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
