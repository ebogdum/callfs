package config

import "testing"

func TestConfigSchemaCoversEveryAppConfigKey(t *testing.T) {
	registered := make(map[string]bool)
	for _, key := range Schema() {
		registered[key.Path] = true
	}
	for path := range Flatten(AppConfig{}) {
		if !registered[path] {
			t.Errorf("koanf key %s is missing from the config schema", path)
		}
	}
}
