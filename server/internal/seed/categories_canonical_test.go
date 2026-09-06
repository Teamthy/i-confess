package seed

import (
	"strings"
	"testing"
)

// The directive fixes the category list at 39 and names them. This list is
// copied from the directive so the code and the product document cannot drift
// apart silently: if either side changes, this test names the difference.
//
// It is intentionally a duplicate of CanonicalCategories' names rather than a
// derivation of them. A test that derives its expectation from the thing it
// checks proves nothing.
var directiveCategoryNames = []string{
	"Healing", "Health", "Finance", "Wealth", "Breakthrough",
	"Marriage", "Relationships", "Faith", "Peace", "Family",
	"Purpose", "Identity", "Wisdom", "Protection", "Career",
	"Business", "Leadership", "Favor", "Provision", "Confidence",
	"Discipline", "Joy", "Hope", "Freedom", "Spiritual Growth",
	"Prayer", "Children", "Parenting", "Direction", "Creativity",
	"Productivity", "Emotional Strength", "Rest", "Gratitude", "Forgiveness",
	"Love", "Overcoming Fear", "Success", "Destiny",
}

func TestCanonicalCategoryCountIsTheAgreedNumber(t *testing.T) {
	if got := len(CanonicalCategories); got != CanonicalCategoryCount {
		t.Errorf("CanonicalCategories has %d entries; the agreed count is %d", got, CanonicalCategoryCount)
	}
	if got := len(directiveCategoryNames); got != CanonicalCategoryCount {
		t.Fatalf("the directive list in this test has %d names, expected %d - update both together", got, CanonicalCategoryCount)
	}
}

func TestCanonicalCategoriesMatchTheDirectiveExactly(t *testing.T) {
	want := make(map[string]bool, len(directiveCategoryNames))
	for _, n := range directiveCategoryNames {
		want[n] = true
	}
	got := make(map[string]bool, len(CanonicalCategories))
	for _, c := range CanonicalCategories {
		got[c.Name] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("category %q is in the directive but missing from CanonicalCategories", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("category %q is in CanonicalCategories but not in the directive - product decision required", name)
		}
	}
}

func TestCanonicalCategoriesAreWellFormed(t *testing.T) {
	names := map[string]bool{}
	slugs := map[string]bool{}

	for i, c := range CanonicalCategories {
		ctx := "CanonicalCategories[" + c.Name + "]"
		if c.Name == "" {
			t.Errorf("index %d has an empty Name", i)
		}
		if c.Slug == "" {
			t.Errorf("%s has an empty Slug", ctx)
		}
		if c.Description == "" {
			t.Errorf("%s has an empty Description; the category detail screen would render blank", ctx)
		}
		if c.Tagline == "" {
			t.Errorf("%s has an empty Tagline; the website category rail would render a broken card", ctx)
		}
		if c.Icon == "" {
			t.Errorf("%s has an empty Icon", ctx)
		}
		if names[c.Name] {
			t.Errorf("%s: duplicate Name", ctx)
		}
		if slugs[c.Slug] {
			t.Errorf("%s: duplicate Slug %q", ctx, c.Slug)
		}
		names[c.Name] = true
		slugs[c.Slug] = true

		// Slug must be the kebab-case of Name so URLs are predictable and so a
		// rename cannot quietly change a category's URL.
		if want := kebab(c.Name); c.Slug != want {
			t.Errorf("%s: Slug is %q but Name implies %q", ctx, c.Slug, want)
		}
		if c.Icon != c.Slug {
			t.Errorf("%s: Icon %q does not match Slug %q; icons are keyed by slug", ctx, c.Icon, c.Slug)
		}
	}
}

// TestCategorySlugsInCanonicalOrder covers the helper that assigns SortOrder.
func TestCategorySlugsInCanonicalOrder(t *testing.T) {
	got := CategorySlugsInCanonicalOrder()
	if len(got) != len(CanonicalCategories) {
		t.Fatalf("got %d slugs, want %d", len(got), len(CanonicalCategories))
	}
	for i, s := range got {
		if s != CanonicalCategories[i].Slug {
			t.Errorf("position %d: got %q, want %q", i, s, CanonicalCategories[i].Slug)
		}
	}
	// Mutation safety: the helper must return a fresh slice, otherwise a caller
	// sorting it would reorder the canonical list for the whole process.
	got[0] = "mutated"
	if CanonicalCategories[0].Slug == "mutated" {
		t.Error("CategorySlugsInCanonicalOrder returned a view into CanonicalCategories, not a copy")
	}
}

func kebab(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
