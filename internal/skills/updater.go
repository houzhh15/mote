// Package skills provides the skill system for Mote Agent Runtime.
package skills

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog"
)

// SkillVersionInfo represents version information for a skill.
type SkillVersionInfo struct {
	SkillID         string    `json:"skill_id"`
	IsBuiltin       bool      `json:"is_builtin"`
	LocalVersion    string    `json:"local_version"`    // Empty if not installed locally
	EmbedVersion    string    `json:"embed_version"`    // Version in embed.FS
	UpdateAvailable bool      `json:"update_available"` // True if embed > local
	LocalModified   bool      `json:"local_modified"`   // True if local files modified
	CheckedAt       time.Time `json:"checked_at"`       // When checked
}

// VersionCheckResult represents the result of checking all builtin skills.
type VersionCheckResult struct {
	TotalChecked     int                  `json:"total_checked"`
	UpdatesAvailable []SkillVersionInfo   `json:"updates_available"`
	Errors           []VersionCheckError  `json:"errors,omitempty"`
}

// VersionCheckError represents an error during version checking.
type VersionCheckError struct {
	SkillID string `json:"skill_id"`
	Error   string `json:"error"`
}

// UpdateOptions represents options for updating a skill.
type UpdateOptions struct {
	Force      bool `json:"force"`       // Force update even if local modified
	Backup     bool `json:"backup"`      // Create backup before update (default true)
	SkipReload bool `json:"skip_reload"` // Skip reload after update (for testing)
}

// UpdateResult represents the result of updating a skill.
type UpdateResult struct {
	SkillID    string `json:"skill_id"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	BackupPath string `json:"backup_path,omitempty"`
	Reloaded   bool   `json:"reloaded"`
	Duration   string `json:"duration"`
}

// VersionChecker checks builtin skill versions.
type VersionChecker struct {
	embedFS    embed.FS
	builtinIDs []string
	cache      map[string]*SkillVersionInfo
	cacheMu    sync.RWMutex
	lastCheck  time.Time
	logger     zerolog.Logger
}

// NewVersionChecker creates a new VersionChecker.
func NewVersionChecker(embedFS embed.FS, logger zerolog.Logger) *VersionChecker {
	return &VersionChecker{
		embedFS:    embedFS,
		builtinIDs: ListBuiltinSkills(),
		cache:      make(map[string]*SkillVersionInfo),
		logger:     logger,
	}
}

// CheckAllVersions checks all builtin skills concurrently.
func (vc *VersionChecker) CheckAllVersions(skillsDir string) (*VersionCheckResult, error) {
	result := &VersionCheckResult{
		UpdatesAvailable: make([]SkillVersionInfo, 0),
		Errors:           make([]VersionCheckError, 0),
	}

	var wg sync.WaitGroup
	results := make(chan SkillVersionInfo, len(vc.builtinIDs))
	errs := make(chan VersionCheckError, len(vc.builtinIDs))

	for _, skillID := range vc.builtinIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			info, err := vc.CheckVersion(id, skillsDir)
			if err != nil {
				errs <- VersionCheckError{SkillID: id, Error: err.Error()}
				return
			}
			results <- *info
		}(skillID)
	}

	wg.Wait()
	close(results)
	close(errs)

	for info := range results {
		if info.UpdateAvailable {
			result.UpdatesAvailable = append(result.UpdatesAvailable, info)
		}
		vc.updateCache(&info)
		result.TotalChecked++
	}

	for err := range errs {
		result.Errors = append(result.Errors, err)
	}

	vc.lastCheck = time.Now()
	return result, nil
}

// CheckVersion checks a single skill's version.
func (vc *VersionChecker) CheckVersion(skillID string, skillsDir string) (*SkillVersionInfo, error) {
	info := &SkillVersionInfo{
		SkillID:   skillID,
		IsBuiltin: true,
		CheckedAt: time.Now(),
	}

	// Read embed version
	embedManifest, err := vc.readEmbedManifest(skillID)
	if err != nil {
		return nil, fmt.Errorf("read embed manifest: %w", err)
	}
	info.EmbedVersion = embedManifest.Version

	// Read local version
	localPath := filepath.Join(skillsDir, skillID, "manifest.json")
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		// Local not installed, needs install
		info.LocalVersion = ""
		info.UpdateAvailable = true
		return info, nil
	}

	localManifest, err := ParseManifest(localPath)
	if err != nil {
		return nil, fmt.Errorf("read local manifest: %w", err)
	}
	info.LocalVersion = localManifest.Version

	// Compare versions
	updateAvailable, err := vc.compareVersions(info.EmbedVersion, info.LocalVersion)
	if err != nil {
		return nil, fmt.Errorf("compare versions: %w", err)
	}
	info.UpdateAvailable = updateAvailable

	// Check if local modified (placeholder for now)
	info.LocalModified = false

	return info, nil
}

// compareVersions compares two versions, returns true if embedVer > localVer.
func (vc *VersionChecker) compareVersions(embedVer, localVer string) (bool, error) {
	embedV, err := semver.NewVersion(embedVer)
	if err != nil {
		return false, fmt.Errorf("invalid embed version %s: %w", embedVer, err)
	}

	localV, err := semver.NewVersion(localVer)
	if err != nil {
		return false, fmt.Errorf("invalid local version %s: %w", localVer, err)
	}

	return embedV.GreaterThan(localV), nil
}

// readEmbedManifest reads manifest from embed.FS.
func (vc *VersionChecker) readEmbedManifest(skillID string) (*Skill, error) {
	path := fmt.Sprintf("skills/%s/manifest.json", skillID)
	data, err := vc.embedFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifestBytes(data)
}

// updateCache updates the version cache.
func (vc *VersionChecker) updateCache(info *SkillVersionInfo) {
	vc.cacheMu.Lock()
	defer vc.cacheMu.Unlock()
	vc.cache[info.SkillID] = info
}

// GetCached returns cached version info.
func (vc *VersionChecker) GetCached(skillID string) (*SkillVersionInfo, bool) {
	vc.cacheMu.RLock()
	defer vc.cacheMu.RUnlock()
	info, ok := vc.cache[skillID]
	return info, ok
}

// BackupManager manages skill backups.
type BackupManager struct {
	skillsDir  string
	backupDir  string
	maxBackups int
	logger     zerolog.Logger
}

// NewBackupManager creates a new BackupManager.
func NewBackupManager(skillsDir string, maxBackups int, logger zerolog.Logger) *BackupManager {
	backupDir := filepath.Join(skillsDir, ".backup")
	return &BackupManager{
		skillsDir:  skillsDir,
		backupDir:  backupDir,
		maxBackups: maxBackups,
		logger:     logger,
	}
}

// Backup creates a backup of a skill directory.
func (bm *BackupManager) Backup(skillID string, version string) (string, error) {
	if err := os.MkdirAll(bm.backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s_v%s_%s", skillID, version, timestamp)
	backupPath := filepath.Join(bm.backupDir, backupName)

	srcPath := filepath.Join(bm.skillsDir, skillID)
	if err := copyDir(srcPath, backupPath); err != nil {
		return "", fmt.Errorf("copy directory: %w", err)
	}

	bm.logger.Info().Str("skill", skillID).Str("backup", backupPath).Msg("Backup created")

	bm.cleanupOldBackups(skillID)

	return backupPath, nil
}

// Restore restores from a backup.
func (bm *BackupManager) Restore(backupPath string, skillID string) error {
	targetPath := filepath.Join(bm.skillsDir, skillID)

	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("remove current dir: %w", err)
	}

	if err := copyDir(backupPath, targetPath); err != nil {
		return fmt.Errorf("restore from backup: %w", err)
	}

	bm.logger.Info().Str("skill", skillID).Str("from", backupPath).Msg("Restored from backup")
	return nil
}

// cleanupOldBackups removes old backups, keeping only maxBackups.
func (bm *BackupManager) cleanupOldBackups(skillID string) {
	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		bm.logger.Warn().Err(err).Msg("Failed to read backup dir")
		return
	}

	var backups []fs.DirEntry
	prefix := skillID + "_v"
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			backups = append(backups, entry)
		}
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name() > backups[j].Name()
	})

	if len(backups) > bm.maxBackups {
		for _, entry := range backups[bm.maxBackups:] {
			path := filepath.Join(bm.backupDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				bm.logger.Warn().Err(err).Str("path", path).Msg("Failed to remove old backup")
			} else {
				bm.logger.Debug().Str("path", path).Msg("Removed old backup")
			}
		}
	}
}

// SkillUpdater updates builtin skills.
type SkillUpdater struct {
	embedFS        embed.FS
	skillsDir      string
	versionChecker *VersionChecker
	backupManager  *BackupManager
	skillManager   *Manager
	updateMu       sync.Mutex
	logger         zerolog.Logger
}

// NewSkillUpdater creates a new SkillUpdater.
func NewSkillUpdater(
	embedFS embed.FS,
	skillsDir string,
	versionChecker *VersionChecker,
	skillManager *Manager,
	logger zerolog.Logger,
) *SkillUpdater {
	return &SkillUpdater{
		embedFS:        embedFS,
		skillsDir:      skillsDir,
		versionChecker: versionChecker,
		backupManager:  NewBackupManager(skillsDir, 5, logger),
		skillManager:   skillManager,
		logger:         logger,
	}
}

// UpdateSkill updates a single builtin skill.
func (su *SkillUpdater) UpdateSkill(skillID string, opts UpdateOptions) (*UpdateResult, error) {
	su.updateMu.Lock()
	defer su.updateMu.Unlock()

	startTime := time.Now()
	result := &UpdateResult{SkillID: skillID}

	if !isBuiltinSkill(skillID) {
		return nil, ErrNotBuiltinSkill
	}

	versionInfo, err := su.versionChecker.CheckVersion(skillID, su.skillsDir)
	if err != nil {
		return nil, fmt.Errorf("check version: %w", err)
	}

	if !versionInfo.UpdateAvailable {
		return nil, ErrNoUpdateAvailable
	}

	result.OldVersion = versionInfo.LocalVersion
	result.NewVersion = versionInfo.EmbedVersion

	if versionInfo.LocalModified && !opts.Force {
		return nil, ErrLocalModified
	}

	if opts.Backup {
		backupPath, err := su.backupManager.Backup(skillID, versionInfo.LocalVersion)
		if err != nil {
			su.logger.Warn().Err(err).Str("skill", skillID).Msg("Backup failed, continuing update")
		} else {
			result.BackupPath = backupPath
		}
	}

	if err := su.overwriteSkillFiles(skillID); err != nil {
		if opts.Backup && result.BackupPath != "" {
			if restoreErr := su.backupManager.Restore(result.BackupPath, skillID); restoreErr != nil {
				su.logger.Error().Err(restoreErr).Msg("Rollback failed")
			}
		}
		return nil, fmt.Errorf("overwrite files: %w", err)
	}

	if !opts.SkipReload {
		if err := su.reloadSkill(skillID); err != nil {
			su.logger.Warn().Err(err).Str("skill", skillID).Msg("Reload skill failed")
			result.Reloaded = false
		} else {
			result.Reloaded = true
		}
	}

	result.Duration = time.Since(startTime).String()
	su.logger.Info().
		Str("skill", skillID).
		Str("old", result.OldVersion).
		Str("new", result.NewVersion).
		Str("duration", result.Duration).
		Msg("Skill updated successfully")

	return result, nil
}

// overwriteSkillFiles copies files from embed.FS to local directory.
func (su *SkillUpdater) overwriteSkillFiles(skillID string) error {
	embedRoot := "skills/" + skillID
	localRoot := filepath.Join(su.skillsDir, skillID)

	if err := os.MkdirAll(localRoot, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	err := fs.WalkDir(su.embedFS, embedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == embedRoot {
			return nil
		}

		relPath, err := filepath.Rel(embedRoot, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(localRoot, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := su.embedFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embed file %s: %w", path, err)
		}

		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("write file %s: %w", targetPath, err)
		}

		return nil
	})

	return err
}

// reloadSkill reloads a skill (deactivate, reload, reactivate if needed).
func (su *SkillUpdater) reloadSkill(skillID string) error {
	statuses := su.skillManager.ListSkills()
	var wasActive bool
	for _, status := range statuses {
		if status.Skill != nil && status.Skill.ID == skillID {
			wasActive = status.State == SkillStateActive
			break
		}
	}

	if wasActive {
		if err := su.skillManager.Deactivate(skillID); err != nil {
			return fmt.Errorf("deactivate: %w", err)
		}
	}

	if err := su.skillManager.ReloadSkill(skillID); err != nil {
		return fmt.Errorf("reload: %w", err)
	}

	if wasActive {
		if err := su.skillManager.Activate(skillID, nil); err != nil {
			return fmt.Errorf("reactivate: %w", err)
		}
	}

	return nil
}

// isBuiltinSkill checks if a skill ID is a builtin skill.
func isBuiltinSkill(skillID string) bool {
	builtinIDs := ListBuiltinSkills()
	for _, id := range builtinIDs {
		if id == skillID {
			return true
		}
	}
	return false
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, info.Mode())
	})
}

// Errors
var (
	ErrNotBuiltinSkill   = errors.New("skill is not a builtin skill")
	ErrNoUpdateAvailable = errors.New("no update available")
	ErrLocalModified     = errors.New("local files have been modified")
)
