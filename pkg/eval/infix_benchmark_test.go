package eval

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

var membershipBenchmarkSink object.Object

func BenchmarkMembershipExpressionArrayFirstHit(b *testing.B) {
	needle := &object.String{Value: "finance"}
	container := &object.Array{Elements: []object.Object{
		needle,
		&object.String{Value: "procurement"},
	}}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		membershipBenchmarkSink = evalMembershipExpression(needle, container)
	}
}

func BenchmarkMembershipExpressionStringHit(b *testing.B) {
	needle := &object.String{Value: "finance"}
	container := &object.String{Value: "finance procurement"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		membershipBenchmarkSink = evalMembershipExpression(needle, container)
	}
}
