package handlers

import (
	"testing"
)

func TestPathSanitization(t *testing.T) {
	maliciousPaths := []struct {
		input           string
		shouldBeBlocked bool
	}{
		{"normal/file.txt", false},
		{"../../../etc/passwd", true},
		{"/etc/passwd", false},
		{"..\\..\\..\\windows\\system32", true},
		{"dir/../../../etc/passwd", true},
		{"./file.txt", false},
		{"", false},
	}

	for _, test := range maliciousPaths {
		t.Run("path_"+test.input, func(t *testing.T) {
			pathInfo := ParseFilePath(test.input)

			if test.shouldBeBlocked {
				if !pathInfo.IsInvalid {
					t.Errorf("malicious path %s was not marked invalid", test.input)
				}
			} else if pathInfo.IsInvalid {
				t.Errorf("safe path %s was marked invalid", test.input)
			}

			if pathInfo.FullPath == "" && test.input != "" {
				t.Errorf("path sanitization resulted in empty path for input: %s", test.input)
			}
		})
	}
}
