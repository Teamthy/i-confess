// Package seed populates a fresh database with a representative MVP content
// library: collections, categories, confessions (with variants + scripture
// references), a default voice, local placeholder audio, and a demo admin.
package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/media"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

const mediaDir = "data/media"

// Seed runs idempotently: if the database already has categories, it returns early.
// Seed populates a fresh database. The signer receives placeholder audio so the
// dev environment exercises the same keyed, signed delivery path as production.
func Seed(db *sql.DB, signer storage.ObjectStorage) error {
	bg := context.Background()
	content := store.NewContentStore(db)
	cats, err := content.ListCategories(bg, true)
	if err != nil {
		return err
	}
	if len(cats) > 0 {
		return nil // already seeded
	}

	audio := store.NewAudioStore(db)
	users := store.NewUserStore(db)

	// ---- Collections ----
	for _, c := range []models.Collection{
		{Name: "The 28", Slug: "the-28", Status: "published", SortOrder: 1, Description: "Launch collection â€” 28 core categories."},
		{Name: "The 38", Slug: "the-38", Status: "published", SortOrder: 2, Description: "Principal expanded content architecture â€” 38 categories."},
	} {
		if err := content.CreateCollection(bg, &c); err != nil {
			return err
		}
	}

	// ---- Categories (representative subset of the 38) ----
	categorySeeds := []struct {
		name, slug, desc, icon string
	}{
		{"Healing", "healing", "Confessions for physical and emotional wholeness.", "healing"},
		{"Faith", "faith", "Declarations that strengthen your faith.", "faith"},
		{"Finance", "finance", "Biblical wisdom for provision and stewardship.", "finance"},
		{"Family", "family", "Declarations over your household.", "family"},
		{"Marriage", "marriage", "Confessions for a strong, God-centered marriage.", "marriage"},
		{"Purpose", "purpose", "Declarations of calling and destiny.", "purpose"},
		{"Breakthrough", "breakthrough", "Confessions for breaking through barriers.", "breakthrough"},
		{"Peace", "peace", "Declarations of calm and rest.", "peace"},
		{"Protection", "protection", "Confessions of safety and covering.", "protection"},
		{"Wisdom", "wisdom", "Declarations for wisdom and clarity.", "wisdom"},
		{"Favor", "favor", "Confessions of grace and favor.", "favor"},
		{"Strength", "strength", "Declarations of endurance and power.", "strength"},
		{"Business", "business", "Confessions for work and enterprise.", "business"},
		{"Joy", "joy", "Declarations of joy and gladness.", "joy"},
		{"Identity", "identity", "Confessions of who you are in Christ.", "identity"},
		{"Thanksgiving", "thanksgiving", "Declarations of gratitude.", "thanksgiving"},
	}

	var categoryIDs []string
	for i, cs := range categorySeeds {
		c := &models.Category{
			Name: cs.name, Slug: cs.slug, Description: cs.desc, Icon: cs.icon,
			Status: "published", SortOrder: i + 1,
		}
		if err := content.CreateCategory(bg, c); err != nil {
			return err
		}
		categoryIDs = append(categoryIDs, c.ID)
	}

	// Attach all categories to both collections (28 â†’ subset of 38 for demo).
	cols, _ := content.ListCollections(bg, true)
	for _, col := range cols {
		for i, cid := range categoryIDs {
			_ = content.AddCategoryToCollection(bg, col.ID, cid, i)
		}
	}

	// ---- Voice ----
	voice := &models.Voice{
		Name: "Grace", Description: "Warm, calm professional narration voice.",
		Type: "professional", Provider: "i-confess", Gender: "female",
		Language: "en", Premium: false, Status: "active",
	}
	if err := audio.CreateVoice(bg, voice); err != nil {
		return err
	}

	// ---- Confessions ----
	// Each confession: title, texts, scripture refs, and duration variants.
	type confessionSeed struct {
		category   string
		title      string
		short      string
		medium     string
		long       string
		scriptures []models.ScriptureRef
		intensity  int
	}
	seeds := []confessionSeed{
		{category: "Healing", title: "I Am Healed", intensity: 3,
			short:  "By His stripes, I am healed.",
			medium: "I declare that by the stripes of Jesus I am healed. Sickness has no authority over my body. I receive wholeness now.",
			long:   "I declare that by the stripes of Jesus I am healed. Sickness and disease have no authority over my body, for my body is the temple of the Holy Spirit. I receive wholeness in every organ, every tissue, and every cell. The life of God flows through me, restoring strength, health, and vitality. I walk in divine health all the days of my life.",
			scriptures: []models.ScriptureRef{
				{Book: "Isaiah", Chapter: 53, Verse: "5", Translation: "KJV", IsDirectQuote: true},
				{Book: "1 Peter", Chapter: 2, Verse: "24", Translation: "KJV", IsDirectQuote: true},
				{Book: "Psalm", Chapter: 103, Verse: "2-3", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Healing", title: "Divine Health", intensity: 2,
			short:  "I walk in divine health.",
			medium: "I walk in divine health and wholeness. Every day my strength is renewed like the eagle's.",
			long:   "I walk in divine health and wholeness. Every day my strength is renewed like the eagle's. The same Spirit that raised Christ from the dead lives in me and quickens my mortal body. I am strong, I am whole, and I refuse to accept any report that contradicts the finished work of the cross.",
			scriptures: []models.ScriptureRef{
				{Book: "Romans", Chapter: 8, Verse: "11", Translation: "KJV", IsDirectQuote: true},
				{Book: "Isaiah", Chapter: 40, Verse: "31", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Faith", title: "Faith That Moves", intensity: 3,
			short:  "My faith moves mountains.",
			medium: "I have the faith of God. I speak to my mountains and they move. Nothing is impossible for me.",
			long:   "I have the God-kind of faith. I speak to the mountains in my life and they move. I believe without wavering, for I know that the word of God cannot fail. My faith grows stronger every day as I hear and confess the word. Nothing shall be impossible for me because I believe.",
			scriptures: []models.ScriptureRef{
				{Book: "Mark", Chapter: 11, Verse: "22-24", Translation: "KJV", IsDirectQuote: true},
				{Book: "Romans", Chapter: 10, Verse: "17", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Faith", title: "Unshakable Trust", intensity: 2,
			short:  "I trust in the Lord with all my heart.",
			medium: "I trust in the Lord with all my heart and lean not on my own understanding. He directs my paths.",
			long:   "I trust in the Lord with all my heart and lean not on my own understanding. In all my ways I acknowledge Him, and He directs my paths. I will not be shaken, for my confidence is in the Lord and not in man. My steps are ordered by God.",
			scriptures: []models.ScriptureRef{
				{Book: "Proverbs", Chapter: 3, Verse: "5-6", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Finance", title: "I Am Prosperous", intensity: 3,
			short:  "I am abundantly provided for.",
			medium: "I declare that I am blessed in the city and blessed in the field. My God supplies all my needs according to His riches.",
			long:   "I declare that I am blessed in the city and blessed in the field, blessed coming in and blessed going out. My God supplies all my needs according to His riches in glory. I am a generous giver, and the windows of heaven are open over my life. Wealth and riches are in my house because I honor the Lord.",
			scriptures: []models.ScriptureRef{
				{Book: "Deuteronomy", Chapter: 28, Verse: "3-6", Translation: "KJV", IsDirectQuote: true},
				{Book: "Philippians", Chapter: 4, Verse: "19", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Finance", title: "Steward of Abundance", intensity: 2,
			short:  "I am a wise steward.",
			medium: "I am a faithful steward of all God entrusts to me. Wisdom guides my financial decisions.",
			long:   "I am a faithful and wise steward of everything God entrusts to my hands. Wisdom guides my financial decisions, and I am diligent in my labor. I honor God with the firstfruits of my increase, and my barns are filled with plenty. Poverty is far from me.",
			scriptures: []models.ScriptureRef{
				{Book: "Proverbs", Chapter: 3, Verse: "9-10", Translation: "KJV", IsDirectQuote: false},
				{Book: "Luke", Chapter: 16, Verse: "10", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Marriage", title: "A Blessed Union", intensity: 2,
			short:  "My marriage is blessed.",
			medium: "My marriage is founded on Christ. I love and honor my spouse, and our home is filled with peace.",
			long:   "My marriage is founded on the rock of Christ. I love and honor my spouse with patience and kindness. Our home is filled with peace, joy, and the presence of God. What God has joined together, no one can separate. We are heirs together of the grace of life.",
			scriptures: []models.ScriptureRef{
				{Book: "Ephesians", Chapter: 5, Verse: "25", Translation: "KJV", IsDirectQuote: false},
				{Book: "Mark", Chapter: 10, Verse: "9", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Purpose", title: "Called With Purpose", intensity: 3,
			short:  "I was created for a purpose.",
			medium: "I was created for a divine purpose. My steps are ordered, and I fulfill the good works prepared for me.",
			long:   "I was created for a divine purpose. Before I was formed in the womb, God knew me and set me apart. My steps are ordered by the Lord, and I fulfill the good works prepared for me before the foundation of the world. I will not live a small life; I will fulfill my assignment with boldness and grace.",
			scriptures: []models.ScriptureRef{
				{Book: "Jeremiah", Chapter: 1, Verse: "5", Translation: "KJV", IsDirectQuote: true},
				{Book: "Ephesians", Chapter: 2, Verse: "10", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Peace", title: "Peace Beyond Understanding", intensity: 1,
			short:  "The peace of God guards my heart.",
			medium: "I am not anxious about anything. The peace of God that passes understanding guards my heart and mind.",
			long:   "I refuse to be anxious about anything. In every situation I bring my requests to God with thanksgiving, and the peace of God that passes all understanding guards my heart and mind in Christ Jesus. I am calm, I am still, and I know that God is with me.",
			scriptures: []models.ScriptureRef{
				{Book: "Philippians", Chapter: 4, Verse: "6-7", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Wisdom", title: "Wisdom And Clarity", intensity: 2,
			short:  "I walk in wisdom and clarity.",
			medium: "I walk in wisdom and clarity. If I lack wisdom, I ask of God, and He gives generously.",
			long:   "I walk in wisdom and clarity. If I lack wisdom, I ask of God, who gives generously to all without finding fault, and it is given to me. The wisdom of God is in me, and I make excellent decisions. I have insight, understanding, and discernment for every situation.",
			scriptures: []models.ScriptureRef{
				{Book: "James", Chapter: 1, Verse: "5", Translation: "KJV", IsDirectQuote: true},
				{Book: "Proverbs", Chapter: 3, Verse: "5-6", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Protection", title: "Under His Wings", intensity: 2,
			short:  "I am safe under His wings.",
			medium: "I dwell in the secret place of the Most High and abide under the shadow of the Almighty. No evil befalls me.",
			long:   "I dwell in the secret place of the Most High and abide under the shadow of the Almighty. He is my refuge and my fortress, my God in whom I trust. No evil shall befall me, nor any plague come near my dwelling, for He gives His angels charge over me.",
			scriptures: []models.ScriptureRef{
				{Book: "Psalm", Chapter: 91, Verse: "1-2", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Strength", title: "Renewed Strength", intensity: 3,
			short:  "My strength is renewed.",
			medium: "I am strong in the Lord and in the power of His might. I can do all things through Christ who strengthens me.",
			long:   "I am strong in the Lord and in the power of His might. I can do all things through Christ who strengthens me. Those who wait on the Lord renew their strength; I mount up with wings as eagles, I run and do not grow weary, I walk and do not faint.",
			scriptures: []models.ScriptureRef{
				{Book: "Philippians", Chapter: 4, Verse: "13", Translation: "KJV", IsDirectQuote: true},
				{Book: "Isaiah", Chapter: 40, Verse: "31", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Business", title: "Fruitful Work", intensity: 2,
			short:  "The work of my hands is blessed.",
			medium: "The Lord blesses the work of my hands. I am diligent, and my labor is fruitful.",
			long:   "The Lord blesses all the work of my hands. I am diligent in my business and faithful in the small things, therefore I am entrusted with more. My enterprise flourishes, my ideas are excellent, and I bring value to everyone I serve. I operate with wisdom and integrity.",
			scriptures: []models.ScriptureRef{
				{Book: "Deuteronomy", Chapter: 28, Verse: "12", Translation: "KJV", IsDirectQuote: false},
				{Book: "Proverbs", Chapter: 22, Verse: "29", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Joy", title: "Joy Unspeakable", intensity: 1,
			short:  "The joy of the Lord is my strength.",
			medium: "The joy of the Lord is my strength. I am filled with joy unspeakable and full of glory.",
			long:   "The joy of the Lord is my strength. I am filled with joy unspeakable and full of glory. This is the day the Lord has made, and I will rejoice and be glad in it. My heart is glad, and my countenance is bright, for the Lord has done great things for me.",
			scriptures: []models.ScriptureRef{
				{Book: "Nehemiah", Chapter: 8, Verse: "10", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Identity", title: "I Am Who God Says", intensity: 3,
			short:  "I am a child of God.",
			medium: "I am a child of God, fearfully and wonderfully made. I am accepted in the Beloved.",
			long:   "I am a child of God, fearfully and wonderfully made. I am accepted in the Beloved, chosen before the foundation of the world. I am the head and not the tail, above and not beneath. My identity is not defined by my past, my mistakes, or the opinions of others, but by the word of God.",
			scriptures: []models.ScriptureRef{
				{Book: "1 John", Chapter: 3, Verse: "1", Translation: "KJV", IsDirectQuote: false},
				{Book: "Psalm", Chapter: 139, Verse: "14", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Thanksgiving", title: "A Grateful Heart", intensity: 1,
			short:  "I enter His gates with thanksgiving.",
			medium: "I enter His gates with thanksgiving and His courts with praise. My heart is full of gratitude.",
			long:   "I enter His gates with thanksgiving and His courts with praise. I am grateful for the goodness of God in my life. In everything I give thanks, for this is the will of God in Christ Jesus concerning me. My heart overflows with gratitude, and I bless the Lord at all times.",
			scriptures: []models.ScriptureRef{
				{Book: "Psalm", Chapter: 100, Verse: "4", Translation: "KJV", IsDirectQuote: true},
				{Book: "1 Thessalonians", Chapter: 5, Verse: "18", Translation: "KJV", IsDirectQuote: true},
			}},
	}

	// Duration variants used across all confessions.
	variantDefs := []struct {
		label   string
		seconds int
	}{{"30s", 30}, {"1m", 60}, {"3m", 180}, {"5m", 300}}

	catByName := map[string]string{}
	for i, cs := range categorySeeds {
		catByName[cs.name] = categoryIDs[i]
	}

	for _, s := range seeds {
		catID := catByName[s.category]
		var variants []models.ConfessionVariant
		for _, vd := range variantDefs {
			variants = append(variants, models.ConfessionVariant{Label: vd.label, DurationSeconds: vd.seconds})
		}
		c := &models.Confession{
			CategoryID: catID, Title: s.title, ShortText: s.short, MediumText: s.medium, LongText: s.long,
			Intensity: s.intensity, Language: "en", Status: "published", Author: "i-confess content team",
			Variants: variants, Scriptures: s.scriptures,
		}
		if err := content.CreateConfession(bg, c); err != nil {
			return err
		}

		// Generate placeholder audio for each variant and attach it to the voice.
		for _, v := range c.Variants {
			// Store the canonical KEY, never a URL. The API mints a signed,
			// expiring link per request (PRD S11).
			key := storage.AudioKeyFor(c.ID, v.ID, voice.ID, "en", 1)
			if err := signer.Upload(bg, key, media.ToneBytes(v.DurationSeconds), map[string]string{
				"confession_id": c.ID, "voice_id": voice.ID, "language": "en",
			}); err != nil {
				return fmt.Errorf("store placeholder audio: %w", err)
			}
			asset := &models.AudioAsset{
				ConfessionID: c.ID, VariantID: v.ID, VoiceID: voice.ID,
				URL: key, DurationSeconds: v.DurationSeconds, Status: "ready",
			}
			if err := audio.UpsertAsset(bg, asset); err != nil {
				return err
			}
		}
	}

	// ---- Demo admin + demo user ----
	adminHash, _ := auth.HashPassword("admin12345")
	admin, err := users.Create(bg, "admin@iconfess.dev", adminHash, "Admin", "UTC")
	if err != nil {
		return err
	}
	if err := users.SetAdminRole(bg, admin.ID, "super_admin"); err != nil {
		return err
	}

	userHash, _ := auth.HashPassword("password123")
	if _, err := users.Create(bg, "demo@iconfess.dev", userHash, "Demo User", "Africa/Lagos"); err != nil {
		return err
	}

	log.Printf("seed: created %d categories, %d confessions, 1 voice, demo admin + user",
		len(categorySeeds), len(seeds))
	return nil
}

// EnsureMediaDir creates the media directory if needed (used by tests/tools).
func EnsureMediaDir() error {
	return os.MkdirAll(mediaDir, 0o755)
}
