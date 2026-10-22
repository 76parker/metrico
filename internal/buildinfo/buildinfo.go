// Package buildinfo formats application build metadata.
package buildinfo

import "fmt"

// Unknown is the placeholder for build metadata that was not provided.
const Unknown = "N/A"

// Format returns three newline-terminated lines, replacing empty values with Unknown.
func Format(version, date, commit string) string {
	if version == "" {
		version = Unknown
	}
	if date == "" {
		date = Unknown
	}
	if commit == "" {
		commit = Unknown
	}

	return fmt.Sprintf("Build version: %s\nBuild date: %s\nBuild commit: %s\n", version, date, commit)
}
