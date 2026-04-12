package eval_test

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestStringEssentialBuiltins(t *testing.T) {
	testBooleanObject(t, testEval(`starts_with("hello world", "hello");`), true)
	testBooleanObject(t, testEval(`ends_with("hello world", "world");`), true)
	testIntegerObject(t, testEval(`index_of("hello", "ll");`), 2)
	testIntegerObject(t, testEval(`count_substr("banana", "an");`), 2)

	sub := testEval(`substring("developer", 0, 4);`)
	subStr, ok := sub.(*object.String)
	if !ok || subStr.Value != "deve" {
		t.Fatalf("unexpected substring result: %#v", sub)
	}

	slugObj := testEval(`slug("Hello, World from SPL!");`)
	if slugObj.(*object.String).Value != "hello-world-from-spl" {
		t.Fatalf("unexpected slug value: %s", slugObj.(*object.String).Value)
	}
	titleObj := testEval(`title("hello_worldFrom-spl");`)
	if titleObj.(*object.String).Value != "Hello World From Spl" {
		t.Fatalf("unexpected title value: %s", titleObj.(*object.String).Value)
	}
	if testEval(`snake_case("helloWorld SPL");`).(*object.String).Value != "hello_world_spl" {
		t.Fatalf("unexpected snake_case result")
	}
	if testEval(`kebab_case("helloWorld SPL");`).(*object.String).Value != "hello-world-spl" {
		t.Fatalf("unexpected kebab_case result")
	}
	if testEval(`camel_case("hello world_spl");`).(*object.String).Value != "helloWorldSpl" {
		t.Fatalf("unexpected camel_case result")
	}
	if testEval(`pascal_case("hello world_spl");`).(*object.String).Value != "HelloWorldSpl" {
		t.Fatalf("unexpected pascal_case result")
	}
	testBooleanObject(t, testEval(`regex_match("abc123", "^[a-z]+[0-9]+$");`), true)
	if testEval(`regex_replace("abc-123", "[^a-z]+", "_");`).(*object.String).Value != "abc_" {
		t.Fatalf("unexpected regex_replace result")
	}
	if testEval(`trim_prefix("prefix-value", "prefix-");`).(*object.String).Value != "value" {
		t.Fatalf("unexpected trim_prefix result")
	}
	if testEval(`trim_suffix("value.spl", ".spl");`).(*object.String).Value != "value" {
		t.Fatalf("unexpected trim_suffix result")
	}
	if testEval(`pad_left("7", 3, "0");`).(*object.String).Value != "007" {
		t.Fatalf("unexpected pad_left result")
	}
	if testEval(`pad_right("7", 3, "0");`).(*object.String).Value != "700" {
		t.Fatalf("unexpected pad_right result")
	}
	if testEval(`"apple".upper();`).(*object.String).Value != "APPLE" {
		t.Fatalf("unexpected string method upper result")
	}
	if testEval(`"apple".toUpperCase();`).(*object.String).Value != "APPLE" {
		t.Fatalf("unexpected string method toUpperCase result")
	}
	testBooleanObject(t, testEval(`"apple".includes("pp");`), true)
}

func TestCollectionEssentialBuiltins(t *testing.T) {
	testIntegerObject(t, testEval(`first([10, 20, 30]);`), 10)
	testIntegerObject(t, testEval(`last([10, 20, 30]);`), 30)
	testIntegerObject(t, testEval(`len(rest([10, 20, 30]));`), 2)
	testIntegerObject(t, testEval(`sum([1,2,3,4]);`), 10)
	testBooleanObject(t, testEval(`has_key({"a":1}, "a");`), true)
	testIntegerObject(t, testEval(`get({"a": 1}, "b", 99);`), 99)
	testIntegerObject(t, testEval(`clamp(20, 0, 10);`), 10)
	testBooleanObject(t, testEval(`is_even(10);`), true)
	testBooleanObject(t, testEval(`is_odd(11);`), true)
	testBooleanObject(t, testEval(`any([null, false, 1]);`), true)
	testBooleanObject(t, testEval(`all([1, true, "x"]);`), true)
}

func TestTimeEssentialBuiltins(t *testing.T) {
	testIntegerObject(t, testEval(`iso_to_unix("1970-01-01T00:00:00Z");`), 0)
	testIntegerObject(t, testEval(`iso_to_unix_ms("1970-01-01T00:00:00Z");`), 0)
	testBooleanObject(t, testEval(`now() > 0;`), true)

	if testEval(`format_time(0, "YYYY-MM-DD HH:mm:ss");`).(*object.String).Value != "1970-01-01 00:00:00" {
		t.Fatalf("unexpected format_time value")
	}
	testIntegerObject(t, testEval(`parse_time("1970-01-01 00:00:00", "YYYY-MM-DD HH:mm:ss");`), 0)
	testIntegerObject(t, testEval(`time_add(0, 1, "day");`), 86400)
	testIntegerObject(t, testEval(`time_sub(86400, 1, "day");`), 0)
	testIntegerObject(t, testEval(`time_diff(86400, 0, "day");`), 1)
	testIntegerObject(t, testEval(`start_of_day(86399);`), 0)
	testIntegerObject(t, testEval(`end_of_day(1);`), 86399)
	testIntegerObject(t, testEval(`start_of_week(0);`), -259200)
	testIntegerObject(t, testEval(`end_of_month(0);`), 2678399)
	testIntegerObject(t, testEval(`add_months(0, 1);`), 2678400)

	if testEval(`date_with_format(2026, 2, 23, "YYYY-MM-DD");`).(*object.String).Value != "2026-02-23" {
		t.Fatalf("unexpected date_with_format value")
	}
	if testEval(`format_time_tz(0, "YYYY-MM-DD HH:mm", "America/New_York");`).(*object.String).Value != "1969-12-31 19:00" {
		t.Fatalf("unexpected format_time_tz value")
	}
	testIntegerObject(t, testEval(`parse_time_tz("1969-12-31 19:00", "YYYY-MM-DD HH:mm", "America/New_York");`), 0)
}

func TestMethodAndParseBuiltins(t *testing.T) {
	testIntegerObject(t, testEval(`(-5).abs();`), 5)
	testIntegerObject(t, testEval(`(10).to_float().to_int();`), 10)
	testIntegerObject(t, testEval(`(10).pow(2);`), 100)
	testIntegerObject(t, testEval(`(10).sqrt();`), 3)
	testBooleanObject(t, testEval(`(10).is_even();`), true)
	testBooleanObject(t, testEval(`(11).is_odd();`), true)

	if testEval(`(0).to_iso();`).(*object.String).Value != "1970-01-01T00:00:00Z" {
		t.Fatalf("unexpected to_iso result")
	}
	if testEval(`(0).format("YYYY-MM-DD HH:mm:ss");`).(*object.String).Value != "1970-01-01 00:00:00" {
		t.Fatalf("unexpected format() on integer timestamp")
	}
	testIntegerObject(t, testEval(`(0).add(1, "day");`), 86400)
	testIntegerObject(t, testEval(`(86400).sub(1, "day");`), 0)
	testIntegerObject(t, testEval(`(86400).diff(0, "day");`), 1)

	if testEval(`type(parse_float("3.14"));`).(*object.String).Value != "FLOAT" {
		t.Fatalf("parse_float should return FLOAT")
	}
	testIntegerObject(t, testEval(`to_int(parse_float("3.9"));`), 3)
	testBooleanObject(t, testEval(`parse_bool("true");`), true)
	testBooleanObject(t, testEval(`parse_bool("0");`), false)
	if testEval(`parse_string(123);`).(*object.String).Value != "123" {
		t.Fatalf("parse_string failed")
	}
	testIntegerObject(t, testEval(`parse_type("12", "int");`), 12)
	if testEval(`type(parse_type("12.5", "float"));`).(*object.String).Value != "FLOAT" {
		t.Fatalf("parse_type float should return FLOAT")
	}
	testBooleanObject(t, testEval(`parse_type("yes", "bool");`), true)
	testIntegerObject(t, testEval(`parse_type("1970-01-01T00:00:00Z", "time");`), 0)
	testIntegerObject(t, testEval(`parse_type("1970-01-01 00:00:00", "time", "YYYY-MM-DD HH:mm:ss");`), 0)
}
