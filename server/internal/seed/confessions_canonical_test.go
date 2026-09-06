package seed

import (
	"strings"
	"testing"
)

// TestEveryCanonicalCategoryHasAtLeastTwoConfessions is the launch test.
//
// Decision D3 says all 39 categories must exist at launch. A category row with
// no confession under it renders as an empty screen, so the count of categories
// is not the claim that matters - the coverage is.
func TestEveryCanonicalCategoryHasAtLeastTwoConfessions(t *testing.T) {
	counts := map[string]int{}
	for _, c := range CanonicalConfessions {
		counts[c.Category]++
	}

	var empty, thin []string
	for _, cat := range CanonicalCategories {
		switch {
		case counts[cat.Name] == 0:
			empty = append(empty, cat.Name)
		case counts[cat.Name] < 2:
			thin = append(thin, cat.Name)
		}
	}

	if len(empty) > 0 {
		t.Errorf("%d canonical categories have no confession at all: %s",
			len(empty), strings.Join(empty, ", "))
	}
	if len(thin) > 0 {
		t.Errorf("%d canonical categories have only one confession: %s",
			len(thin), strings.Join(thin, ", "))
	}

	t.Logf("%d confessions across %d categories", len(CanonicalConfessions), len(CanonicalCategories))
}

// TestConfessionsOnlyReferenceCanonicalCategories catches a typo in a category
// name. A misspelt name silently produces an orphan confession that no
// category screen can ever show, and the count test above would not notice
// because it counts by canonical name.
func TestConfessionsOnlyReferenceCanonicalCategories(t *testing.T) {
	valid := make(map[string]bool, len(CanonicalCategories))
	for _, cat := range CanonicalCategories {
		valid[cat.Name] = true
	}

	for _, c := range CanonicalConfessions {
		if !valid[c.Category] {
			t.Errorf("confession %q references unknown category %q", c.Title, c.Category)
		}
	}
}

// TestConfessionTitlesAreUnique covers the case where the same confession was
// pasted under two headings. The user cannot tell the rows apart.
func TestConfessionTitlesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, c := range CanonicalConfessions {
		if prev, dup := seen[c.Title]; dup {
			t.Errorf("title %q used by both %q and %q", c.Title, prev, c.Category)
			continue
		}
		seen[c.Title] = c.Category
	}
}

// TestConfessionTextsAreOrderedByLength is the one the session engine depends
// on. The engine assembles a queue to fill a requested duration and picks the
// variant whose text best fits. A "long" text shorter than its "medium"
// inverts that choice and the queue silently under-fills.
func TestConfessionTextsAreOrderedByLength(t *testing.T) {
	for _, c := range CanonicalConfessions {
		if c.Title == "" {
			t.Error("a confession has no title")
			continue
		}
		if strings.TrimSpace(c.Short) == "" || strings.TrimSpace(c.Medium) == "" || strings.TrimSpace(c.Long) == "" {
			t.Errorf("%q has an empty text variant", c.Title)
			continue
		}
		if len(c.Short) > len(c.Medium) || len(c.Medium) > len(c.Long) {
			t.Errorf("%q: variants are not increasing in length (short=%d medium=%d long=%d)",
				c.Title, len(c.Short), len(c.Medium), len(c.Long))
		}
		if len(c.Short) == len(c.Long) {
			t.Errorf("%q: short and long are the same text", c.Title)
		}
	}
}

// TestEveryConfessionCarriesScripture is what separates this from an
// affirmation app. Every confession has to be traceable to a text.
func TestEveryConfessionCarriesScripture(t *testing.T) {
	for _, c := range CanonicalConfessions {
		if len(c.Scriptures) == 0 {
			t.Errorf("%q has no scripture reference", c.Title)
			continue
		}
		for _, s := range c.Scriptures {
			if s.Book == "" || s.Chapter <= 0 || s.Verse == "" {
				t.Errorf("%q has an incomplete scripture reference: %+v", c.Title, s)
			}
			if s.Translation == "" {
				t.Errorf("%q cites %s without naming a translation", c.Title, s.Book)
			}
		}
	}
}

// TestIntensityIsInRange keeps the field usable as a sort key. The session
// engine and the explore screen both order by it, so a value outside the
// documented band quietly sorts to the wrong end.
func TestIntensityIsInRange(t *testing.T) {
	for _, c := range CanonicalConfessions {
		if c.Intensity < 1 || c.Intensity > 5 {
			t.Errorf("%q has intensity %d, outside 1-5", c.Title, c.Intensity)
		}
	}
}
