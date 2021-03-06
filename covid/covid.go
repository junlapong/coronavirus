package covid

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Data mutex to protect access to data
var mutex sync.RWMutex

// Store our data globally, use mutex to access
var data SeriesSlice

// Data types for imported series data
const (
	DataDeaths = iota
	DataConfirmed
	DataRecovered // No longer active
	DataTodayState
	DataTodayCountry
)

// Series stores data for one country or province within a country
type Series struct {
	// UTC Date data last updated
	UpdatedAt time.Time
	// The Country or Region
	Country string
	// The Province or State - may be blank for countries
	Province string
	// The date at which the series starts - all datasets must be the same length
	StartsAt time.Time
	// Total Deaths, Confirmed or Recovered by day (cumulative)
	Deaths    []int
	Confirmed []int
	//	Recovered []int

	// Daily totals
	DeathsDaily    []int
	ConfirmedDaily []int
}

// UpdatedAtDisplay retuns a string to display updated at date (if we have a date)
func (s *Series) UpdatedAtDisplay() string {
	if s.UpdatedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf("Data last updated at %s", s.UpdatedAt.Format("2006-01-02 15:04 MST"))
}

// Title returns a display title for this series
func (s *Series) Title() string {
	if s.Country == "" && s.Province == "" {
		return "Global"
	} else if s.Province == "" {
		return s.Country
	}

	return fmt.Sprintf("%s (%s)", s.Province, s.Country)
}

// Dates returns a set of date labels as an array of strings
// for every datapoint in this series
func (s *Series) Dates() (dates []string) {
	d := s.StartsAt
	for range s.Deaths {
		dates = append(dates, d.Format("Jan 2"))
		d = d.AddDate(0, 0, 1)
	}
	return dates
}

// FetchDate retuns the data for the given date from datum
func (s *Series) FetchDate(datum int, date time.Time) int {

	// Calculate index in series given StartsAt
	days := date.Sub(s.StartsAt)
	i := int(days.Hours() / 24)

	// Bounds check index
	if i < 0 || i > len(s.Deaths)-1 {
		return 0
	}

	// Fetch the data at index
	switch datum {
	case DataDeaths:
		return s.Deaths[i]
	case DataConfirmed:
		return s.Confirmed[i]
	}
	return 0
}

// Valid returns true if this series is valid
// a series without a start date set is considered invalid
func (s *Series) Valid() bool {
	return !s.StartsAt.IsZero()
}

// Key converts a value into one suitable for use in urls
func (s *Series) Key(v string) string {
	return strings.Replace(strings.ToLower(v), " ", "-", -1)
}

// Match returns true if this series matches data from a row
// performs a case insensitive match
func (s *Series) Match(country string, province string) bool {
	return s.Key(s.Country) == s.Key(country) && s.Key(s.Province) == s.Key(province)
}

// Merge the data from the incoming series with ours
// Merge is used also to load initial data into an empty series
func (s *Series) Merge(series *Series) {
	// If we are len 0 make sure we have enough space
	if len(s.Deaths) == 0 {
		s.Deaths = make([]int, len(series.Deaths))
		s.Confirmed = make([]int, len(series.Confirmed))
		s.DeathsDaily = make([]int, len(series.Deaths))
		s.ConfirmedDaily = make([]int, len(series.Confirmed))
	}

	// Then add to the data we have (if any)
	for i, d := range series.Deaths {
		s.Deaths[i] += d
	}
	for i, d := range series.Confirmed {
		s.Confirmed[i] += d
	}

	// Calculate daily totals
	// first day is just set to first total after that daily totals are stored
	for i := range series.Deaths {
		if i == 0 {
			s.DeathsDaily[i] = s.Deaths[i]
		} else {
			s.DeathsDaily[i] = s.Deaths[i] - s.Deaths[i-1]
		}
	}
	for i := range series.Confirmed {
		if i == 0 {
			s.ConfirmedDaily[i] = s.Confirmed[i]
		} else {
			s.ConfirmedDaily[i] = s.Confirmed[i] - s.Confirmed[i-1]
		}
	}

	// Update updated at on series
	if !series.UpdatedAt.IsZero() && (s.UpdatedAt.IsZero() || series.UpdatedAt.After(s.UpdatedAt)) {
		s.UpdatedAt = series.UpdatedAt
	}

}

// MergeFinalDay merges the final day of data
func (s *Series) MergeFinalDay(series *Series) error {
	if len(series.Confirmed) < 2 || len(series.Deaths) < 2 {
		return nil
	}

	if len(series.Confirmed) != len(s.Confirmed) {
		return fmt.Errorf("series: mismatch in days length for:%s", s.Country)
	}
	i := len(s.Confirmed) - 1

	s.Confirmed[i] += series.Confirmed[i]
	s.Deaths[i] += series.Deaths[i]
	s.DeathsDaily[i] += series.DeathsDaily[i]
	s.ConfirmedDaily[i] += series.ConfirmedDaily[i]

	if !series.UpdatedAt.IsZero() && (s.UpdatedAt.IsZero() || series.UpdatedAt.After(s.UpdatedAt)) {
		s.UpdatedAt = series.UpdatedAt
	}

	return nil
}

// Global returns true if this is the global series
func (s *Series) Global() bool {
	return s.Country == "" && s.Province == ""
}

// AddToGlobal returns true if this is the global series
func (s *Series) AddToGlobal() bool {
	// For provinces - add all
	if s.Province != "" {
		return true
	}

	// For countries, exclude those not in original dataset
	// usually because they have sub-country level data (e.g. US States)
	switch s.Country {
	case "":
		return false
	case "US":
		// the US province data is now missing - when added in perhaps remove it here?
	//	return false
	case "China":
		return false
	case "Australia":
		return false
	case "Canada":
		return false
	default:
		return true
	}

	return true
}

// Format formats a given number for display and returns a string
func (s *Series) Format(i int) string {
	if i < 10000 {
		return fmt.Sprintf("%d", i)
	}

	if i < 1000000 {
		return fmt.Sprintf("%.2fk", float64(i)/1000)
	}
	return fmt.Sprintf("%.2fm", float64(i)/1000000)
}

// DeathsDisplay returns a string representation of TotalDeaths
func (s *Series) DeathsDisplay() string {
	return s.Format(s.TotalDeaths())
}

// ConfirmedDisplay returns a string representation of TotalConfirmed
func (s *Series) ConfirmedDisplay() string {
	return s.Format(s.TotalConfirmed())
}

// ConfirmedToday returns a string representation of confirmed for last data in series
func (s *Series) ConfirmedToday() string {
	return s.Format(s.ConfirmedDaily[len(s.ConfirmedDaily)-1])
}

// DeathsToday returns a string representation of deaths for last data in series
func (s *Series) DeathsToday() string {
	return s.Format(s.DeathsDaily[len(s.DeathsDaily)-1])
}

// DailyData - for a given series of cumulative total ints,
// return daily data for them (by subtracting previous day totals)
// NB this requires more data than the range
func (s *Series) DailyData(start int, ints []int) (daily []int) {
	for i := start; i < len(ints); i++ {
		// For first entry in series, we can only store start value
		if i == 0 {
			daily = append(daily, ints[i])
		}
		daily = append(daily, ints[i]-ints[i-1])
	}
	return daily
}

// TotalDeaths returns the cumulative death due to COVID-19 for this series
func (s *Series) TotalDeaths() int {
	if len(s.Deaths) == 0 {
		return 0
	}
	if len(s.Deaths) < 2 || len(s.Deaths) > 60 {
		return s.Deaths[len(s.Deaths)-1]
	}
	return s.Deaths[len(s.Deaths)-1] - s.Deaths[0]
}

// TotalConfirmed returns the cumulative confirmed cases of COVID-19 for this series
func (s *Series) TotalConfirmed() int {
	if len(s.Confirmed) == 0 {
		return 0
	}
	if len(s.Confirmed) < 2 || len(s.Confirmed) > 60 {
		return s.Confirmed[len(s.Confirmed)-1]
	}
	return s.Confirmed[len(s.Confirmed)-1] - s.Confirmed[0]
}

// Days returns a copy of this series for just the given number of days in the past
func (s *Series) Days(days int) *Series {
	if days >= len(s.Deaths) {
		return s
	}

	i := len(s.Deaths) - days
	return &Series{
		Country:        s.Country,
		Province:       s.Province,
		StartsAt:       s.StartsAt.AddDate(0, 0, i),
		Deaths:         s.Deaths[i:],
		Confirmed:      s.Confirmed[i:],
		DeathsDaily:    s.DeathsDaily[i:],
		ConfirmedDaily: s.ConfirmedDaily[i:],
	}
}

// UpdateDaily updates the confirmed daily based on a new set of values for Confirmed
func (s *Series) UpdateDaily() {
	s.DeathsDaily = make([]int, len(s.Deaths))
	for i := range s.Deaths {
		if i == 0 {
			s.DeathsDaily[i] = s.Deaths[i]
		} else {
			s.DeathsDaily[i] = s.Deaths[i] - s.Deaths[i-1]
		}
	}

	s.ConfirmedDaily = make([]int, len(s.Confirmed))
	for i := range s.Confirmed {
		if i == 0 {
			s.ConfirmedDaily[i] = s.Confirmed[i]
		} else {
			s.ConfirmedDaily[i] = s.Confirmed[i] - s.Confirmed[i-1]
		}
	}
}

// AddDayData sets the data at dayIndex to the supplied data
// if necessary a day will be added
func (s *Series) AddDayData(dayIndex int, updated time.Time, confirmed, deaths int) {
	s.UpdatedAt = updated

	if dayIndex > len(s.Deaths)-1 {
		//	fmt.Printf("dayIndex:%d %d\n", dayIndex, len(s.Deaths))
		s.Deaths = append(s.Deaths, deaths)
		s.Confirmed = append(s.Confirmed, confirmed)
	} else {
		//	fmt.Printf("dayIndex exists:%d %d\n", dayIndex, len(s.Deaths))
		s.Deaths[dayIndex] = deaths
		s.Confirmed[dayIndex] = confirmed
	}

}

// SLICE OF Series

// SeriesSlice is a collection of Series
type SeriesSlice []*Series

func (slice SeriesSlice) Len() int      { return len(slice) }
func (slice SeriesSlice) Swap(i, j int) { slice[i], slice[j] = slice[j], slice[i] }

// Sort first on number of deaths, then on alpha order
func (slice SeriesSlice) Less(i, j int) bool {
	if slice[i].TotalDeaths() > 0 || slice[j].TotalDeaths() > 0 {
		return slice[i].TotalDeaths() > slice[j].TotalDeaths()
	}
	return slice[i].Country < slice[j].Country
}

// FetchDate fetches the datapiont for a given datum and date
func (slice SeriesSlice) FetchDate(country, province string, datum int, date time.Time) (int, error) {
	// Find the series, if none found return 0
	series, err := slice.FetchSeries(country, province)
	if err != nil {
		return 0, err
	}
	if !series.Valid() {
		return 0, fmt.Errorf("series: no such series")
	}

	return series.FetchDate(datum, date), nil
}

// FetchSeries returns a series (if found) for this combination of country and province
func (slice SeriesSlice) FetchSeries(country string, province string) (*Series, error) {

	for _, s := range slice {
		if s.Match(country, province) {
			return s, nil
		}
	}

	return &Series{}, fmt.Errorf("series: not found")
}

// PrintSeries uses our stored data to fetch a series
func (slice SeriesSlice) PrintSeries(country string, province string) error {
	s, err := slice.FetchSeries(country, province)
	if err != nil {
		log.Printf("error: series err:%s %s", country, err)
		return err
	}
	log.Printf("series:%s,%s %v %v", s.Country, s.Province, s.Confirmed, s.Deaths)
	return nil
}

// FetchSeries uses our stored data to fetch a series
func FetchSeries(country string, province string) (*Series, error) {
	mutex.RLock()
	defer mutex.RUnlock()
	return data.FetchSeries(country, province)
}

// CountryOptions uses our stored data to fetch country options
func CountryOptions() (options []Option) {
	mutex.RLock()
	defer mutex.RUnlock()
	return data.CountryOptions()
}

// ProvinceOptions uses our stored data to fetch province options for a country
func ProvinceOptions(country string) (options []Option) {
	mutex.RLock()
	defer mutex.RUnlock()
	return data.ProvinceOptions(country)
}

// PeriodOptions returns a set of options for period filters
func PeriodOptions() (options []Option) {

	options = append(options, Option{Name: "All Time", Value: "0"})
	options = append(options, Option{Name: "112 Days", Value: "112"})
	options = append(options, Option{Name: "56 Days", Value: "56"})
	options = append(options, Option{Name: "28 Days", Value: "28"})
	options = append(options, Option{Name: "14 Days", Value: "14"})
	options = append(options, Option{Name: "7 Days", Value: "7"})
	options = append(options, Option{Name: "3 Days", Value: "3"})
	options = append(options, Option{Name: "2 Days", Value: "2"})

	return options
}

// Option is used to generate options for selects in the view
type Option struct {
	Name  string
	Value string
}

// CountryOptions returns a set of options for the country dropdown (including a global one)
func (slice SeriesSlice) CountryOptions() (options []Option) {

	options = append(options, Option{Name: "Global", Value: ""})

	for _, s := range slice {
		if s.Province == "" && s.Country != "" {
			name := s.Country
			if s.TotalDeaths() > 0 {
				name = fmt.Sprintf("%s (%d Deaths)", s.Country, s.TotalDeaths())
			}
			options = append(options, Option{Name: name, Value: s.Key(s.Country)})
		}
	}

	return options
}

// ProvinceOptions returns a set of options for the province dropdown
// this should probably be based on the current country selection, and filtered from there
// to avoid inconsistency
// for now just show all which have province filled in.
func (slice SeriesSlice) ProvinceOptions(country string) (options []Option) {

	options = append(options, Option{Name: "All Areas", Value: ""})

	// Ignore UK and France for now as these are just outlying areas, not a breakdown
	if country == "United Kingdom" || country == "France" {
		return options
	}

	for _, s := range slice {
		if s.Country == country && s.Province != "" {
			name := s.Province
			if s.TotalDeaths() > 0 {
				name = fmt.Sprintf("%s (%d Deaths)", s.Province, s.TotalDeaths())
			}
			options = append(options, Option{Name: name, Value: s.Key(s.Province)})
		}
	}

	return options
}

// MergeCSV merges the data in this CSV with the data we already have in the SeriesSlice
func (slice SeriesSlice) MergeCSV(records [][]string, dataType int) (SeriesSlice, error) {

	// If daily data, merge it to existing last date
	switch dataType {
	case DataTodayCountry:
		return slice.mergeDailyCountryCSV(records, dataType)
	case DataTodayState:
		return slice.mergeDailyStateCSV(records, dataType)
	}

	return slice.mergeTimeSeriesCSV(records, dataType)
}

// mergeTimeSeriesCSV merges the data in this time series CSV with the data we already have in the SeriesSlice
func (slice SeriesSlice) mergeTimeSeriesCSV(records [][]string, dataType int) (SeriesSlice, error) {

	// Make an assumption about the starting date (checked below on header row)
	startDate := time.Date(2020, 1, 22, 0, 0, 0, 0, time.UTC)

	for i, row := range records {
		// Check header to see this is the file we expect, if not skip
		if i == 0 {
			// We just check a few cols - we assume the start date of the data won't change
			if row[0] != "Province/State" || row[1] != "Country/Region" || row[2] != "Lat" || row[4] != "1/22/20" {
				return slice, fmt.Errorf("load: error loading file - time series csv data format invalid")
			}

		} else {

			// Fetch data to match series
			country := row[1]
			province := row[0]

			// We ignore rows which match ,CA etc
			// these are US sub-state level data which is no longer included in the dataset and is zeroed out
			if country == "US" && strings.Contains(province, ", ") {
				//	fmt.Printf("ignoring series:%s %s\n", country, province)
				continue
			}

			// Ignore duplicate Virgin Islands
			if province == "Virgin Islands, U.S." {
				continue
			}

			// Fetch the series
			var series *Series
			series, _ = slice.FetchSeries(country, province)

			// If we don't have one yet, create one
			if !series.Valid() {
				series = &Series{
					Country:  country,
					Province: province,
					StartsAt: startDate,
				}
				slice = append(slice, series)
			}

			// Walk through row, reading days data after col 3 (longitude)
			for ii, d := range row {
				if ii < 4 {
					continue
				}
				var v int
				var err error
				if d != "" {
					v, err = strconv.Atoi(d)
					if err != nil {
						log.Printf("load: error loading series:%s row:%d col:%d row:\n%s\nerror:%s", country, i+1, ii+1, row, err)
						return slice, fmt.Errorf("load: error loading row %d - csv day data invalid:%s", i+1, err)
					}
				} else {
					// This is typically a clerical error - in this case invalid rows ending in ,
					// So just quietly ignore it
					log.Printf("load: missing data for series:%s row:%d col:%d", country, i, ii)
				}

				switch dataType {
				case DataDeaths:
					series.Deaths = append(series.Deaths, v)
				case DataConfirmed:
					series.Confirmed = append(series.Confirmed, v)
				}
			}

			// After reading row data, calculate confirmed daily from confirmed
			// first day is just set to first total after that daily totals are stored
			series.UpdateDaily()

		}

	}
	return slice, nil
}

// mergeDailyCountryCSV merges the data in this country daily series CSV with the data we already have in the SeriesSlice
func (slice SeriesSlice) mergeDailyCountryCSV(records [][]string, dataType int) (SeriesSlice, error) {

	log.Printf("load: merge daily country csv")

	// Make an assumption about the starting date - if this changes update
	startDate := time.Date(2020, 1, 22, 0, 0, 0, 0, time.UTC)

	// Calculate index in series given shared StartsAt vs today (we assume data in these files is for today)
	days := time.Now().UTC().Sub(startDate)
	dayIndex := int(days.Hours() / 24)

	// Bounds check index
	if dayIndex < 0 {
		return nil, fmt.Errorf("day index out of bounds")
	}

	for i, row := range records {
		// Check header to see this is the file we expect, if not skip
		if i == 0 {
			//Country_Region,Last_Update,Lat,Long_,Confirmed,Deaths,Recovered,Active
			// We just check a few cols - we assume the start date of the data won't change
			if row[0] != "Country_Region" || row[1] != "Last_Update" || row[2] != "Lat" || row[4] != "Confirmed" {
				return slice, fmt.Errorf("load: error loading file - daily country csv data format invalid")
			}

		} else {

			// Fetch data to match series
			country := row[0]
			province := ""

			// There are several province series with bad names or dates which are duplicated in the state level dataset
			// we therefore ignore them here as the data seems to be out of date anyway

			// Fetch the series
			series, err := slice.FetchSeries(country, province)
			if err != nil {
				log.Printf("load: warning reading daily series:%s error:%s", row[0], err)
				//return nil, fmt.Errorf("load: error reading daily series:%s error:%s", row[0], err)
			}

			// Get the series data from the row
			updated, confirmed, deaths, err := readCountryRow(row)
			if err != nil {
				return nil, fmt.Errorf("load: error reading row series:%s error:%s", row[0], err)
			}

			series.AddDayData(dayIndex, updated, confirmed, deaths)

			// After reading row data, recalculate confirmed daily from confirmed
			// first day is just set to first total after that daily totals are stored
			series.UpdateDaily()
		}

	}
	return slice, nil
}

func readCountryRow(row []string) (time.Time, int, int, error) {

	// Dates are, remarkably, in two different formats in one file
	// Try first in the one true format
	updated, err := time.Parse("2006-01-02 15:04:05", row[1])
	if err != nil {
		// Then try the US format  3/13/2020 22:22
		updated, err = time.Parse("1/2/2006 15:04", row[1])
		if err != nil {
			return updated, 0, 0, fmt.Errorf("load: error reading updated at series:%s error:%s", row[0], err)
		}
	}

	confirmed, err := strconv.Atoi(row[4])
	if err != nil {
		return updated, 0, 0, fmt.Errorf("load: error reading confirmed series:%s error:%s", row[0], err)
	}

	deaths, err := strconv.Atoi(row[5])
	if err != nil {
		return updated, 0, 0, fmt.Errorf("load: error reading deaths series:%s error:%s", row[0], err)
	}
	/*
		recovered, err := strconv.Atoi(row[6])
		if err != nil {
			return updated, 0, 0, 0, fmt.Errorf("load: error reading recovered series:%s error:%s", row[0], err)
		}
	*/
	return updated, confirmed, deaths, nil
}

// mergeDailyStateCSV merges the data in this state daily series CSV with the data we already have in the SeriesSlice
func (slice SeriesSlice) mergeDailyStateCSV(records [][]string, dataType int) (SeriesSlice, error) {

	log.Printf("load: merge daily state csv")

	// Make an assumption about the starting date - if this changes update
	startDate := time.Date(2020, 1, 22, 0, 0, 0, 0, time.UTC)

	// Calculate index in series given shared StartsAt vs today (we assume data in these files is for today)
	days := time.Now().UTC().Sub(startDate)
	dayIndex := int(days.Hours() / 24)

	// Bounds check index
	if dayIndex < 0 {
		return nil, fmt.Errorf("day index out of bounds")
	}

	for i, row := range records {
		// Check header to see this is the file we expect, if not skip
		if i == 0 {
			//FIPS,Province_State,Country_Region,Last_Update,Lat,Long_,Confirmed,Deaths,Recovered,Active
			// We just check a few cols - we assume the start date of the data won't change
			if row[0] != "FIPS" || row[1] != "Province_State" || row[2] != "Country_Region" || row[6] != "Confirmed" {
				return slice, fmt.Errorf("load: error loading file - daily country csv data format invalid")
			}

		} else {

			// Fetch data to match series
			country := row[2]
			province := row[1]

			if province == "Virgin Islands, U.S" {
				continue
			}

			// There are several province series with bad names or dates which are duplicated in the state level dataset
			// we therefore ignore them here as the data seems to be out of date anyway

			// Fetch the series
			series, err := slice.FetchSeries(country, province)
			if err != nil {
				log.Printf("load: warning reading daily state series:%s error:%s", row[1], err)
				//return nil, fmt.Errorf("load: error reading daily series:%s error:%s", row[0], err)
			}

			// Get the series data from the row
			updated, confirmed, deaths, err := readStateRow(row)
			if err != nil {
				return nil, fmt.Errorf("load: error reading row series:%s error:%s", row[1], err)
			}

			series.AddDayData(dayIndex, updated, confirmed, deaths)

			// After reading row data, recalculate confirmed daily from confirmed
			// first day is just set to first total after that daily totals are stored
			series.UpdateDaily()
		}

	}
	return slice, nil
}

func readStateRow(row []string) (time.Time, int, int, error) {

	// Dates are, remarkably, in two different formats in one file
	// Try first in the one true format
	var updated time.Time
	var err error
	if row[3] != "" {
		// Ignore blank dates
		updated, err = time.Parse("2006-01-02 15:04:05", row[3])
		if err != nil {
			// Then try the US format  3/13/2020 22:22
			updated, err = time.Parse("1/2/2006 15:04", row[3])
			if err != nil {
				return updated, 0, 0, fmt.Errorf("load: error reading updated at series:%s error:%s", row[1], err)
			}
		}
	}

	confirmed, err := strconv.Atoi(row[6])
	if err != nil {
		return updated, 0, 0, fmt.Errorf("load: error reading confirmed series:%s error:%s", row[1], err)
	}

	deaths, err := strconv.Atoi(row[7])
	if err != nil {
		return updated, 0, 0, fmt.Errorf("load: error reading deaths series:%s error:%s", row[1], err)
	}
	/*
		recovered, err := strconv.Atoi(row[8])
		if err != nil {
			return updated, 0, 0, 0, fmt.Errorf("load: error reading recovered series:%s error:%s", row[1], err)
		}
	*/
	return updated, confirmed, deaths, nil
}
