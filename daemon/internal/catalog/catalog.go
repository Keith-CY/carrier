package catalog

import "sort"

type CandidateStatus string

const (
	StatusActive    CandidateStatus = "active"
	StatusCandidate CandidateStatus = "candidate"
)

type Entry struct {
	ID     string
	Name   string
	Status CandidateStatus
}

func DefaultEntries() []Entry {
	entries := []Entry{
		{ID: "openclaw", Name: "OpenClaw", Status: StatusActive},
		{ID: "pi-mono", Name: "Pi Mono", Status: StatusCandidate},
		{ID: "nanoclaw", Name: "NanoClaw", Status: StatusCandidate},
		{ID: "picoclaw", Name: "Pico Claw", Status: StatusCandidate},
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

func FindByID(id string) (Entry, bool) {
	for _, e := range DefaultEntries() {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}
