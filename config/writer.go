package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const backupTimestampLayout = "20060102T150405.000000000Z"

func ReadConfigDocument(path string) ([]byte, *yaml.Node, error) {
	data, err := os.ReadFile(path)
	if nil != err && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	doc := &yaml.Node{}
	if 0 < len(bytes.TrimSpace(data)) {
		if unmarshalErr := yaml.Unmarshal(data, doc); nil != unmarshalErr {
			return nil, nil, fmt.Errorf("failed to parse config file %s: %w", path, unmarshalErr)
		}
	}
	if 0 == doc.Kind {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if 0 == len(doc.Content) || yaml.MappingNode != doc.Content[0].Kind {
		return nil, nil, fmt.Errorf("config file %s must hold a mapping at the top level", path)
	}
	return data, doc, nil
}

func SetDocumentValue(doc *yaml.Node, path string, value any) error {
	segments := strings.Split(path, ".")
	current := doc.Content[0]
	for index, segment := range segments {
		child := mappingChild(current, segment)
		if nil == value && index == len(segments)-1 {
			removeMappingChild(current, segment)
			return nil
		}
		if nil == child {
			child = &yaml.Node{Kind: yaml.ScalarNode}
			if index < len(segments)-1 {
				child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			}
			current.Content = append(current.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: segment}, child)
		}
		if index == len(segments)-1 {
			return child.Encode(value)
		}
		if yaml.MappingNode != child.Kind {
			return fmt.Errorf("%s is not a section", strings.Join(segments[:index+1], "."))
		}
		current = child
	}
	return nil
}

func mappingChild(mapping *yaml.Node, key string) *yaml.Node {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if key == mapping.Content[index].Value {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func RevisionOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func WriteConfigDocument(path string, original []byte, doc *yaml.Node, now time.Time) (string, string, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if encodeErr := encoder.Encode(doc); nil != encodeErr {
		return "", "", fmt.Errorf("failed to encode config: %w", encodeErr)
	}
	if closeErr := encoder.Close(); nil != closeErr {
		return "", "", fmt.Errorf("failed to encode config: %w", closeErr)
	}
	data := buf.Bytes()

	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); nil == statErr {
		mode = info.Mode().Perm()
	}

	backupPath := ""
	if 0 < len(original) {
		backupPath = path + ".bak." + now.UTC().Format(backupTimestampLayout)
		if backupErr := writeSynced(backupPath, original, mode); nil != backupErr {
			return "", "", fmt.Errorf("failed to write config backup: %w", backupErr)
		}
	}
	if writeErr := replaceFileAtomically(path, data, mode); nil != writeErr {
		return "", "", fmt.Errorf("failed to write config file: %w", writeErr)
	}
	return backupPath, RevisionOf(data), nil
}

func writeSynced(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if nil != err {
		return err
	}
	if _, writeErr := file.Write(data); nil != writeErr {
		_ = file.Close()
		return writeErr
	}
	if syncErr := file.Sync(); nil != syncErr {
		_ = file.Close()
		return syncErr
	}
	return file.Close()
}

func replaceFileAtomically(path string, data []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if nil != err {
		return err
	}
	tempPath := temp.Name()
	if _, writeErr := temp.Write(data); nil != writeErr {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return writeErr
	}
	if syncErr := temp.Sync(); nil != syncErr {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return syncErr
	}
	if closeErr := temp.Close(); nil != closeErr {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if chmodErr := os.Chmod(tempPath, mode); nil != chmodErr {
		_ = os.Remove(tempPath)
		return chmodErr
	}
	if renameErr := os.Rename(tempPath, path); nil != renameErr {
		_ = os.Remove(tempPath)
		return renameErr
	}
	return nil
}

func removeMappingChild(mapping *yaml.Node, key string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if key == mapping.Content[index].Value {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
}
