package config

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type Source string

const (
	SourceDefault Source = "default"
	SourceFile    Source = "file"
	SourceEnv     Source = "env"
	SourceRuntime Source = "runtime"
)

type SourceMap map[string]Source

func CaptureSources(configFilePath string) (SourceMap, error) {
	fileValues := map[string]any{}
	if "" != configFilePath {
		data, err := os.ReadFile(configFilePath)
		if nil != err {
			return nil, fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
		}
		if unmarshalErr := yaml.Unmarshal(data, &fileValues); nil != unmarshalErr {
			return nil, fmt.Errorf("failed to parse config file %s: %w", configFilePath, unmarshalErr)
		}
	}

	envNames := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		envNames = append(envNames, name)
	}

	sources := make(SourceMap, len(keySpecs)+1)
	for _, key := range Schema() {
		sources[key.Path] = sourceFor(key, fileValues, envNames)
	}
	return sources, nil
}

func sourceFor(key SchemaKey, fileValues map[string]any, envNames []string) Source {
	for _, name := range envNames {
		if name == key.Env || strings.HasPrefix(name, key.Env+"__") {
			return SourceEnv
		}
	}
	if _, ok := LookupNested(fileValues, key.Path); ok {
		return SourceFile
	}
	return SourceDefault
}

func KeysFromEnv(sources SourceMap) []string {
	return keysWithSource(sources, SourceEnv)
}

func KeysAtDefault(sources SourceMap) []string {
	return keysWithSource(sources, SourceDefault)
}

func keysWithSource(sources SourceMap, source Source) []string {
	keys := make([]string, 0, len(sources))
	for key, value := range sources {
		if source == value {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func LookupNested(values map[string]any, path string) (any, bool) {
	var current any = values
	for _, segment := range strings.Split(path, ".") {
		section, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, found := section[segment]
		if !found {
			return nil, false
		}
		current = next
	}
	return current, true
}
