package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redhat-appstudio/konflux-test/internal/model"
)

type ManifestStore struct {
	Root string
}

func NewManifestStore(root string) ManifestStore {
	return ManifestStore{Root: root}
}

func (s ManifestStore) RunDir(runID string) string {
	return filepath.Join(s.Root, runID)
}

func (s ManifestStore) Create(manifest model.RunManifest) error {
	if strings.TrimSpace(manifest.RunID) == "" {
		return errors.New("run ID is required")
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	manifest.UpdatedAt = manifest.CreatedAt
	if err := os.MkdirAll(s.RunDir(manifest.RunID), 0o750); err != nil {
		return err
	}
	if err := s.writeJSON(filepath.Join(s.RunDir(manifest.RunID), "manifest.json"), manifest); err != nil {
		return err
	}
	return s.writeStatus(manifest)
}

func (s ManifestStore) Save(manifest model.RunManifest) error {
	if manifest.RunID == "" {
		return errors.New("run ID is required")
	}
	manifest.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(s.RunDir(manifest.RunID), 0o750); err != nil {
		return err
	}
	if err := s.writeJSON(filepath.Join(s.RunDir(manifest.RunID), "manifest.json"), manifest); err != nil {
		return err
	}
	return s.writeStatus(manifest)
}

func (s ManifestStore) Transition(manifest *model.RunManifest, phase model.Phase) error {
	if manifest == nil {
		return errors.New("manifest is nil")
	}
	if err := model.ValidateTransition(manifest.Phase, phase); err != nil {
		return err
	}
	manifest.Phase = phase
	return s.Save(*manifest)
}

func (s ManifestStore) Load(runID string) (model.RunManifest, error) {
	data, err := os.ReadFile(filepath.Join(s.RunDir(runID), "manifest.json"))
	if err != nil {
		return model.RunManifest{}, err
	}
	var manifest model.RunManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return model.RunManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

func (s ManifestStore) Latest() (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Root, "latest"))
	if err != nil {
		return "", err
	}
	runID := strings.TrimSpace(string(data))
	if runID == "" {
		return "", errors.New("latest alias is empty")
	}
	return runID, nil
}

func (s ManifestStore) writeStatus(manifest model.RunManifest) error {
	status := struct {
		RunID     string         `json:"runID"`
		Phase     model.Phase    `json:"phase"`
		UpdatedAt time.Time      `json:"updatedAt"`
		Failure   *model.Failure `json:"failure,omitempty"`
	}{manifest.RunID, manifest.Phase, manifest.UpdatedAt, manifest.Failure}
	if err := s.writeJSON(filepath.Join(s.RunDir(manifest.RunID), "status.json"), status); err != nil {
		return err
	}
	return s.atomicWrite(filepath.Join(s.Root, "latest"), []byte(manifest.RunID+"\n"), 0o640)
}

func (s ManifestStore) writeJSON(path string, value any) error {
	data, err := RedactedJSON(value)
	if err != nil {
		return err
	}
	return s.atomicWrite(path, append(data, '\n'), 0o640)
}

func (s ManifestStore) atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".konflux-test-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
