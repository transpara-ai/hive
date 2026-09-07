package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// The caller reads the externally pinned workflow once at startup. Both peer
// hosts receive the same bytes without relying on host-specific installation
// discovery. No release identity or workflow body is copied into Hive.
func loadRoutingContext(lockPath, skillPath string) (string, error) {
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		return "", fmt.Errorf("read external workflow lock: %w", err)
	}
	var lock struct {
		SkillSHA256 string `json:"skill_sha256"`
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		return "", errors.New("invalid external workflow lock")
	}
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("read externally pinned workflow: %w", err)
	}
	digest := sha256.Sum256(skill)
	if len(skill) == 0 || len(skill) > 128*1024 || hex.EncodeToString(digest[:]) != lock.SkillSHA256 {
		return "", errors.New("external workflow bytes do not match the pinned digest or size limit")
	}
	return string(skill), nil
}
