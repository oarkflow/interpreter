package interpreter

import (
	"fmt"
	"strings"
	"time"
)

func normalizeTimeFormat(format string) string {
	replacer := strings.NewReplacer(
		"YYYY", "2006",
		"YY", "06",
		"MM", "01",
		"DD", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)
	return replacer.Replace(format)
}

func asTimeUnitDuration(amount int64, unit string) (time.Duration, error) {
	switch strings.ToLower(unit) {
	case "ms", "millisecond", "milliseconds":
		return time.Duration(amount) * time.Millisecond, nil
	case "s", "sec", "second", "seconds":
		return time.Duration(amount) * time.Second, nil
	case "m", "min", "minute", "minutes":
		return time.Duration(amount) * time.Minute, nil
	case "h", "hour", "hours":
		return time.Duration(amount) * time.Hour, nil
	case "d", "day", "days":
		return time.Duration(amount) * 24 * time.Hour, nil
	case "w", "week", "weeks":
		return time.Duration(amount) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
}

func asTimeUnitDivisor(unit string) (int64, error) {
	switch strings.ToLower(unit) {
	case "s", "sec", "second", "seconds":
		return 1, nil
	case "m", "min", "minute", "minutes":
		return 60, nil
	case "h", "hour", "hours":
		return 3600, nil
	case "d", "day", "days":
		return 86400, nil
	case "w", "week", "weeks":
		return 604800, nil
	default:
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
}

func loadLocationOrError(name string) (*time.Location, Object) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, newError("invalid timezone %q", name)
	}
	return loc, nil
}

var timeBuiltins = map[string]*Builtin{
	"now": {
		Fn: func(args ...Object) Object {
			if len(args) != 0 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0", len(args))}
			}
			return &Integer{Value: time.Now().Unix()}
		},
	},
	"time_ms": {
		Fn: func(args ...Object) Object {
			if len(args) != 0 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0", len(args))}
			}
			return &Integer{Value: time.Now().UnixMilli()}
		},
	},
	"now_iso": {
		Fn: func(args ...Object) Object {
			if len(args) != 0 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=0", len(args))}
			}
			return &String{Value: time.Now().UTC().Format(time.RFC3339)}
		},
	},
	"now_format": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			format, errObj := asString(args[0], "format")
			if errObj != nil {
				return errObj
			}
			return &String{Value: time.Now().UTC().Format(normalizeTimeFormat(format))}
		},
	},
	"format_time": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			format, errObj := asString(args[1], "format")
			if errObj != nil {
				return errObj
			}
			return &String{Value: time.Unix(ts, 0).UTC().Format(normalizeTimeFormat(format))}
		},
	},
	"parse_time": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			input, errObj := asString(args[0], "input")
			if errObj != nil {
				return errObj
			}
			format, errObj := asString(args[1], "format")
			if errObj != nil {
				return errObj
			}
			tm, err := time.Parse(normalizeTimeFormat(format), input)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: tm.Unix()}
		},
	},
	"date_with_format": {
		Fn: func(args ...Object) Object {
			if len(args) != 4 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=4", len(args))}
			}
			year, errObj := asInt(args[0], "year")
			if errObj != nil {
				return errObj
			}
			month, errObj := asInt(args[1], "month")
			if errObj != nil {
				return errObj
			}
			day, errObj := asInt(args[2], "day")
			if errObj != nil {
				return errObj
			}
			format, errObj := asString(args[3], "format")
			if errObj != nil {
				return errObj
			}
			tm := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
			return &String{Value: tm.Format(normalizeTimeFormat(format))}
		},
	},
	"time_add": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			amount, errObj := asInt(args[1], "amount")
			if errObj != nil {
				return errObj
			}
			unit, errObj := asString(args[2], "unit")
			if errObj != nil {
				return errObj
			}
			d, err := asTimeUnitDuration(amount, unit)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: time.Unix(ts, 0).Add(d).Unix()}
		},
	},
	"time_sub": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			amount, errObj := asInt(args[1], "amount")
			if errObj != nil {
				return errObj
			}
			unit, errObj := asString(args[2], "unit")
			if errObj != nil {
				return errObj
			}
			d, err := asTimeUnitDuration(amount, unit)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: time.Unix(ts, 0).Add(-d).Unix()}
		},
	},
	"time_diff": {
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
			}
			left, errObj := asInt(args[0], "left_unix")
			if errObj != nil {
				return errObj
			}
			right, errObj := asInt(args[1], "right_unix")
			if errObj != nil {
				return errObj
			}
			diff := left - right
			if len(args) == 2 {
				return &Integer{Value: diff}
			}
			unit, errObj := asString(args[2], "unit")
			if errObj != nil {
				return errObj
			}
			divisor, err := asTimeUnitDivisor(unit)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: diff / divisor}
		},
	},
	"start_of_day": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			tm := time.Unix(ts, 0).UTC()
			return &Integer{Value: time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, time.UTC).Unix()}
		},
	},
	"end_of_day": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			tm := time.Unix(ts, 0).UTC()
			nextDay := time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
			return &Integer{Value: nextDay.Unix() - 1}
		},
	},
	"unix_to_iso": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			return &String{Value: time.Unix(ts, 0).UTC().Format(time.RFC3339)}
		},
	},
	"unix_ms_to_iso": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_ms")
			if errObj != nil {
				return errObj
			}
			return &String{Value: time.UnixMilli(ts).UTC().Format(time.RFC3339Nano)}
		},
	},
	"iso_to_unix": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "iso")
			if errObj != nil {
				return errObj
			}
			tm, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: tm.Unix()}
		},
	},
	"iso_to_unix_ms": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			s, errObj := asString(args[0], "iso")
			if errObj != nil {
				return errObj
			}
			tm, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: tm.UnixMilli()}
		},
	},
	"parse_time_tz": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
			}
			input, errObj := asString(args[0], "input")
			if errObj != nil {
				return errObj
			}
			format, errObj := asString(args[1], "format")
			if errObj != nil {
				return errObj
			}
			tz, errObj := asString(args[2], "timezone")
			if errObj != nil {
				return errObj
			}
			loc, locErr := loadLocationOrError(tz)
			if locErr != nil {
				return locErr
			}
			tm, err := time.ParseInLocation(normalizeTimeFormat(format), input, loc)
			if err != nil {
				return newError("%s", err)
			}
			return &Integer{Value: tm.Unix()}
		},
	},
	"format_time_tz": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			format, errObj := asString(args[1], "format")
			if errObj != nil {
				return errObj
			}
			tz, errObj := asString(args[2], "timezone")
			if errObj != nil {
				return errObj
			}
			loc, locErr := loadLocationOrError(tz)
			if locErr != nil {
				return locErr
			}
			return &String{Value: time.Unix(ts, 0).In(loc).Format(normalizeTimeFormat(format))}
		},
	},
	"to_timezone": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			tz, errObj := asString(args[1], "timezone")
			if errObj != nil {
				return errObj
			}
			loc, locErr := loadLocationOrError(tz)
			if locErr != nil {
				return locErr
			}
			return &String{Value: time.Unix(ts, 0).In(loc).Format(time.RFC3339)}
		},
	},
	"start_of_week": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			tm := time.Unix(ts, 0).UTC()
			weekday := int(tm.Weekday())
			if weekday == 0 {
				weekday = 7 // ISO week: Sunday as 7
			}
			start := time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
			return &Integer{Value: start.Unix()}
		},
	},
	"end_of_month": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			tm := time.Unix(ts, 0).UTC()
			firstNextMonth := time.Date(tm.Year(), tm.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			return &Integer{Value: firstNextMonth.Unix() - 1}
		},
	},
	"add_months": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			ts, errObj := asInt(args[0], "unix_seconds")
			if errObj != nil {
				return errObj
			}
			months, errObj := asInt(args[1], "months")
			if errObj != nil {
				return errObj
			}
			return &Integer{Value: time.Unix(ts, 0).UTC().AddDate(0, int(months), 0).Unix()}
		},
	},
}

func init() {
	registerBuiltins(timeBuiltins)
}
