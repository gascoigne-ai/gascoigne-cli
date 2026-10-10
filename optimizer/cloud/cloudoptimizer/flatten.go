package cloudoptimizer

import "github.com/lukaszb/gascoigne/compress/jsonx"

// Cloud list responses are arrays of wide, nested, mostly identical objects.
// Flattening them to dotted keys ("Placement.AvailabilityZone") lets the
// table step hoist every leaf equal across items instead of whole subtrees.

// reshape unwraps AWS envelopes, turns AWS tag lists into maps and flattens
// items of object arrays, recursively.
func reshape(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if res, ok := v["Reservations"].([]any); ok {
			v["Instances"] = instances(res)
			delete(v, "Reservations")
		}
		for k, x := range v {
			if k == "Tags" || k == "TagList" {
				x = tagMap(x)
			}
			v[k] = reshape(x)
		}
	case []any:
		for i, x := range v {
			x = reshape(x)
			if m, ok := x.(map[string]any); ok {
				flat := map[string]any{}
				jsonx.Flatten(flat, "", m)
				x = flat
			}
			v[i] = x
		}
	}
	return v
}

// instances lists ec2 instances across reservations, which only group them
// by launch request.
func instances(reservations []any) []any {
	var out []any
	for _, r := range reservations {
		m, _ := r.(map[string]any)
		list, _ := m["Instances"].([]any)
		for _, inst := range list {
			if im, ok := inst.(map[string]any); ok {
				im["ReservationId"] = m["ReservationId"]
			}
			out = append(out, inst)
		}
	}
	return out
}

// tagMap turns [{"Key": k, "Value": v}] into {k: v}.
func tagMap(v any) any {
	list, ok := v.([]any)
	if !ok {
		return v
	}
	tags := map[string]any{}
	for _, t := range list {
		m, ok := t.(map[string]any)
		key, isStr := m["Key"].(string)
		if !ok || !isStr || len(m) != 2 {
			return v
		}
		tags[key] = m["Value"]
	}
	return tags
}
