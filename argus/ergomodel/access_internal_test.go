package ergomodel

import "testing"

func TestWriteReachesHolder(t *testing.T) {
	cases := []struct {
		why       string
		write     string
		held      string
		addressed bool
		want      bool
	}{
		{"an insert moves the map, not a value copied out of it", ".running[]", ".running[]", false, false},
		{"a holder of the map sees the insert", ".running[]", ".running", false, true},
		{"a write through a stored pointer lands in the pointee", ".entries[]*.Attrs[]", ".entries[]", false, true},
		{"a sibling field of the element is not the held slice", ".streams[]*.revived", ".streams[]*.sources", false, false},
		{"a write into the held slice is", ".streams[]*.sources[]", ".streams[]*.sources", false, true},
		{"a value sub-field never reaches a copy", ".ann.Version", ".ann", false, false},
		{"the map behind a value field does", ".ann.Attrs[]", ".ann", false, true},
		{"replacing a field is invisible to a holder of its value", ".items", ".items", false, false},
		{"and visible to a holder of its address", ".items", ".items", true, true},
		{"a new header does not move the element a pointer points at", ".items", ".items[]", true, false},
		{"an array element is part of the array value", ".arr[#]", ".arr", false, false},
		{"a field name is matched whole", ".itemsX[]", ".items", false, false},
		{"a write above the held part is not under it", ".streams[]", ".streams[]*.sources", false, false},
	}
	for _, c := range cases {
		if got := reaches(c.write, c.held, c.addressed); got != c.want {
			t.Errorf("%s: reaches(%q, %q, %v) = %v, want %v", c.why, c.write, c.held, c.addressed, got, c.want)
		}
	}
}

func TestStepsSplitAndMerge(t *testing.T) {
	steps := []struct{ path, head, rest string }{
		{".a.b", ".a", ".b"},
		{"*.a", "*", ".a"},
		{"[]*.f", "[]", "*.f"},
		{"[#].f", "[#]", ".f"},
		{".f[]", ".f", "[]"},
		{"", "", ""},
	}
	for _, s := range steps {
		head, rest := firstStep(s.path)
		if head != s.head || rest != s.rest {
			t.Errorf("firstStep(%q) = %q, %q, want %q, %q", s.path, head, rest, s.head, s.rest)
		}
	}

	common := []struct{ a, b, want string }{
		{"[]*.sources", "[]*.revived", "[]*"},
		{".f[]", ".f", ".f"},
		{".items", ".itemsX", ""},
		{"", "*.f", ""},
	}
	for _, c := range common {
		if got := commonSteps(c.a, c.b); got != c.want {
			t.Errorf("commonSteps(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}
