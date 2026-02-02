package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSidebarItems(t *testing.T) {
	items := GetSidebarItems()

	// 1. Validation: Should have at least one favorite (Home) and headers
	assert.NotEmpty(t, items, "Sidebar items should not be empty")

	// 2. Check for Headers
	hasFavHeader := false
	hasLocHeader := false
	hasNetHeader := false

	for _, item := range items {
		if item.IsHeader {
			switch item.Section {
			case SectionFavorites:
				hasFavHeader = true
			case SectionLocations:
				hasLocHeader = true
			case SectionNetwork:
				hasNetHeader = true
			}
		}
	}

	assert.True(t, hasFavHeader, "Should have FAVORITES header")
	assert.True(t, hasLocHeader, "Should have LOCATIONS header")
	assert.True(t, hasNetHeader, "Should have NETWORK header")

	// 3. Verify Home Directory is present in Favorites
	foundHome := false
	for _, item := range items {
		if item.Section == SectionFavorites && item.Label == "Home" {
			foundHome = true
			assert.NotEmpty(t, item.Path, "Home path should not be empty")
		}
	}
	assert.True(t, foundHome, "Should find Home directory in sidebar")
}
