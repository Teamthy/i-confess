package bible

import "testing"

func TestRegistryRightsAreExplicitAndDefaultDeny(t *testing.T) {
	foundSwahili := false
	for _, version := range Versions {
		if version.Provider != "open-bibles" || version.ProviderTranslationID != version.ID {
			t.Errorf("%s has inconsistent source provenance: provider=%q provider id=%q", version.ID, version.Provider, version.ProviderTranslationID)
		}
		if version.ID == "swahili" {
			foundSwahili = true
			if version.Status != "pending_review" || version.Rights.APIExposureAllowed || version.Rights.CopyAllowed || version.Rights.OfflineAllowed || version.Rights.AudioAllowed {
				t.Errorf("Swahili must remain pending with use rights disabled: %+v", version)
			}
			continue
		}
		if version.Status != "active" || !version.Rights.APIExposureAllowed || !version.Rights.CopyAllowed || !version.Rights.OfflineAllowed {
			t.Errorf("reviewed local translation %q lacks its explicit grants", version.ID)
		}
		if version.Rights.AudioAllowed {
			t.Errorf("audio must require a separate review for %q", version.ID)
		}
	}
	if !foundSwahili {
		t.Fatal("the pending Swahili candidate disappeared from the registry")
	}
}
