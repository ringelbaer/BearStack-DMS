// Datei erstellt Auswahloptionen für die Darstellung der Einstellungen.
package server

type TagDisplayOption struct {
	Value string
	Label string
}

type ThemeOption struct {
	Value       string
	Label       string
	Description string
}

type HomePageOption struct {
	Value string
	Label string
	URL   string
}

type TrashRetentionOption struct {
	Value int
	Label string
}

func tagDisplayOptions() []TagDisplayOption {
	return []TagDisplayOption{
		{Value: tagDisplayModeLower, Label: "kleinschreibung"},
		{Value: tagDisplayModeFirst, Label: "Erster Buchstabe Groß"},
		{Value: tagDisplayModeUpper, Label: "GROSSSCHREIBUNG"},
	}
}

func themeOptions() []ThemeOption {
	return []ThemeOption{
		{Value: themeModeDefault, Label: "Standard", Description: "Kühles BearStack-Design"},
		{Value: themeModeDesign2, Label: "Wüste", Description: "Warme, helle Admin-Oberfläche"},
	}
}

func homePageOptions(photosEnabled, cloudEnabled bool) []HomePageOption {
	options := []HomePageOption{
		{Value: homePageDocuments, Label: "Dokumente", URL: homePageURL(homePageDocuments)},
		{Value: homePageFolders, Label: "Ordner", URL: homePageURL(homePageFolders)},
	}
	if cloudEnabled {
		options = append(options, HomePageOption{Value: homePageCloud, Label: "Wolke", URL: homePageURL(homePageCloud)})
	}
	if photosEnabled {
		options = append(options, HomePageOption{Value: homePagePhotos, Label: "Fotos", URL: homePageURL(homePagePhotos)})
	}
	return options
}

func trashRetentionOptions() []TrashRetentionOption {
	return []TrashRetentionOption{
		{Value: 0, Label: "Nie"},
		{Value: 30, Label: "Nach 30 Tagen"},
		{Value: 60, Label: "Nach 60 Tagen"},
		{Value: 90, Label: "Nach 90 Tagen"},
	}
}
