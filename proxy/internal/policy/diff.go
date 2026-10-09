package policy

import (
	"reflect"
	"sort"
	"strings"
)

// Change is one difference between two policies. It names where the
// difference is and never what the value was or became: a path goes into the
// audit log and into error bodies, and a policy holds webhook URLs and the
// text of patterns.
type Change struct {
	// Section is the top-level key of the policy, as written in YAML.
	Section string
	// Path is the section, and below it the key that differs where one can
	// be named: "providers.allowed", "rules.pii_detection.action",
	// "custom_entities.EMPLOYEE_ID".
	Path string
	// Privileged is true when the change needs the admin role.
	Privileged bool
}

// analystSections are the top-level keys the analyst role may change. Every
// other key of Policy needs the admin role, including one added to the
// struct after this list was written: a section nobody classified is
// privileged. Inside "rules" one key is privileged all the same, see
// privilegedRuleKey.
//
// name and version label the policy and decide nothing; they are open so
// that an analyst can number an edit.
var analystSections = map[string]bool{
	"name":            true,
	"version":         true,
	"rules":           true,
	"output_rules":    true,
	"custom_entities": true,
	"competitors":     true,
}

// privilegedRuleKey is the one key of a rule the analyst role may not
// change: where a finding is reported to. Pointing it elsewhere, or
// removing it, silences an alert or sends it to a host of the writer's
// choosing.
const privilegedRuleKey = "notify"

func yamlKey(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
	if name == "" || name == "-" {
		return strings.ToLower(f.Name)
	}
	return name
}

// sameValue compares two decoded values. A list or map that is absent and
// one that is empty are the same; everything else must match exactly, an
// absent section and an empty one included.
func sameValue(a, b reflect.Value) bool {
	if a.Kind() != b.Kind() {
		return false
	}
	switch a.Kind() {
	case reflect.Ptr, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() && b.IsNil()
		}
		return sameValue(a.Elem(), b.Elem())
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !sameValue(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if a.Len() != b.Len() {
			return false
		}
		for _, k := range a.MapKeys() {
			bv := b.MapIndex(k)
			if !bv.IsValid() || !sameValue(a.MapIndex(k), bv) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !sameValue(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a.Interface(), b.Interface())
}

// structKeyDiff returns the yaml keys of the fields that differ between two
// structs of the same type.
func structKeyDiff(a, b reflect.Value) []string {
	var keys []string
	for i := 0; i < a.NumField(); i++ {
		if !sameValue(a.Field(i), b.Field(i)) {
			keys = append(keys, yamlKey(a.Type().Field(i)))
		}
	}
	return keys
}

// Diff lists what differs between two policies, comparing the decoded
// values. The order of keys, comments and quoting of the YAML they came from
// are not part of a policy and are not differences. A nil policy is an
// empty one.
func Diff(old, new *Policy) []Change {
	if old == nil {
		old = &Policy{}
	}
	if new == nil {
		new = &Policy{}
	}
	var out []Change
	ov, nv := reflect.ValueOf(*old), reflect.ValueOf(*new)
	for i := 0; i < ov.NumField(); i++ {
		section := yamlKey(ov.Type().Field(i))
		a, b := ov.Field(i), nv.Field(i)
		if sameValue(a, b) {
			continue
		}
		privileged := !analystSections[section]
		add := func(path string, priv bool) {
			out = append(out, Change{Section: section, Path: path, Privileged: priv})
		}
		switch {
		case section == "rules":
			out = append(out, ruleChanges(old.Rules, new.Rules)...)
		case a.Kind() == reflect.Ptr && a.Type().Elem().Kind() == reflect.Struct:
			// An absent section is compared as an empty one to name the
			// keys; absent against empty differs in no key and is reported
			// as the section itself.
			zero := reflect.Zero(a.Type().Elem())
			av, bv := zero, zero
			if !a.IsNil() {
				av = a.Elem()
			}
			if !b.IsNil() {
				bv = b.Elem()
			}
			keys := structKeyDiff(av, bv)
			if len(keys) == 0 {
				add(section, privileged)
			}
			for _, k := range keys {
				add(section+"."+k, privileged)
			}
		case a.Kind() == reflect.Slice && a.Type().Elem().Kind() == reflect.Struct:
			names, ok := namedListDiff(a, b)
			if !ok || len(names) == 0 {
				add(section, privileged)
			}
			for _, n := range names {
				add(section+"."+n, privileged)
			}
		default:
			add(section, privileged)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ruleChanges compares the rules by name and, for a rule on both sides, key
// by key. A rule that comes or goes with a notify list changes that list.
func ruleChanges(old, new map[string]Rule) []Change {
	names := map[string]struct{}{}
	for k := range old {
		names[k] = struct{}{}
	}
	for k := range new {
		names[k] = struct{}{}
	}
	var out []Change
	for name := range names {
		path := "rules." + name
		o, inOld := old[name]
		n, inNew := new[name]
		if inOld && inNew {
			for _, k := range structKeyDiff(reflect.ValueOf(o), reflect.ValueOf(n)) {
				out = append(out, Change{Section: "rules", Path: path + "." + k, Privileged: k == privilegedRuleKey})
			}
			continue
		}
		out = append(out, Change{Section: "rules", Path: path})
		if len(o.Notify) > 0 || len(n.Notify) > 0 {
			out = append(out, Change{Section: "rules", Path: path + "." + privilegedRuleKey, Privileged: true})
		}
	}
	return out
}

// namedListDiff compares two lists of structs that carry a Name field and
// returns the names added, removed or changed. ok is false when the list
// cannot be compared by name: no such field, or a name used twice. An empty
// result with ok true means the same entries in another order.
func namedListDiff(a, b reflect.Value) (names []string, ok bool) {
	index := func(v reflect.Value) (map[string]reflect.Value, bool) {
		m := make(map[string]reflect.Value, v.Len())
		for i := 0; i < v.Len(); i++ {
			f := v.Index(i).FieldByName("Name")
			if !f.IsValid() || f.Kind() != reflect.String {
				return nil, false
			}
			if _, dup := m[f.String()]; dup {
				return nil, false
			}
			m[f.String()] = v.Index(i)
		}
		return m, true
	}
	am, aok := index(a)
	bm, bok := index(b)
	if !aok || !bok {
		return nil, false
	}
	for name, av := range am {
		if bv, found := bm[name]; !found || !sameValue(av, bv) {
			names = append(names, name)
		}
	}
	for name := range bm {
		if _, found := am[name]; !found {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, true
}

// PrivilegedChanges names what differs between two policies that only the
// admin role may change: the privileged top-level sections, and
// "rules.<name>.notify" for a rule whose notify list differs. Empty means
// the analyst role may make the change.
func PrivilegedChanges(old, new *Policy) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, c := range Diff(old, new) {
		if !c.Privileged {
			continue
		}
		name := c.Section
		if c.Section == "rules" {
			name = c.Path
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ChangedPaths returns the paths of a diff, for the audit log.
func ChangedPaths(changes []Change) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Path)
	}
	return out
}

// ChangedSections returns the top-level sections a diff touches, sorted.
func ChangedSections(changes []Change) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, c := range changes {
		if _, dup := seen[c.Section]; !dup {
			seen[c.Section] = struct{}{}
			out = append(out, c.Section)
		}
	}
	sort.Strings(out)
	return out
}

// ChangedRules returns the names of the rules a diff adds, removes or
// changes, sorted.
func ChangedRules(changes []Change) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, c := range changes {
		if c.Section != "rules" {
			continue
		}
		name := strings.TrimPrefix(c.Path, "rules.")
		// A rule's own path has no key after the name; a key's path has one.
		// Rule names may hold dots, so the key is cut from the right.
		for _, k := range ruleKeys {
			if strings.HasSuffix(name, "."+k) {
				name = strings.TrimSuffix(name, "."+k)
				break
			}
		}
		if _, dup := seen[name]; !dup {
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// ruleKeys are the yaml keys of Rule.
var ruleKeys = func() []string {
	t := reflect.TypeOf(Rule{})
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		keys = append(keys, yamlKey(t.Field(i)))
	}
	return keys
}()
