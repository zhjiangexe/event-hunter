package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

var (
	registryOnce    sync.Once
	registryData    RegistryDocument
	registrySources map[string]string
)

func Registry() []RegistryEntry {
	registryOnce.Do(func() {
		if err := json.Unmarshal([]byte(generatedCheckModelRegistryJSON), &registryData); err != nil {
			panic(fmt.Sprintf("decode generated Check Model registry: %v", err))
		}
		if err := json.Unmarshal([]byte(generatedCheckModelSourcesJSON), &registrySources); err != nil {
			panic(fmt.Sprintf("decode generated Check Model sources: %v", err))
		}
		for _, entry := range registryData.Models {
			source, ok := registrySources[modelSourceKey(entry.Model.ID, entry.Model.Version)]
			if !ok {
				panic(fmt.Sprintf("generated Check Model source missing: %s@%d", entry.Model.ID, entry.Model.Version))
			}
			checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
			if checksum != entry.Checksum {
				panic(fmt.Sprintf("generated Check Model source checksum mismatch: %s@%d", entry.Model.ID, entry.Model.Version))
			}
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

func LookupModelSource(id string, version int) (ModelSourceDocument, bool) {
	entry, ok := LookupModel(id, version)
	if !ok {
		return ModelSourceDocument{}, false
	}
	source, ok := registrySources[modelSourceKey(id, version)]
	if !ok {
		return ModelSourceDocument{}, false
	}
	return ModelSourceDocument{
		ModelID: id, Version: version, SourcePath: entry.SourcePath,
		Checksum: entry.Checksum, YAML: source,
	}, true
}

func modelSourceKey(id string, version int) string {
	return fmt.Sprintf("%s@%d", id, version)
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
