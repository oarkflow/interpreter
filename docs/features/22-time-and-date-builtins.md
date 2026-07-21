# 22 — Time & Date Builtins

Source: `pkg/builtins/time.go`, plus extra day/week/month helpers in
`pkg/builtins/enhancements.go`.

Timestamps are Unix seconds (`int`) unless a function name says otherwise
(`_ms` variants use milliseconds).

## Current time

```spl
print now();          // unix seconds, e.g. 1784436058
print time_ms();      // unix milliseconds
print now_iso();      // "2026-07-19T04:40:58Z"
print now_format("YYYY-MM-DD"); // "2026-07-19"
```

## Formatting & parsing

Format tokens use `YYYY MM DD HH mm ss`-style placeholders (normalized
internally to Go's reference-time layout):

```spl
print format_time(now(), "YYYY-MM-DD HH:mm:ss");
print parse_time("2024-01-15", "YYYY-MM-DD"); // 1705276800
print date_with_format(2024, 1, 15, "YYYY-MM-DD");
```

## ISO 8601 conversions

```spl
print iso_to_unix("2024-01-15T00:00:00Z");    // 1705276800
print iso_to_unix_ms("2024-01-15T00:00:00Z"); // milliseconds
print unix_to_iso(1705276800);                 // "2024-01-15T00:00:00Z"
print unix_ms_to_iso(1705276800000);
```

## Arithmetic & comparison

```spl
print time_add(1705276800, 1, "day");  // 1705363200
print time_sub(1705276800, 1, "day");
print time_diff(1705276800, 1705363200, "day"); // -1 (a - b, in the given unit)
```

Supported units generally include `"second"`, `"minute"`, `"hour"`, `"day"`,
`"week"`, `"month"`.

## Day/week/month boundaries

```spl
print start_of_day(now());
print end_of_day(now());
print start_of_week(now());
print end_of_month(now());
print add_months(now(), 1);
```

Additional boundary helpers from `enhancements.go`: `start_of_month`,
`end_of_week`, `is_weekend`, `weekday`, `month`, `year` (all take a Unix
timestamp).

## Timezones

```spl
print parse_time_tz("2024-01-15 09:00", "YYYY-MM-DD HH:mm", "America/New_York");
print format_time_tz(now(), "YYYY-MM-DD HH:mm:ss", "America/New_York");
print to_timezone(now(), "America/New_York"); // "2026-07-19T00:40:58-04:00"
```

## Integer timestamp methods

Since a Unix timestamp is just an `Integer`, the interpreter also exposes
the same operations as **dot methods** directly on integers (doc 15/19):

```spl
let ts = 1705276800;
print ts.to_iso();               // "2024-01-15T00:00:00Z"
print ts.format("YYYY-MM-DD");   // "2024-01-15"
print ts.add(1, "day");          // 1705363200
print ts.sub(1, "day");
print ts.diff(otherTs, "day");
print ts.start_of_day();
print ts.end_of_day();
print ts.start_of_week();
print ts.end_of_month();
print ts.add_months(1);
print ts.to_timezone("America/New_York");
print ts.format_tz("YYYY-MM-DD HH:mm:ss", "America/New_York");
```

## Durations

```spl
print parse_duration("1h30m");   // milliseconds
print format_duration(5400000);  // "1h30m0s"-style string
```
