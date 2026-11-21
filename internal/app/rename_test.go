package app

import "testing"

func TestRename(t *testing.T) {
	a, _ := three(t)

	if err := a.Rename("work", "home"); err != nil {
		t.Fatal(err)
	}
	st := load(t, a)
	if st.Logins["a1"].Name != "home" || st.Default != "a1" {
		t.Errorf("after rename: %+v, default %q", st.Logins["a1"], st.Default)
	}

	cases := []struct{ old, name string }{
		{"home", "ci"},    // taken
		{"home", "-x"},    // not a name
		{"nope", "other"}, // no such login
	}
	for _, tc := range cases {
		if err := a.Rename(tc.old, tc.name); err == nil {
			t.Errorf("rename %s %s: want an error", tc.old, tc.name)
		}
	}
}
