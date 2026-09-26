package domain

import (
	"maps"
	"time"
)

// The record of a suspension lives in Server.Metadata under a key the panel
// owns: only Suspend and Unsuspend write it, and ReplaceMetadata carries it
// over, so an update that sends the metadata back as a whole can neither forge
// nor drop it.
const (
	metaKeySuspension     = "suspension"
	suspensionFieldSince  = "since"
	suspensionFieldReason = "reason"

	// MaxSuspendReasonLength bounds the reason given for a suspension. It is a
	// line shown next to the server, not a place for a log.
	MaxSuspendReasonLength = 255
)

// ServerSuspension records when and why a server was suspended. Since is nil
// for a server blocked before the panel kept the record.
type ServerSuspension struct {
	Since  *time.Time
	Reason string
}

// IsSuspended reports whether the server is suspended: stopped, and refused
// everything that would run it again until the suspension is lifted. Every
// enforcement point asks this rather than reading Blocked, so a suspension by
// date can join here later.
func (s *Server) IsSuspended() bool {
	return s.Blocked
}

// Suspension returns the record of the current suspension, or nil while the
// server is not suspended. A record the panel cannot read counts as an
// unknown date and no reason rather than as no suspension: the flag decides.
func (s *Server) Suspension() *ServerSuspension {
	if !s.IsSuspended() {
		return nil
	}

	suspension := &ServerSuspension{}

	record, ok := s.Metadata[metaKeySuspension].(map[string]any)
	if !ok {
		return suspension
	}

	if raw, ok := record[suspensionFieldSince].(string); ok {
		if since, err := time.Parse(time.RFC3339, raw); err == nil {
			suspension.Since = &since
		}
	}

	if reason, ok := record[suspensionFieldReason].(string); ok {
		suspension.Reason = reason
	}

	return suspension
}

// Suspend blocks the server and records when and why. Suspending a suspended
// server keeps the original date; the reason changes only when one is given,
// and an empty one clears it.
func (s *Server) Suspend(now time.Time, reason *string) {
	suspension := ServerSuspension{Since: new(now.UTC().Truncate(time.Second))}
	if current := s.Suspension(); current != nil {
		suspension = *current
	}

	if reason != nil {
		suspension.Reason = *reason
	}

	record := map[string]any{}
	if suspension.Since != nil {
		record[suspensionFieldSince] = suspension.Since.UTC().Format(time.RFC3339)
	}
	if suspension.Reason != "" {
		record[suspensionFieldReason] = suspension.Reason
	}

	s.Blocked = true
	s.Metadata = maps.Clone(s.Metadata)

	if s.Metadata == nil {
		s.Metadata = Metadata{}
	}

	s.Metadata[metaKeySuspension] = record
}

// Unsuspend lifts the suspension and drops its record.
func (s *Server) Unsuspend() {
	s.Blocked = false

	if _, ok := s.Metadata[metaKeySuspension]; !ok {
		return
	}

	s.Metadata = maps.Clone(s.Metadata)
	delete(s.Metadata, metaKeySuspension)
}

// ReplaceMetadata replaces the metadata as a whole, as an update does, while
// keeping the panel's own record: whatever the new bag says about it is
// ignored.
func (s *Server) ReplaceMetadata(metadata Metadata) {
	record, hasRecord := s.Metadata[metaKeySuspension]

	_, sendsRecord := metadata[metaKeySuspension]
	if !hasRecord && !sendsRecord {
		s.Metadata = metadata

		return
	}

	replaced := maps.Clone(metadata)
	if replaced == nil {
		replaced = Metadata{}
	}

	delete(replaced, metaKeySuspension)

	if hasRecord {
		replaced[metaKeySuspension] = record
	}

	s.Metadata = replaced
}

// PublicMetadata returns the metadata as the API shows it: without the
// panel's own record, which is served as a field of its own.
func (s *Server) PublicMetadata() Metadata {
	if _, ok := s.Metadata[metaKeySuspension]; !ok {
		return s.Metadata
	}

	public := maps.Clone(s.Metadata)
	delete(public, metaKeySuspension)

	return public
}

// MayBeRunning reports whether the server's process could be up: it is
// installed and the daemon has not reported it down within the window in
// which a report is trusted. An unknown state counts as running.
func (s *Server) MayBeRunning() bool {
	if s.Installed != ServerInstalledStatusInstalled {
		return false
	}

	if s.ProcessActive || s.LastProcessCheck == nil {
		return true
	}

	return !s.LastProcessCheck.UTC().After(time.Now().UTC().Add(-timeExpireProcessCheck))
}

// RefusedWhileSuspended reports whether a task of this type is refused for a
// suspended server: whatever runs the server or prepares its files to run,
// as opposed to stopping, deleting or moving it.
func (t DaemonTaskType) RefusedWhileSuspended() bool {
	switch t {
	case DaemonTaskTypeServerStart,
		DaemonTaskTypeServerRestart,
		DaemonTaskTypeServerUpdate,
		DaemonTaskTypeServerInstall:
		return true
	default:
		return false
	}
}
