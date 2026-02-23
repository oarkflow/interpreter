package interpreter

import "testing"

func TestStringEssentialBuiltins(t *testing.T) {
	testBooleanObject(t, testEval(`starts_with("hello world", "hello");`), true)
	testBooleanObject(t, testEval(`ends_with("hello world", "world");`), true)
	testIntegerObject(t, testEval(`index_of("hello", "ll");`), 2)
	testIntegerObject(t, testEval(`count_substr("banana", "an");`), 2)

	sub := testEval(`substring("developer", 0, 4);`)
	subStr, ok := sub.(*String)
	if !ok || subStr.Value != "deve" {
		t.Fatalf("unexpected substring result: %#v", sub)
	}

	slugObj := testEval(`slug("Hello, World from SPL!");`)
	if slugObj.(*String).Value != "hello-world-from-spl" {
		t.Fatalf("unexpected slug value: %s", slugObj.(*String).Value)
	}
	titleObj := testEval(`title("hello_worldFrom-spl");`)
	if titleObj.(*String).Value != "Hello World From Spl" {
		t.Fatalf("unexpected title value: %s", titleObj.(*String).Value)
	}
	if testEval(`snake_case("helloWorld SPL");`).(*String).Value != "hello_world_spl" {
		t.Fatalf("unexpected snake_case result")
	}
	if testEval(`kebab_case("helloWorld SPL");`).(*String).Value != "hello-world-spl" {
		t.Fatalf("unexpected kebab_case result")
	}
	if testEval(`camel_case("hello world_spl");`).(*String).Value != "helloWorldSpl" {
		t.Fatalf("unexpected camel_case result")
	}
	if testEval(`pascal_case("hello world_spl");`).(*String).Value != "HelloWorldSpl" {
		t.Fatalf("unexpected pascal_case result")
	}
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
}

func TestTimeEssentialBuiltins(t *testing.T) {
	testIntegerObject(t, testEval(`iso_to_unix("1970-01-01T00:00:00Z");`), 0)
	testIntegerObject(t, testEval(`iso_to_unix_ms("1970-01-01T00:00:00Z");`), 0)
	testBooleanObject(t, testEval(`now() > 0;`), true)

	if testEval(`format_time(0, "YYYY-MM-DD HH:mm:ss");`).(*String).Value != "1970-01-01 00:00:00" {
		t.Fatalf("unexpected format_time value")
	}
	testIntegerObject(t, testEval(`parse_time("1970-01-01 00:00:00", "YYYY-MM-DD HH:mm:ss");`), 0)
	testIntegerObject(t, testEval(`time_add(0, 1, "day");`), 86400)
	testIntegerObject(t, testEval(`time_sub(86400, 1, "day");`), 0)
	testIntegerObject(t, testEval(`time_diff(86400, 0, "day");`), 1)
	testIntegerObject(t, testEval(`start_of_day(86399);`), 0)
	testIntegerObject(t, testEval(`end_of_day(1);`), 86399)

	if testEval(`date_with_format(2026, 2, 23, "YYYY-MM-DD");`).(*String).Value != "2026-02-23" {
		t.Fatalf("unexpected date_with_format value")
	}
}
