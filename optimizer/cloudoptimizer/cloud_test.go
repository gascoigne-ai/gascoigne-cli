package cloudoptimizer

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

func TestPruneDropsNoise(t *testing.T) {
	values, err := jsonx.Decode(`{"ResponseMetadata": {"RequestId": "x"}, "NextToken": "t",
		"items": [{"kind": "compute#instance", "provisioningState": "Succeeded", "name": "a"},
		          {"provisioningState": "Failed", "etag": "\"1\"", "name": "b"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	got := prune(values[0])
	want := map[string]any{"items": []any{map[string]any{"name": "a"}, map[string]any{"provisioningState": "Failed", "name": "b"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestReshapeFlattensInstances(t *testing.T) {
	values, err := jsonx.Decode(`{"Reservations": [{"ReservationId": "r-1", "Instances": [
		{"InstanceId": "i-1", "Placement": {"AvailabilityZone": "eu-west-1a"},
		 "Tags": [{"Key": "Name", "Value": "web"}], "NetworkInterfaces": [{"Groups": [{"GroupId": "sg-1"}]}]}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	got := reshape(values[0])
	want := map[string]any{"Instances": []any{map[string]any{
		"InstanceId": "i-1", "ReservationId": "r-1", "Placement.AvailabilityZone": "eu-west-1a",
		"Tags.Name": "web", "NetworkInterfaces[0].Groups[0].GroupId": "sg-1",
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestS3Summary(t *testing.T) {
	var in []string
	for i := range 40 {
		in = append(in, fmt.Sprintf("2026-09-%02d 10:00:00 %10d logs/app-%02d.json.gz", i%28+1, 1000+i, i))
	}
	in = append(in, "2026-09-30 11:00:00   90000000 exports/orders.parquet")
	o := &Optimizer{cfg: &Config{MaxObjects: 30}}
	got := compress.Join(o.s3(in))
	for _, want := range []string{"41 objects", "logs/*.json.gz ×40", "exports/*.parquet ×1", "newest:\n  2026-09-30 11:00:00 exports/orders.parquet"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
