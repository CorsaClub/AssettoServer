// Package handlers manages interactions with the Assetto Corsa server
package handlers

// This file exports all the functions from the other files to avoid redeclaration errors.
// The functions are defined in their respective files but exported here to avoid conflicts.

// Export functions from server_core.go
var (
	// HandleServerOutput is exported from server_core.go
	HandleServerOutput = handleServerOutput
)

// Export functions from session_handlers.go
var (
	// StartNewSession is exported from session_handlers.go
	StartNewSession = startNewSession
)

// Export functions from utility.go
var (
	// CopyLabels is exported from utility.go
	CopyLabels = copyLabels

	// UpdatePlayerCount is exported from utility.go
	UpdatePlayerCount = updatePlayerCount

	// LogEvent is exported from utility.go
	LogEvent = logEvent

	// ExtractVersion is exported from utility.go
	ExtractVersion = extractVersion

	// ExtractConfigFile is exported from utility.go
	ExtractConfigFile = extractConfigFile

	// ExtractPluginName is exported from utility.go
	ExtractPluginName = extractPluginName

	// ExtractChecksumAsset is exported from utility.go
	ExtractChecksumAsset = extractChecksumAsset
)
