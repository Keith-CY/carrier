package profilesync

import (
	"encoding/json"
	"reflect"
	"sort"
)

type ConflictPolicy string

const (
	ConflictPolicyPreferLocal  ConflictPolicy = "prefer_local"
	ConflictPolicyPreferRemote ConflictPolicy = "prefer_remote"
)

type ReconcileOptions struct {
	ConflictPolicy ConflictPolicy
}

func ReconcileProfiles(
	base map[string]interface{},
	local map[string]interface{},
	remote map[string]interface{},
	options ReconcileOptions,
) ReconcileReport {
	if options.ConflictPolicy == "" {
		options.ConflictPolicy = ConflictPolicyPreferLocal
	}
	report := ReconcileReport{
		Conflicts:           []string{},
		AcceptedRemotePaths: []string{},
		ReconciledProfile:   map[string]interface{}{},
	}
	report.ReconciledProfile = reconcileValue(base, local, remote, "", &report, options).(map[string]interface{})
	return report
}

func reconcileValue(
	base interface{},
	local interface{},
	remote interface{},
	path string,
	report *ReconcileReport,
	options ReconcileOptions,
) interface{} {
	baseMap, baseMapOK := asMap(base)
	localMap, localMapOK := asMap(local)
	remoteMap, remoteMapOK := asMap(remote)
	if baseMapOK || localMapOK || remoteMapOK {
		keys := unionKeys(baseMap, localMap, remoteMap)
		result := map[string]interface{}{}
		for _, key := range keys {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			result[key] = reconcileValue(baseMap[key], localMap[key], remoteMap[key], childPath, report, options)
		}
		return result
	}

	localChanged := !reflect.DeepEqual(local, base)
	remoteChanged := !reflect.DeepEqual(remote, base)

	if localChanged && remoteChanged && !reflect.DeepEqual(local, remote) {
		report.ConflictCount++
		report.Conflicts = append(report.Conflicts, path)
		if options.ConflictPolicy == ConflictPolicyPreferRemote {
			return remote
		}
		return local
	}

	if remoteChanged && !localChanged {
		report.AcceptedRemoteCount++
		report.AcceptedRemotePaths = append(report.AcceptedRemotePaths, path)
		return remote
	}

	return local
}

func asMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	default:
		return map[string]interface{}{}, false
	}
}

func unionKeys(maps ...map[string]interface{}) []string {
	seen := map[string]struct{}{}
	for _, current := range maps {
		for key := range current {
			seen[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nestedString(payload map[string]interface{}, path ...string) string {
	if len(path) == 0 {
		return ""
	}
	current := payload
	for idx, segment := range path {
		value, ok := current[segment]
		if !ok {
			return ""
		}
		if idx == len(path)-1 {
			text, _ := value.(string)
			return text
		}
		next, ok := value.(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

func deepCopyMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		out := map[string]interface{}{}
		for key, value := range in {
			out[key] = value
		}
		return out
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		out = map[string]interface{}{}
		for key, value := range in {
			out[key] = value
		}
	}
	return out
}
