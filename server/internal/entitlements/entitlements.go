package entitlements

const (
	PlanFree    = "free"
	PlanPremium = "premium"
)

// Entitlements defines what features a user can access.
type Entitlements struct {
	Plan                         string
	CanCreatePersonalConfessions bool
	PersonalConfessionsLimit     int
	CanGenerateAudio             bool
	CanDownload                  bool
	DownloadsLimit               int
	CanCreateFolders             bool
	CanCreateCollections         bool
	CanAccessPremiumVoices       bool
	CanAccessPremiumContent      bool
	CanUseSleepTimer             bool
	CanUsePlaybackSpeed          bool
	MaxConcurrentDownloads       int
	OfflineHoursAllowed          int
	AnalyticsAccess              bool
}

// FromPlan returns the entitlements for a given plan.
func FromPlan(plan string) Entitlements {
	switch plan {
	case PlanPremium:
		return Entitlements{
			Plan:                         PlanPremium,
			CanCreatePersonalConfessions: true,
			PersonalConfessionsLimit:     1000,
			CanGenerateAudio:             true,
			CanDownload:                  true,
			DownloadsLimit:               500,
			CanCreateFolders:             true,
			CanCreateCollections:         true,
			CanAccessPremiumVoices:       true,
			CanAccessPremiumContent:      true,
			CanUseSleepTimer:             true,
			CanUsePlaybackSpeed:          true,
			MaxConcurrentDownloads:       5,
			OfflineHoursAllowed:          168, // 7 days
			AnalyticsAccess:              true,
		}
	case PlanFree:
		fallthrough
	default:
		return Entitlements{
			Plan:                         PlanFree,
			CanCreatePersonalConfessions: true,
			PersonalConfessionsLimit:     10,
			CanGenerateAudio:             false,
			CanDownload:                  false,
			DownloadsLimit:               0,
			CanCreateFolders:             false,
			CanCreateCollections:         false,
			CanAccessPremiumVoices:       false,
			CanAccessPremiumContent:      false,
			CanUseSleepTimer:             true,
			CanUsePlaybackSpeed:          false,
			MaxConcurrentDownloads:       0,
			OfflineHoursAllowed:          0,
			AnalyticsAccess:              false,
		}
	}
}

// CanAccessVoice checks if a user with these entitlements can access a voice.
func (e *Entitlements) CanAccessVoice(voicePremium bool) bool {
	if !voicePremium {
		return true // free voices accessible to everyone
	}
	return e.CanAccessPremiumVoices
}

// CanAccessConfession checks if a user can access a confession.
func (e *Entitlements) CanAccessConfession(confessionPremium bool) bool {
	if !confessionPremium {
		return true // free confessions accessible to everyone
	}
	return e.CanAccessPremiumContent
}
