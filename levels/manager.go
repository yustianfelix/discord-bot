package levels

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// UserXP holds XP and level information for a user in a guild.
type UserXP struct {
	UserID      string `json:"user_id"`
	XP          int    `json:"xp"`
	Level       int    `json:"level"`
	LastAwarded int64  `json:"last_awarded_unix"`
}

// GuildState stores all users in a guild.
type GuildState struct {
	Users map[string]*UserXP `json:"users"`
}

// Manager handles per-guild leveling and persistence.
type Manager struct {
	mu            sync.Mutex
	data          map[string]*GuildState
	path          string
	awardCooldown time.Duration
	rnd           *rand.Rand
}

// NewManager creates a leveling manager and loads data from disk if available.
func NewManager(path string) (*Manager, error) {
	m := &Manager{
		data:          make(map[string]*GuildState),
		path:          path,
		awardCooldown: 60 * time.Second,
		rnd:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if err := m.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return m, nil
}

// AddMessage awards experience for a user message and enforces a cooldown.
func (m *Manager) AddMessage(ctx context.Context, guildID, userID string) (int, bool, int, error) {
	select {
	case <-ctx.Done():
		return 0, false, 0, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	gs := m.data[guildID]
	if gs == nil {
		gs = &GuildState{Users: make(map[string]*UserXP)}
		m.data[guildID] = gs
	}

	user := gs.Users[userID]
	if user == nil {
		user = &UserXP{UserID: userID}
		gs.Users[userID] = user
	}

	if time.Since(time.Unix(user.LastAwarded, 0)) < m.awardCooldown {
		return 0, false, user.Level, nil
	}

	xp := m.rnd.Intn(6) + 5
	user.XP += xp
	user.LastAwarded = time.Now().Unix()

	newLevel := levelFromXP(user.XP)
	leveledUp := newLevel > user.Level
	if leveledUp {
		user.Level = newLevel
	}

	if err := m.saveLocked(); err != nil {
		return xp, leveledUp, newLevel, err
	}

	return xp, leveledUp, newLevel, nil
}

// GetUser returns the stored XP/level entry for a user in a guild.
func (m *Manager) GetUser(guildID, userID string) (UserXP, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	gs, ok := m.data[guildID]
	if !ok {
		return UserXP{}, false
	}
	user, ok := gs.Users[userID]
	if !ok || user == nil {
		return UserXP{}, false
	}
	return *user, true
}

// LeaderboardEntry is a single leaderboard item.
type LeaderboardEntry struct {
	UserID string
	XP     int
	Level  int
}

// GetLeaderboard returns the top users by XP for the guild.
func (m *Manager) GetLeaderboard(guildID string, limit int) []LeaderboardEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	gs, ok := m.data[guildID]
	if !ok || gs == nil {
		return nil
	}

	entries := make([]LeaderboardEntry, 0, len(gs.Users))
	for _, user := range gs.Users {
		if user == nil {
			continue
		}
		entries = append(entries, LeaderboardEntry{
			UserID: user.UserID,
			XP:     user.XP,
			Level:  user.Level,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].XP == entries[j].XP {
			if entries[i].Level == entries[j].Level {
				return entries[i].UserID < entries[j].UserID
			}
			return entries[i].Level > entries[j].Level
		}
		return entries[i].XP > entries[j].XP
	})

	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	if limit == 0 {
		return nil
	}

	return entries[:limit]
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}

	tmpPath := m.path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(m.data); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, m.path)
}

func (m *Manager) load() error {
	file, err := os.Open(m.path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&m.data); err != nil {
		return err
	}
	if m.data == nil {
		m.data = make(map[string]*GuildState)
	}
	return nil
}

// xpForLevel is the XP needed for the next level threshold.
func xpForLevel(level int) int {
	return 5*level*level + 50*level + 100
}

func levelFromXP(xp int) int {
	level := 0
	for xp >= xpForLevel(level+1) {
		level++
	}
	return level
}
