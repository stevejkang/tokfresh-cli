package schedule

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ActiveTriggerCount is the number of cron triggers actually scheduled.
// The 5th slot is calculated but not deployed (overnight gap).
const ActiveTriggerCount = 4

// Calculate returns 5 time slots at 5-hour intervals starting from startTime.
// startTime must be in "HH:MM" 24-hour format.
func Calculate(startTime string) []string {
	h, m := parseHHMM(startTime)
	schedule := make([]string, 5)

	for i := 0; i < 5; i++ {
		totalMinutes := (h*60 + m + i*300) % 1440
		sh := totalMinutes / 60
		sm := totalMinutes % 60
		schedule[i] = fmt.Sprintf("%02d:%02d", sh, sm)
	}

	return schedule
}

// GetResetTime returns the time 5 hours after triggerTime.
// triggerTime must be in "HH:MM" 24-hour format.
func GetResetTime(triggerTime string) string {
	h, m := parseHHMM(triggerTime)
	totalMinutes := (h*60 + m + 300) % 1440
	rh := totalMinutes / 60
	rm := totalMinutes % 60
	return fmt.Sprintf("%02d:%02d", rh, rm)
}

// ToCron converts local times + timezone to a UTC cron expression.
// Only the first ActiveTriggerCount slots are included.
// Returns format: "M H1,H2,H3,H4 * * *"
//
// Known limitation: uses a single UTC offset snapshot from time.Now().
// For DST-observing timezones, the baked-in offset becomes stale after
// a DST transition (±1 hour). Cloudflare Workers cron API does not support
// CRON_TZ, so there is no server-side fix. Users should run `tokfresh update`
// after a DST change, or choose a non-DST timezone (e.g., UTC).
func ToCron(times []string, timezone string) string {
	active := times
	if len(active) > ActiveTriggerCount {
		active = active[:ActiveTriggerCount]
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Fallback: treat as UTC
		loc = time.UTC
	}

	// Calculate offset from timezone. Use a reference time to get the offset.
	// We use the current time to reflect DST if applicable.
	now := time.Now().In(loc)
	_, offsetSec := now.Zone()
	offsetMinutes := offsetSec / 60

	// Convert each local time to UTC
	utcHours := make([]int, len(active))
	var utcMinute int

	for i, t := range active {
		h, m := parseHHMM(t)
		localMinutes := h*60 + m
		utcTotal := ((localMinutes-offsetMinutes)%1440 + 1440) % 1440
		utcHours[i] = utcTotal / 60
		if i == 0 {
			utcMinute = utcTotal % 60
		}
	}

	hourStrs := make([]string, len(utcHours))
	for i, h := range utcHours {
		hourStrs[i] = strconv.Itoa(h)
	}

	return fmt.Sprintf("%d %s * * *", utcMinute, strings.Join(hourStrs, ","))
}

// GetNextTrigger returns the next upcoming trigger as a time.Time and a human-readable label.
// Uses only the first ActiveTriggerCount slots.
func GetNextTrigger(schedule []string, timezone string) (nextTime time.Time, label string) {
	active := schedule
	if len(active) > ActiveTriggerCount {
		active = active[:ActiveTriggerCount]
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	now := time.Now().In(loc)
	tzAbbr := getTimezoneAbbr(now)

	sorted := make([]string, len(active))
	copy(sorted, active)
	sort.Slice(sorted, func(i, j int) bool {
		hi, mi := parseHHMM(sorted[i])
		hj, mj := parseHHMM(sorted[j])
		return hi*60+mi < hj*60+mj
	})

	for _, t := range sorted {
		h, m := parseHHMM(t)
		candidate := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
		if candidate.After(now) {
			return candidate, fmt.Sprintf("Today %s %s", t, tzAbbr)
		}
	}

	h, m := parseHHMM(sorted[0])
	tomorrow := now.AddDate(0, 0, 1)
	candidate := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), h, m, 0, 0, loc)
	return candidate, fmt.Sprintf("Tomorrow %s %s", sorted[0], tzAbbr)
}

func DetectTimezone() string {
	loc := time.Now().Location()
	if loc != nil && loc.String() != "Local" {
		return loc.String()
	}

	if tz := os.Getenv("TZ"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}

	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if idx := strings.Index(target, "zoneinfo/"); idx >= 0 {
			iana := target[idx+len("zoneinfo/"):]
			if _, err := time.LoadLocation(iana); err == nil {
				return iana
			}
		}
	}

	_, offset := time.Now().Zone()
	return offsetToIANA(offset)
}

func offsetToIANA(offsetSec int) string {
	offsetMin := offsetSec / 60
	m := map[int]string{
		-660: "Pacific/Pago_Pago",
		-600: "Pacific/Honolulu",
		-570: "Pacific/Marquesas",
		-540: "America/Anchorage",
		-480: "America/Los_Angeles",
		-420: "America/Denver",
		-360: "America/Chicago",
		-300: "America/New_York",
		-240: "America/Caracas",
		-210: "America/St_Johns",
		-180: "America/Sao_Paulo",
		-120: "Atlantic/South_Georgia",
		-60:  "Atlantic/Azores",
		0:    "UTC",
		60:   "Europe/Paris",
		120:  "Africa/Cairo",
		180:  "Europe/Moscow",
		210:  "Asia/Tehran",
		240:  "Asia/Dubai",
		270:  "Asia/Kabul",
		300:  "Asia/Karachi",
		330:  "Asia/Kolkata",
		345:  "Asia/Kathmandu",
		360:  "Asia/Dhaka",
		390:  "Asia/Yangon",
		420:  "Asia/Bangkok",
		480:  "Asia/Singapore",
		540:  "Asia/Seoul",
		570:  "Australia/Darwin",
		600:  "Australia/Sydney",
		660:  "Pacific/Noumea",
		720:  "Pacific/Auckland",
		780:  "Pacific/Tongatapu",
		840:  "Pacific/Kiritimati",
	}
	if iana, ok := m[offsetMin]; ok {
		return iana
	}
	return "UTC"
}

// parseHHMM splits "HH:MM" into hours and minutes.
func parseHHMM(t string) (int, int) {
	parts := strings.SplitN(t, ":", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h, m
}

// getTimezoneAbbr returns the timezone abbreviation (e.g., "KST", "PST").
func getTimezoneAbbr(t time.Time) string {
	abbr, _ := t.Zone()
	if abbr != "" {
		return abbr
	}
	return t.Location().String()
}
