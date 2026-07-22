package eval_test

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestRecordCollectionProjectionMethods(t *testing.T) {
	input := `
let users = [
  { id: "u1", name: "Ada", password: "x", profile: { email: "ada@example.com" } },
  { id: "u2", name: "Lin", password: "y", profile: { email: "lin@example.com" } }
];
[
  users.pluck("id"),
  users.pluck("id", "name"),
  users.except("password"),
  users.pluck("profile.email"),
  users.pluck(),
  users.only("id", "profile.email"),
  users.except("profile.email"),
  users[0].profile.email
];
`

	result := evalWithParserCheck(t, input, object.NewEnvironment())
	rows, ok := result.(*object.Array)
	if !ok || len(rows.Elements) != 8 {
		t.Fatalf("expected eight projection results, got %T (%s)", result, result.Inspect())
	}

	ids := rows.Elements[0].(*object.Array)
	if hashStringValue(t, ids.Elements[0].(*object.Hash), "id") != "u1" || hashStringValue(t, ids.Elements[1].(*object.Hash), "id") != "u2" {
		t.Fatalf("unexpected pluck result: %s", ids.Inspect())
	}

	selected := rows.Elements[1].(*object.Array)
	firstSelected := selected.Elements[0].(*object.Hash)
	if len(firstSelected.Pairs) != 2 || hashStringValue(t, firstSelected, "name") != "Ada" {
		t.Fatalf("unexpected multi-field pluck result: %s", selected.Inspect())
	}

	withoutPasswords := rows.Elements[2].(*object.Array)
	firstSafe := withoutPasswords.Elements[0].(*object.Hash)
	if hashHasKey(firstSafe, "password") || !hashHasKey(firstSafe, "id") {
		t.Fatalf("unexpected except result: %s", withoutPasswords.Inspect())
	}

	emails := rows.Elements[3].(*object.Array)
	emailProfile := hashValue(t, emails.Elements[1].(*object.Hash), "profile").(*object.Hash)
	if hashStringValue(t, emailProfile, "email") != "lin@example.com" {
		t.Fatalf("unexpected dotted pluck result: %s", emails.Inspect())
	}

	copy := rows.Elements[4].(*object.Array)
	if len(copy.Elements) != 2 || hashStringValue(t, copy.Elements[0].(*object.Hash), "id") != "u1" {
		t.Fatalf("unexpected zero-argument pluck result: %s", copy.Inspect())
	}

	only := rows.Elements[5].(*object.Array).Elements[0].(*object.Hash)
	profile := hashValue(t, only, "profile").(*object.Hash)
	if hashStringValue(t, profile, "email") != "ada@example.com" {
		t.Fatalf("unexpected dotted only result: %s", only.Inspect())
	}

	withoutEmail := rows.Elements[6].(*object.Array).Elements[0].(*object.Hash)
	redactedProfile := hashValue(t, withoutEmail, "profile").(*object.Hash)
	if hashHasKey(redactedProfile, "email") {
		t.Fatalf("dotted except did not remove nested field: %s", withoutEmail.Inspect())
	}
	if rows.Elements[7].(*object.String).Value != "ada@example.com" {
		t.Fatalf("dotted except mutated the source collection: %s", result.Inspect())
	}
}

func TestArrayDotNotationExceptAndZeroArgumentPluck(t *testing.T) {
	input := `
let orders = [
  { id: "A-17", total: 680 },
  { id: "B-42", total: 1400 },
  { id: "C-08", total: 2200 }
];
let large = orders.filter(o => o.total >= 1000);
let withoutIDs = large.except("id");
let copy = large.pluck();
[
  withoutIDs.length,
  withoutIDs[0].id == null,
  withoutIDs[0].total,
  copy[0].id,
  large[0].id
];
`

	result := evalWithParserCheck(t, input, object.NewEnvironment())
	values, ok := result.(*object.Array)
	if !ok || len(values.Elements) != 5 {
		t.Fatalf("expected five dot-method results, got %T (%s)", result, result.Inspect())
	}
	if values.Elements[0].(*object.Integer).Value != 2 || values.Elements[1] != object.TRUE {
		t.Fatalf("unexpected except result: %s", result.Inspect())
	}
	if values.Elements[2].(*object.Integer).Value != 1400 || values.Elements[3].(*object.String).Value != "B-42" || values.Elements[4].(*object.String).Value != "B-42" {
		t.Fatalf("unexpected pluck/source result: %s", result.Inspect())
	}
}

func TestRecordCollectionQueryAndOrganizationMethods(t *testing.T) {
	input := `
let orders = [
  { id: "A", team: "north", total: 680 },
  { id: "B", team: "south", total: 1400 },
  { id: "C", team: "north", total: 2200 },
  { id: "D", team: "south", total: 1400 }
];
[
  orders.where("team", "north").column("id"),
  orders.where_in("id", ["A", "C"]).values_of("id"),
  orders.first_where("total", 1400).id,
  orders.group_by("team").north.length,
  orders.key_by("id").C.total,
  orders.sort_by("total", "desc").first().id,
  orders.unique_by("total").length
];
`

	result := evalWithParserCheck(t, input, object.NewEnvironment())
	values, ok := result.(*object.Array)
	if !ok || len(values.Elements) != 7 {
		t.Fatalf("expected seven collection results, got %T (%s)", result, result.Inspect())
	}
	if values.Elements[2].(*object.String).Value != "B" {
		t.Fatalf("unexpected first_where result: %s", result.Inspect())
	}
	if values.Elements[3].(*object.Integer).Value != 2 || values.Elements[4].(*object.Integer).Value != 2200 {
		t.Fatalf("unexpected grouping/indexing result: %s", result.Inspect())
	}
	if values.Elements[5].(*object.String).Value != "C" || values.Elements[6].(*object.Integer).Value != 3 {
		t.Fatalf("unexpected sorting/unique result: %s", result.Inspect())
	}
}

func TestCollectionSliceAggregateAndHashSelectionMethods(t *testing.T) {
	input := `
let rows = [{ n: 1 }, { n: 2 }, null, { n: 3 }, { n: 4 }];
let user = { id: "u1", name: "Ada", password: "secret" };
[
  rows.compact().take(3).drop(1).column("n"),
  rows.compact().chunk(2).length,
  rows.compact().sum("n"),
  rows.compact().avg("n"),
  user.only("id", "name").name,
  user.except("password").password == null,
  user.password
];
`

	result := evalWithParserCheck(t, input, object.NewEnvironment())
	values, ok := result.(*object.Array)
	if !ok || len(values.Elements) != 7 {
		t.Fatalf("expected seven collection results, got %T (%s)", result, result.Inspect())
	}
	if values.Elements[1].(*object.Integer).Value != 2 || values.Elements[2].(*object.Integer).Value != 10 {
		t.Fatalf("unexpected slice/aggregate result: %s", result.Inspect())
	}
	if values.Elements[3].(*object.Float).Value != 2.5 || values.Elements[4].(*object.String).Value != "Ada" {
		t.Fatalf("unexpected average/hash selection result: %s", result.Inspect())
	}
	if values.Elements[5] != object.TRUE || values.Elements[6].(*object.String).Value != "secret" {
		t.Fatalf("hash methods mutated source or returned wrong result: %s", result.Inspect())
	}
}

func hashHasKey(hash *object.Hash, key string) bool {
	_, ok := hash.Pairs[(&object.String{Value: key}).HashKey()]
	return ok
}

func hashStringValue(t *testing.T, hash *object.Hash, key string) string {
	t.Helper()
	pair, ok := hash.Pairs[(&object.String{Value: key}).HashKey()]
	if !ok {
		t.Fatalf("missing key %q in %s", key, hash.Inspect())
	}
	value, ok := pair.Value.(*object.String)
	if !ok {
		t.Fatalf("key %q: expected string, got %T", key, pair.Value)
	}
	return value.Value
}

func hashValue(t *testing.T, hash *object.Hash, key string) object.Object {
	t.Helper()
	pair, ok := hash.Pairs[(&object.String{Value: key}).HashKey()]
	if !ok {
		t.Fatalf("missing key %q in %s", key, hash.Inspect())
	}
	return pair.Value
}
