package main

import (
	"fmt"

	redc "red-cloud/mod"
)

// ListTimelineEvents returns paginated timeline events with optional filters
func (a *App) ListTimelineEvents(limit, offset int, category, search string) (*redc.TimelineListResult, error) {
	if a.timelineStore == nil {
		return &redc.TimelineListResult{Events: []redc.TimelineEvent{}, Total: 0}, nil
	}
	return a.timelineStore.List(limit, offset, category, search)
}

// ClearTimeline deletes all timeline events
func (a *App) ClearTimeline() error {
	if a.timelineStore == nil {
		return fmt.Errorf("timeline store not initialized")
	}
	return a.timelineStore.Clear()
}

// logTimeline is a convenience method to record a timeline event
func (a *App) logTimeline(category, eventType, caseID, caseName, message, detail, level string) {
	if a.timelineStore != nil {
		a.timelineStore.Log(category, eventType, caseID, caseName, message, detail, level)
	}
}
