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

const latestDirectory = "latest"

func NewManifestStore(root string) ManifestStore {
	return ManifestStore{Root: root}
}

func (s ManifestStore) RunDir(runID string) string {
	return filepath.Join(s.Root, runID)
}

func (s ManifestStore) RunDirFor(manifest model.RunManifest) string {
	directory := manifest.ArtifactDirectory
	if strings.TrimSpace(directory) == "" {
		directory = manifest.RunID
	}
	return filepath.Join(s.Root, directory)
}

func ArtifactDirectoryName(provider string, createdAt time.Time) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "run"
	}
	return fmt.Sprintf("%s-run-%s", provider, createdAt.Format("15:04")+"_"+createdAt.Format("2.1.2006"))
}

func (s ManifestStore) Create(manifest *model.RunManifest) error {
	if manifest == nil || strings.TrimSpace(manifest.RunID) == "" {
		return errors.New("run ID is required")
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(manifest.ArtifactDirectory) == "" {
		manifest.ArtifactDirectory = ArtifactDirectoryName(manifest.Provider, manifest.CreatedAt)
	}
	manifest.UpdatedAt = manifest.CreatedAt
	if err := os.MkdirAll(s.RunDirFor(*manifest), 0o750); err != nil {
		return err
	}
	if err := s.writeJSON(filepath.Join(s.RunDirFor(*manifest), "manifest.json"), manifest); err != nil {
		return err
	}
	return s.writeStatus(*manifest)
}

func (s ManifestStore) Save(manifest *model.RunManifest) error {
	if manifest == nil || manifest.RunID == "" {
		return errors.New("run ID is required")
	}
	if strings.TrimSpace(manifest.ArtifactDirectory) == "" {
		manifest.ArtifactDirectory = ArtifactDirectoryName(manifest.Provider, manifest.CreatedAt)
	}
	manifest.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(s.RunDirFor(*manifest), 0o750); err != nil {
		return err
	}
	if err := s.writeJSON(filepath.Join(s.RunDirFor(*manifest), "manifest.json"), manifest); err != nil {
		return err
	}
	return s.writeStatus(*manifest)
}

func (s ManifestStore) Transition(manifest *model.RunManifest, phase model.Phase) error {
	if manifest == nil {
		return errors.New("manifest is nil")
	}
	if err := model.ValidateTransition(manifest.Phase, phase); err != nil {
		return err
	}
	manifest.Phase = phase
	return s.Save(manifest)
}

func (s ManifestStore) Load(runID string) (model.RunManifest, error) {
	directory, err := s.findRunDir(runID)
	if err != nil {
		return model.RunManifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
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
	latestPath := filepath.Join(s.Root, latestDirectory)
	info, err := os.Stat(latestPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		data, err := os.ReadFile(filepath.Join(latestPath, "manifest.json"))
		if err != nil {
			return "", err
		}
		var manifest model.RunManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return "", fmt.Errorf("decode latest manifest: %w", err)
		}
		if manifest.RunID == "" {
			return "", errors.New("latest manifest has no run ID")
		}
		return manifest.RunID, nil
	}
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return "", err
	}
	runID := strings.TrimSpace(string(data))
	if runID == "" {
		return "", errors.New("latest alias is empty")
	}
	return runID, nil
}

func (s ManifestStore) PublishLatest(manifest model.RunManifest) error {
	if manifest.Phase != model.PhaseCompleted {
		return fmt.Errorf("only completed runs can be published as latest")
	}
	source := s.RunDirFor(manifest)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("run artifact path is not a directory")
		}
		return fmt.Errorf("read completed run artifacts: %w", err)
	}
	if err := os.MkdirAll(s.Root, 0o750); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(s.Root, ".latest-staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := copyDirectory(source, staging); err != nil {
		return fmt.Errorf("stage latest artifacts: %w", err)
	}
	latestPath := filepath.Join(s.Root, latestDirectory)
	backupPath := ""
	if _, err := os.Stat(latestPath); err == nil {
		file, err := os.CreateTemp(s.Root, ".latest-backup-")
		if err != nil {
			return err
		}
		backupPath = file.Name()
		if err := file.Close(); err != nil {
			return err
		}
		if err := os.Remove(backupPath); err != nil {
			return err
		}
		if err := os.Rename(latestPath, backupPath); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, latestPath); err != nil {
		if backupPath != "" {
			_ = os.Rename(backupPath, latestPath)
		}
		return err
	}
	if backupPath != "" {
		if err := os.RemoveAll(backupPath); err != nil {
			return err
		}
	}
	return nil
}

func (s ManifestStore) findRunDir(runID string) (string, error) {
	legacy := s.RunDir(runID)
	if info, err := os.Stat(filepath.Join(legacy, "manifest.json")); err == nil && !info.IsDir() {
		return legacy, nil
	}
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == latestDirectory || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Root, entry.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var manifest model.RunManifest
		if json.Unmarshal(data, &manifest) == nil && manifest.RunID == runID {
			return filepath.Join(s.Root, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("run %s not found: %w", runID, os.ErrNotExist)
}

func (s ManifestStore) writeStatus(manifest model.RunManifest) error {
	status := struct {
		RunID     string         `json:"runID"`
		Phase     model.Phase    `json:"phase"`
		UpdatedAt time.Time      `json:"updatedAt"`
		Failure   *model.Failure `json:"failure,omitempty"`
	}{manifest.RunID, manifest.Phase, manifest.UpdatedAt, manifest.Failure}
	if err := s.writeJSON(filepath.Join(s.RunDirFor(manifest), "status.json"), status); err != nil {
		return err
	}
	return nil
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := destination
		if relative != "." {
			target = filepath.Join(destination, relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o640)
	})
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
