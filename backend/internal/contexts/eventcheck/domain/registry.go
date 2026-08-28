package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

var (
	registryOnce sync.Once
	registryData RegistryDocument
)

func Registry() []RegistryEntry {
	registryOnce.Do(func() {
		if err := json.Unmarshal([]byte(generatedCheckModelRegistryJSON), &registryData); err != nil {
			panic(fmt.Sprintf("decode generated Check Model registry: %v", err))
		}
	})
	return cloneRegistryEntries(registryData.Models)
}

func ActiveRegistry() []RegistryEntry {
	entries := Registry()
	active := make([]RegistryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Model.Status == ModelStatusActive {
			active = append(active, entry)
		}
	}
	return active
}

func LookupModel(id string, version int) (RegistryEntry, bool) {
	for _, entry := range Registry() {
		if entry.Model.ID == id && entry.Model.Version == version {
			return entry, true
		}
	}
	return RegistryEntry{}, false
}

func ActiveGlobalChecks() []RegistryEntry {
	entries := make([]RegistryEntry, 0)
	for _, entry := range ActiveRegistry() {
		if entry.Model.Kind == ModelKindGlobalCheck {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Model.ID == entries[right].Model.ID {
			return entries[left].Model.Version < entries[right].Model.Version
		}
		return entries[left].Model.ID < entries[right].Model.ID
	})
	return entries
}

func cloneRegistryEntries(entries []RegistryEntry) []RegistryEntry {
	encoded, err := json.Marshal(entries)
	if err != nil {
		panic(fmt.Sprintf("clone Check Model registry: %v", err))
	}
	var cloned []RegistryEntry
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		panic(fmt.Sprintf("decode cloned Check Model registry: %v", err))
	}
	return cloned
}
