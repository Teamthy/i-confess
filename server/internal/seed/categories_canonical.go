package seed

// This file is the canonical category list for I CONFESS, and it exists to
// settle an argument that documentation alone cannot settle.
//
// The categories are backend-configurable at runtime: they live in the
// categories table and are served through the API, so mobile clients never
// hard-code them. This list is not that mechanism. It is the *seed of record* —
// the agreed set a fresh deployment is expected to contain, and the reference
// against which drift in the table is detected.
//
// Two rules apply and both are enforced by categories_canonical_test.go:
//
//  1. Exactly 39 entries, each with a unique name and a unique slug.
//  2. Every entry carries a Description (shown on the category detail screen)
//     and a Tagline (shown on the website's category rail). Neither may be
//     empty, because an empty tagline silently renders as a broken card.
//
// Adding a category is a product decision, not an engineering one. Change this
// list only when the product owner has agreed the change.

// CanonicalCategory is one agreed category of life that users confess over.
type CanonicalCategory struct {
	Name string // display name, e.g. "Healing"
	Slug string // URL-stable identifier, e.g. "healing"

	// Description is the longer line shown on the category detail screen.
	// It should read as an invitation, not a definition.
	Description string

	// Tagline is the short line shown on the marketing website's category
	// rail and on explore cards. It is written in the second person because
	// the product asks the user to speak, not to read.
	Tagline string

	Icon string // design-system icon key
}

// CanonicalCategories is the 39 categories I CONFESS is built around.
//
// Order is deliberate: it leads with the areas users reach for first when they
// are in trouble (healing, health, finance, breakthrough), then widens into
// relationships and identity, then into growth and practice. SortOrder in the
// database is assigned from this order.
var CanonicalCategories = []CanonicalCategory{
	{
		Name:        "Healing",
		Slug:        "healing",
		Description: "Confessions for physical and emotional wholeness.",
		Tagline:     "Speak life over your health.",
		Icon:        "healing",
	},
	{
		Name:        "Health",
		Slug:        "health",
		Description: "Declarations for the body you have been given.",
		Tagline:     "Honour the body you have been given.",
		Icon:        "health",
	},
	{
		Name:        "Finance",
		Slug:        "finance",
		Description: "Biblical wisdom for provision and stewardship.",
		Tagline:     "Steward what is in your hands.",
		Icon:        "finance",
	},
	{
		Name:        "Wealth",
		Slug:        "wealth",
		Description: "Declarations over abundance and its responsibilities.",
		Tagline:     "Declare God's provision and wisdom.",
		Icon:        "wealth",
	},
	{
		Name:        "Breakthrough",
		Slug:        "breakthrough",
		Description: "Confessions for breaking through barriers.",
		Tagline:     "Name the wall, then speak through it.",
		Icon:        "breakthrough",
	},
	{
		Name:        "Marriage",
		Slug:        "marriage",
		Description: "Confessions for a strong, God-centred marriage.",
		Tagline:     "Speak covenant over your home.",
		Icon:        "marriage",
	},
	{
		Name:        "Relationships",
		Slug:        "relationships",
		Description: "Declarations over the people closest to you.",
		Tagline:     "Truth spoken over the people near you.",
		Icon:        "relationships",
	},
	{
		Name:        "Faith",
		Slug:        "faith",
		Description: "Declarations that strengthen your faith.",
		Tagline:     "Say what you believe before you see it.",
		Icon:        "faith",
	},
	{
		Name:        "Peace",
		Slug:        "peace",
		Description: "Declarations of calm and rest.",
		Tagline:     "Quiet the noise with a settled word.",
		Icon:        "peace",
	},
	{
		Name:        "Family",
		Slug:        "family",
		Description: "Declarations over your household.",
		Tagline:     "Cover the people you love in prayer and truth.",
		Icon:        "family",
	},
	{
		Name:        "Purpose",
		Slug:        "purpose",
		Description: "Declarations of calling and destiny.",
		Tagline:     "Speak with clarity about where you are going.",
		Icon:        "purpose",
	},
	{
		Name:        "Identity",
		Slug:        "identity",
		Description: "Confessions of who you are in Christ.",
		Tagline:     "Answer the question of who you are.",
		Icon:        "identity",
	},
	{
		Name:        "Wisdom",
		Slug:        "wisdom",
		Description: "Declarations for wisdom and clarity.",
		Tagline:     "Ask, then speak what you were given.",
		Icon:        "wisdom",
	},
	{
		Name:        "Protection",
		Slug:        "protection",
		Description: "Confessions of safety and covering.",
		Tagline:     "Stand under a covering you did not build.",
		Icon:        "protection",
	},
	{
		Name:        "Career",
		Slug:        "career",
		Description: "Declarations over your work and calling.",
		Tagline:     "Work as one who is sent.",
		Icon:        "career",
	},
	{
		Name:        "Business",
		Slug:        "business",
		Description: "Confessions for work and enterprise.",
		Tagline:     "Build with integrity and courage.",
		Icon:        "business",
	},
	{
		Name:        "Leadership",
		Slug:        "leadership",
		Description: "Declarations for those carrying weight.",
		Tagline:     "Carry weight without losing your soul.",
		Icon:        "leadership",
	},
	{
		Name:        "Favor",
		Slug:        "favor",
		Description: "Confessions of grace and favour.",
		Tagline:     "Walk into rooms already prepared for you.",
		Icon:        "favor",
	},
	{
		Name:        "Provision",
		Slug:        "provision",
		Description: "Declarations over daily supply.",
		Tagline:     "Name your need, then name your Provider.",
		Icon:        "provision",
	},
	{
		Name:        "Confidence",
		Slug:        "confidence",
		Description: "Declarations against self-doubt.",
		Tagline:     "Speak from who you are, not what you have done.",
		Icon:        "confidence",
	},
	{
		Name:        "Discipline",
		Slug:        "discipline",
		Description: "Confessions for consistency and self-control.",
		Tagline:     "Choose today what tomorrow will thank you for.",
		Icon:        "discipline",
	},
	{
		Name:        "Joy",
		Slug:        "joy",
		Description: "Declarations of joy and gladness.",
		Tagline:     "Rejoice before the answer arrives.",
		Icon:        "joy",
	},
	{
		Name:        "Hope",
		Slug:        "hope",
		Description: "Declarations for the long wait.",
		Tagline:     "Hold the promise when the room is quiet.",
		Icon:        "hope",
	},
	{
		Name:        "Freedom",
		Slug:        "freedom",
		Description: "Confessions for deliverance and release.",
		Tagline:     "Speak over the chains that still feel real.",
		Icon:        "freedom",
	},
	{
		Name:        "Spiritual Growth",
		Slug:        "spiritual-growth",
		Description: "Declarations for maturity and depth.",
		Tagline:     "Grow on purpose, not by accident.",
		Icon:        "spiritual-growth",
	},
	{
		Name:        "Prayer",
		Slug:        "prayer",
		Description: "Confessions that teach you to pray.",
		Tagline:     "Learn to speak to God, and out loud.",
		Icon:        "prayer",
	},
	{
		Name:        "Children",
		Slug:        "children",
		Description: "Declarations over your children.",
		Tagline:     "Speak destiny over the next generation.",
		Icon:        "children",
	},
	{
		Name:        "Parenting",
		Slug:        "parenting",
		Description: "Confessions for those raising others.",
		Tagline:     "Lead your home with patience and truth.",
		Icon:        "parenting",
	},
	{
		Name:        "Direction",
		Slug:        "direction",
		Description: "Declarations for decisions and crossroads.",
		Tagline:     "Ask for the next step, then take it.",
		Icon:        "direction",
	},
	{
		Name:        "Creativity",
		Slug:        "creativity",
		Description: "Confessions for makers and builders.",
		Tagline:     "Make things with what you were given.",
		Icon:        "creativity",
	},
	{
		Name:        "Productivity",
		Slug:        "productivity",
		Description: "Declarations for focus and follow-through.",
		Tagline:     "Finish what matters most first.",
		Icon:        "productivity",
	},
	{
		Name:        "Emotional Strength",
		Slug:        "emotional-strength",
		Description: "Declarations for steady hearts.",
		Tagline:     "Stand steady when you feel unsteady.",
		Icon:        "emotional-strength",
	},
	{
		Name:        "Rest",
		Slug:        "rest",
		Description: "Confessions for sabbath and stillness.",
		Tagline:     "Stop striving long enough to receive.",
		Icon:        "rest",
	},
	{
		Name:        "Gratitude",
		Slug:        "gratitude",
		Description: "Declarations of thanksgiving.",
		Tagline:     "Name the good before you ask for more.",
		Icon:        "gratitude",
	},
	{
		Name:        "Forgiveness",
		Slug:        "forgiveness",
		Description: "Confessions for releasing and receiving pardon.",
		Tagline:     "Release what you were never meant to carry.",
		Icon:        "forgiveness",
	},
	{
		Name:        "Love",
		Slug:        "love",
		Description: "Declarations over how you love.",
		Tagline:     "Speak love until it becomes ordinary.",
		Icon:        "love",
	},
	{
		Name:        "Overcoming Fear",
		Slug:        "overcoming-fear",
		Description: "Confessions against anxiety and dread.",
		Tagline:     "Say it out loud until fear loses its voice.",
		Icon:        "overcoming-fear",
	},
	{
		Name:        "Success",
		Slug:        "success",
		Description: "Declarations for achievement without compromise.",
		Tagline:     "Succeed without losing your soul.",
		Icon:        "success",
	},
	{
		Name:        "Destiny",
		Slug:        "destiny",
		Description: "Confessions over the future you are walking into.",
		Tagline:     "Speak the future you are walking into.",
		Icon:        "destiny",
	},
}

// CanonicalCategoryCount is the agreed number of categories. It is a constant
// so a test can assert against the product decision rather than against
// len(CanonicalCategories), which would make the test tautological.
const CanonicalCategoryCount = 39

// CategorySlugsInCanonicalOrder returns the slugs in canonical order. Callers
// use it to assign SortOrder deterministically.
func CategorySlugsInCanonicalOrder() []string {
	out := make([]string, len(CanonicalCategories))
	for i, c := range CanonicalCategories {
		out[i] = c.Slug
	}
	return out
}
