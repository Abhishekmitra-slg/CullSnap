package picker

import "fyne.io/fyne/v2"

// SidebarSection represents categories in the sidebar
type SidebarSection string

const (
	SectionFavorites SidebarSection = "FAVORITES"
	SectionLocations SidebarSection = "LOCATIONS"
	SectionNetwork   SidebarSection = "NETWORK"
)

// SidebarItem represents a single row in the sidebar
type SidebarItem struct {
	Label    string
	Icon     fyne.Resource
	Path     string
	Section  SidebarSection
	IsAction bool // If true, clicking this triggers an action
	IsHeader bool // If true, this is a section header
}
