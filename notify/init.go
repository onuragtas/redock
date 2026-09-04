package notify

import (
	"fmt"
	"log"
	"time"

	"redock/platform/memory"
)

// checkInterval is how often the watched states are read. Often enough that a
// problem is noticed within minutes, rare enough that the checks themselves
// cost nothing.
const checkInterval = 5 * time.Minute

var current *Notifier

// Get returns the running notifier, or nil before Init.
func Get() *Notifier { return current }

// Init loads the settings, wires the notifier to the states it watches, and
// starts the timer.
func Init(db *memory.Database, sources Sources, send Sender) {
	settings := loadSettings(db)
	current = New(settings, sources, send)

	go func() {
		// Give the subsystems a moment to come up, or the first check reports
		// that a certificate cannot be read simply because nothing has loaded
		// it yet.
		time.Sleep(time.Minute)

		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for range ticker.C {
			current.Check()
		}
	}()
}

// loadSettings reads the stored settings, creating them on first run.
func loadSettings(db *memory.Database) *Settings {
	if db == nil {
		return DefaultSettings()
	}

	stored := memory.FindAll[*Settings](db, TableName)
	for _, settings := range stored {
		if settings != nil {
			settings.normalize()
			return settings
		}
	}

	settings := DefaultSettings()
	if err := memory.Create(db, TableName, settings); err != nil {
		log.Printf("notify: could not store the default settings: %v", err)
	}
	return settings
}

// Settings returns the live settings, so the dashboard reads and writes the
// same object the notifier consults.
func CurrentSettings(db *memory.Database) *Settings {
	if current == nil {
		return loadSettings(db)
	}

	current.mu.Lock()
	defer current.mu.Unlock()
	if current.settings == nil {
		current.settings = DefaultSettings()
	}
	return current.settings
}

// Save persists changed settings and points the notifier at them.
func Save(db *memory.Database, updated *Settings) (*Settings, error) {
	settings := CurrentSettings(db)

	settings.Enabled = updated.Enabled
	settings.MailboxID = updated.MailboxID
	settings.Recipient = updated.Recipient
	settings.WatchCertificate = updated.WatchCertificate
	settings.WatchQueue = updated.WatchQueue
	settings.WatchMemory = updated.WatchMemory
	settings.WatchBlocked = updated.WatchBlocked
	settings.CertDaysBefore = updated.CertDaysBefore
	settings.QueueThreshold = updated.QueueThreshold
	settings.BlockedThreshold = updated.BlockedThreshold
	settings.RepeatHours = updated.RepeatHours
	settings.normalize()

	if db != nil && settings.ID != 0 {
		if err := memory.Update(db, TableName, settings); err != nil {
			return nil, fmt.Errorf("could not save the settings: %w", err)
		}
	}

	if current != nil {
		current.SetSettings(settings)
	}
	return settings, nil
}
