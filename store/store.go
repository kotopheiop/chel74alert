package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gofrs/flock"

	"chel74alert/models"
)

const seenTTL = 14 * 24 * time.Hour

type seenMap map[string]int64

func (m *seenMap) UnmarshalJSON(data []byte) error {
	var times map[string]int64
	if err := json.Unmarshal(data, &times); err == nil {
		*m = times
		return nil
	}
	var flags map[string]bool
	if err := json.Unmarshal(data, &flags); err != nil {
		return err
	}
	now := time.Now().Unix()
	out := make(map[string]int64, len(flags))
	for k, ok := range flags {
		if ok {
			out[k] = now
		}
	}
	*m = out
	return nil
}

type Store struct {
	mu             sync.Mutex
	path           string
	lock           *flock.Flock
	Subscribers    map[int64]bool `json:"subscribers"`
	Seen           seenMap        `json:"seen"`
	Seeded         bool           `json:"seeded"`
	LastNotifyKind string         `json:"last_notify_kind"`
	LastNotifyAt   time.Time      `json:"last_notify_at"`
	Last           *models.Alert  `json:"last_alert,omitempty"`
	Pending        []models.Alert `json:"pending,omitempty"`
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	lock := flock.New(filepath.Join(dir, "state.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("хранилище уже используется другим процессом")
	}

	s := &Store{
		path:        filepath.Join(dir, "state.json"),
		lock:        lock,
		Subscribers: make(map[int64]bool),
		Seen:        make(seenMap),
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		_ = lock.Unlock()
		return nil, err
	}
	if err := json.Unmarshal(raw, s); err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	if s.Subscribers == nil {
		s.Subscribers = make(map[int64]bool)
	}
	if s.Seen == nil {
		s.Seen = make(seenMap)
	}
	s.lock = lock
	s.path = filepath.Join(dir, "state.json")
	return s, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	err := s.lock.Unlock()
	s.lock = nil
	return err
}

func (s *Store) saveLocked() error {
	s.pruneSeenLocked(time.Now())
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) pruneSeenLocked(now time.Time) {
	cutoff := now.Add(-seenTTL).Unix()
	for id, ts := range s.Seen {
		if ts < cutoff {
			delete(s.Seen, id)
		}
	}
}

func (s *Store) Subscribe(chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Subscribers[chatID] = true
	return s.saveLocked()
}

func (s *Store) Unsubscribe(chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Subscribers, chatID)
	return s.saveLocked()
}

func (s *Store) IsSubscribed(chatID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Subscribers[chatID]
}

func (s *Store) ListSubscribers() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int64, 0, len(s.Subscribers))
	for id, ok := range s.Subscribers {
		if ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Store) MarkSeen(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	for _, id := range ids {
		if id == "" {
			continue
		}
		s.Seen[id] = now
	}
	s.Seeded = true
	return s.saveLocked()
}

func (s *Store) FilterNew(ids []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, id := range ids {
		if _, ok := s.Seen[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

func (s *Store) LastAlert() *models.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Last == nil {
		return nil
	}
	cp := *s.Last
	return &cp
}

func (s *Store) LastKind() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastNotifyKind
}

func (s *Store) LastNotifiedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastNotifyAt
}

func (s *Store) LastClearTime() time.Time {
	a := s.LastAlert()
	if a == nil || a.Kind != models.KindClear {
		return time.Time{}
	}
	if !a.Published.IsZero() {
		return a.Published
	}
	return s.LastNotifiedAt()
}

func (s *Store) ActiveDanger() *models.Alert {
	a := s.LastAlert()
	if a == nil || a.Kind != models.KindDanger {
		return nil
	}
	if !looksLikeAlert(a.Title + " " + a.Summary) {
		return nil
	}
	return a
}

func (s *Store) HydrateLast(a models.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Last != nil {
		return nil
	}
	cp := a
	s.Last = &cp
	return s.saveLocked()
}

func (s *Store) ShouldNotify(kind, title string, cooldown time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	lastTitle := ""
	if s.Last != nil {
		lastTitle = s.Last.Title
	}
	return shouldNotify(s.LastNotifyKind, s.LastNotifyAt, lastTitle, kind, title, cooldown)
}

func (s *Store) NextToNotify(alerts []models.Alert, cooldown time.Duration) []models.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind := s.LastNotifyKind
	at := s.LastNotifyAt
	title := ""
	if s.Last != nil {
		title = s.Last.Title
	}
	var out []models.Alert
	for _, a := range alerts {
		if !shouldNotify(kind, at, title, string(a.Kind), a.Title, cooldown) {
			continue
		}
		out = append(out, a)
		kind = string(a.Kind)
		at = time.Now()
		title = a.Title
	}
	return out
}

func shouldNotify(lastKind string, lastAt time.Time, lastTitle, kind, title string, cooldown time.Duration) bool {
	if lastKind != kind {
		return true
	}
	if lastAt.IsZero() {
		return true
	}
	if !looksLikeAlert(lastTitle) {
		return true
	}
	if similarTitle(lastTitle, title) {
		return false
	}
	return time.Since(lastAt) > cooldown
}

func looksLikeAlert(s string) bool {
	s = foldTitle(s)
	return strings.Contains(s, "опасност") ||
		strings.Contains(s, "режим") ||
		strings.Contains(s, "отбой") ||
		strings.Contains(s, "угроза миновала")
}

func similarTitle(a, b string) bool {
	a = foldTitle(a)
	b = foldTitle(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if !looksLikeAlert(a) || !looksLikeAlert(b) {
		return false
	}
	if ca, cb := threatClass(a), threatClass(b); ca != "" && ca == cb {
		return true
	}
	if (runeCount(b) >= 12 && strings.Contains(a, b)) || (runeCount(a) >= 12 && strings.Contains(b, a)) {
		return true
	}
	ta, tb := significantTokens(a), significantTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(ta))
	for _, t := range ta {
		set[t] = struct{}{}
	}
	inter := 0
	for _, t := range tb {
		if _, ok := set[t]; ok {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	return union > 0 && float64(inter)/float64(union) >= 0.5
}

func threatClass(s string) string {
	uav := strings.Contains(s, "бпла") ||
		strings.Contains(s, "беспилот") ||
		strings.Contains(s, "дрон") ||
		strings.Contains(s, "шахед") ||
		strings.Contains(s, "бвс")
	missile := strings.Contains(s, "ракет")
	switch {
	case uav && missile:
		return "both"
	case uav:
		return "uav"
	case missile:
		return "missile"
	default:
		return ""
	}
}

func foldTitle(s string) string {
	var b strings.Builder
	space := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func significantTokens(s string) []string {
	var out []string
	for _, tok := range strings.Fields(s) {
		if runeCount(tok) >= 4 {
			out = append(out, tok)
		}
	}
	return out
}

func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func (s *Store) RecordNotify(a models.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := a
	s.Last = &cp
	s.LastNotifyKind = string(a.Kind)
	s.LastNotifyAt = time.Now()
	return s.saveLocked()
}

func (s *Store) CommitPoll(seenIDs []string, notify []models.Alert, hydrate *models.Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	unix := now.Unix()
	for _, id := range seenIDs {
		if id == "" {
			continue
		}
		s.Seen[id] = unix
	}
	s.Seeded = true
	for _, a := range notify {
		if pendingHasLocked(s.Pending, a.ID) {
			continue
		}
		cp := a
		s.Pending = append(s.Pending, cp)
		s.Last = &cp
		s.LastNotifyKind = string(a.Kind)
		s.LastNotifyAt = now
	}
	if s.Last == nil && hydrate != nil {
		cp := *hydrate
		s.Last = &cp
	}
	return s.saveLocked()
}

func pendingHasLocked(pending []models.Alert, id string) bool {
	for _, a := range pending {
		if a.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) PendingAlerts() []models.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.Alert, len(s.Pending))
	copy(out, s.Pending)
	return out
}

func (s *Store) AckPending(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.Pending {
		if a.ID == id {
			s.Pending = append(s.Pending[:i], s.Pending[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}
