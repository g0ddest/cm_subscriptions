package models

type EnrichmentMsg struct {
	ID               string   `json:"id"`
	MP               string   `json:"mp"`
	Organization     string   `json:"organization"`
	ShortDescription string   `json:"short_description"`
	Event            string   `json:"event"`
	EventStart       string   `json:"event_start"`
	EventStop        *string  `json:"event_stop"`
	City             string   `json:"city"`
	StreetType       *string  `json:"street_type"`
	StreetTypeRaw    string   `json:"street_type_raw"`
	Street           string   `json:"street"`
	Service          string   `json:"service"`
	HouseNumbers     []string `json:"house_numbers"`
	HouseRanges      []string `json:"house_ranges"`
	RegionKladr      string   `json:"region_kladr,omitempty"`
	RegionName       string   `json:"region_name,omitempty"`
	RegionType       string   `json:"region_type,omitempty"`
	StreetKladr      string   `json:"street_kladr,omitempty"`
	StreetName       string   `json:"street_name,omitempty"`
	StreetTypeFull   string   `json:"street_type_full,omitempty"`
	CityKladr        string   `json:"city_kladr,omitempty"`
	CityName         string   `json:"city_name,omitempty"`
	CityType         string   `json:"city_type,omitempty"`
}
