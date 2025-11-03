package carbon

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrInvalidNumber = errors.New("invalid number in interval definition")
	ErrInvalidUnit   = errors.New("invalid unit in interval definition")
)

// Constants matching PHP Carbon's CarbonInterface.
const (
	MonthsPerQuarter           = 3
	DaysPerWeek                = 7
	HoursPerDay                = 24
	MinutesPerHour             = 60
	SecondsPerMinute           = 60
	MillisecondsPerSecond      = 1000
	MicrosecondsPerMillisecond = 1000
)

// ParseInterval parses a string interval definition like "1 year 2 months 3 days"
// This is a port of PHP Carbon's fromString method
// Note: Years and months are approximated (365 days/year, 30 days/month).
//
//nolint:gocognit,funlen
func ParseInterval(intervalDefinition string) (time.Duration, error) {
	if intervalDefinition == "" {
		return 0, nil
	}

	// Track accumulated duration components
	var (
		years        int
		months       int
		weeks        int
		days         int
		hours        int
		minutes      int
		seconds      int
		microseconds int
	)

	// Pattern to match number (with optional decimal) followed by unit
	// \d+(?:\.\d+)? matches integer or decimal number
	// \s* matches optional whitespace
	// ([^\d\s]*) captures the unit
	pattern := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*([^\d\s]*)`)
	matches := pattern.FindAllStringSubmatch(intervalDefinition, -1)

	// Track parts to process (for handling fractional values)
	type part struct {
		value float64
		unit  string
	}
	parts := make([]part, 0)

	// Convert matches to parts
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			return 0, errors.WithMessagef(ErrInvalidNumber, "failed to parse value '%s'", match[1])
		}

		parts = append(parts, part{value: value, unit: match[2]})
	}

	// Process all parts
	for len(parts) > 0 {
		p := parts[0]
		parts = parts[1:]

		intValue := int(p.value)
		fraction := p.value - float64(intValue)

		// Fix calculation precision
		roundedFraction := roundFloat(fraction, 6)
		switch roundedFraction {
		case 1:
			fraction = 0
			intValue++
		case 0:
			fraction = 0
		}

		// Handle different units
		unit := strings.ToLower(p.unit)
		//nolint:goconst
		if p.unit == "µs" {
			unit = "µs"
		}

		switch unit {
		case "year", "years", "y", "yr", "yrs":
			years += intValue

		case "quarter", "quarters":
			months += intValue * MonthsPerQuarter

		case "month", "months", "mo", "mos":
			months += intValue

		case "week", "weeks", "w":
			weeks += intValue
			if fraction > 0 {
				parts = append(parts, part{
					value: fraction * DaysPerWeek,
					unit:  "d",
				})
			}

		case "day", "days", "d":
			days += intValue
			if fraction > 0 {
				parts = append(parts, part{
					value: fraction * HoursPerDay,
					unit:  "h",
				})
			}

		case "hour", "hours", "h":
			hours += intValue
			if fraction > 0 {
				parts = append(parts, part{
					value: fraction * MinutesPerHour,
					unit:  "m",
				})
			}

		case "minute", "minutes", "m":
			minutes += intValue
			if fraction > 0 {
				parts = append(parts, part{
					value: fraction * SecondsPerMinute,
					unit:  "s",
				})
			}

		case "second", "seconds", "s":
			seconds += intValue
			if fraction > 0 {
				parts = append(parts, part{
					value: fraction * MillisecondsPerSecond,
					unit:  "ms",
				})
			}

		case "millisecond", "milliseconds", "milli", "ms":
			// Convert milliseconds to microseconds
			microseconds += intValue * MicrosecondsPerMillisecond
			if fraction > 0 {
				microseconds += int(roundFloat(fraction*MicrosecondsPerMillisecond, 0))
			}

		case "microsecond", "microseconds", "micro", "µs":
			microseconds += intValue

		default:
			return 0, errors.WithMessagef(ErrInvalidUnit, "failed to parse unit '%s' in '%s'", p.unit, intervalDefinition)
		}
	}

	// Convert all components to microseconds for precision
	totalMicros := int64(0)

	// Approximate conversions (using average days per year/month)
	totalMicros += int64(years) * 365 * 24 * 60 * 60 * 1000000
	totalMicros += int64(months) * 30 * 24 * 60 * 60 * 1000000
	totalMicros += int64(weeks) * 7 * 24 * 60 * 60 * 1000000
	totalMicros += int64(days) * 24 * 60 * 60 * 1000000
	totalMicros += int64(hours) * 60 * 60 * 1000000
	totalMicros += int64(minutes) * 60 * 1000000
	totalMicros += int64(seconds) * 1000000
	totalMicros += int64(microseconds)

	return time.Duration(totalMicros) * time.Microsecond, nil
}

// Humanize converts a time.Duration to a human-readable string
// Examples: "1 second", "2 minutes", "3 hours", "1 day", "2 weeks", "1 year".
//
//nolint:nestif
func Humanize(d time.Duration) string {
	if d == 0 {
		return "0 seconds"
	}

	// Handle negative durations
	negative := d < 0
	if negative {
		d = -d
	}

	// Convert to total seconds for easier calculation
	totalSeconds := int64(d.Seconds())

	// Define time units in seconds
	const (
		secondsPerMinute = 60
		secondsPerHour   = 60 * 60
		secondsPerDay    = 24 * 60 * 60
		secondsPerWeek   = 7 * 24 * 60 * 60
		secondsPerMonth  = 30 * 24 * 60 * 60  // Approximation
		secondsPerYear   = 365 * 24 * 60 * 60 // Approximation
	)

	// Determine the appropriate unit and value
	var value int64
	var unit string

	switch {
	case totalSeconds >= secondsPerYear:
		value = totalSeconds / secondsPerYear
		unit = "year"
	case totalSeconds >= secondsPerMonth:
		value = totalSeconds / secondsPerMonth
		unit = "month"
	case totalSeconds >= secondsPerWeek:
		value = totalSeconds / secondsPerWeek
		unit = "week"
	case totalSeconds >= secondsPerDay:
		value = totalSeconds / secondsPerDay
		unit = "day"
	case totalSeconds >= secondsPerHour:
		value = totalSeconds / secondsPerHour
		unit = "hour"
	case totalSeconds >= secondsPerMinute:
		value = totalSeconds / secondsPerMinute
		unit = "minute"
	default:
		// Handle sub-second durations
		if d < time.Millisecond {
			microseconds := d.Microseconds()
			if microseconds == 1 {
				unit = "microsecond"
			} else {
				unit = "microseconds"
			}
			if negative {
				return fmt.Sprintf("-%d %s", microseconds, unit)
			}

			return fmt.Sprintf("%d %s", microseconds, unit)
		} else if d < time.Second {
			milliseconds := d.Milliseconds()
			if milliseconds == 1 {
				unit = "millisecond"
			} else {
				unit = "milliseconds"
			}

			if negative {
				return fmt.Sprintf("-%d %s", milliseconds, unit)
			}

			return fmt.Sprintf("%d %s", milliseconds, unit)
		}

		value = totalSeconds
		unit = "second"
	}

	// Pluralize the unit if needed
	if value != 1 {
		unit += "s"
	}

	// Format the result
	if negative {
		return fmt.Sprintf("-%d %s", value, unit)
	}

	return fmt.Sprintf("%d %s", value, unit)
}

// roundFloat rounds a float to the specified number of decimal places.
func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))

	return float64(int(val*ratio+0.5)) / ratio
}
