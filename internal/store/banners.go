package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Banners are announcements the admins publish on the sign-in page and in
// the app, for everyone, the admins or a group.

// Banner kinds, audiences and the longest text.
const (
	BannerInfo     = "info"
	BannerWarning  = "warning"
	BannerCritical = "critical"

	AudienceEveryone = "everyone"
	AudienceAdmins   = "admins"
	AudienceGroup    = "group"

	MaxBannerRunes = 1000
)

// ErrNotDismissible refuses closing a banner that must stay.
var ErrNotDismissible = errors.New("this banner cannot be closed")

// Banner is an announcement. StartsAt and EndsAt are zero when open.
type Banner struct {
	ID          int64     `json:"id"`
	Message     string    `json:"message"`
	Kind        string    `json:"kind"`
	OnLogin     bool      `json:"onLogin"`
	InApp       bool      `json:"inApp"`
	Audience    string    `json:"audience"`
	GroupID     int64     `json:"groupId,omitempty"`
	GroupName   string    `json:"groupName,omitempty"`
	Dismissible bool      `json:"dismissible"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt      time.Time `json:"endsAt"`
	// Revision grows with every change of the text; a closing counts for
	// one revision.
	Revision  int64     `json:"revision"`
	CreatedBy *Person   `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const bannerCols = `b.id, b.message, b.kind, b.on_login, b.in_app, b.audience, COALESCE(b.group_id, 0), COALESCE(g.name, ''),
	b.dismissible, b.starts_at, b.ends_at, b.revision, COALESCE(u.id, 0), COALESCE(u.username, ''), COALESCE(u.display_name, ''),
	b.created_at, b.updated_at`

const bannerFrom = ` FROM banners b LEFT JOIN groups g ON g.id = b.group_id LEFT JOIN users u ON u.id = b.created_by`

func scanBanner(row interface{ Scan(...any) error }) (Banner, error) {
	var (
		b                              Banner
		starts, ends, created, updated int64
		p                              Person
	)
	err := row.Scan(&b.ID, &b.Message, &b.Kind, &b.OnLogin, &b.InApp, &b.Audience, &b.GroupID, &b.GroupName,
		&b.Dismissible, &starts, &ends, &b.Revision, &p.ID, &p.Username, &p.DisplayName, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	b.StartsAt, b.EndsAt = unixOrZero(starts), unixOrZero(ends)
	b.CreatedAt, b.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	if p.ID != 0 {
		b.CreatedBy = &p
	}
	return b, err
}

func (s *Store) queryBanners(ctx context.Context, where string, args ...any) ([]Banner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+bannerCols+bannerFrom+` `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Banner{}
	for rows.Next() {
		b, err := scanBanner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func unixOf(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// bannerOrder: critical first, then the earliest start, then the oldest.
const bannerOrder = ` ORDER BY CASE b.kind WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, b.starts_at, b.id`

// Banners lists every banner (administration), newest first.
func (s *Store) Banners(ctx context.Context) ([]Banner, error) {
	return s.queryBanners(ctx, `ORDER BY b.id DESC`)
}

// BannerByID returns one banner.
func (s *Store) BannerByID(ctx context.Context, id int64) (Banner, error) {
	return scanBanner(s.db.QueryRowContext(ctx, `SELECT `+bannerCols+bannerFrom+` WHERE b.id = ?`, id))
}

// CreateBanner stores a new banner (checked by the caller).
func (s *Store) CreateBanner(ctx context.Context, b Banner, by int64) (Banner, error) {
	now := s.unixNow()
	res, err := s.db.ExecContext(ctx, `INSERT INTO banners
		(message, kind, on_login, in_app, audience, group_id, dismissible, starts_at, ends_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.Message, b.Kind, b.OnLogin, b.InApp, b.Audience, nullID(b.GroupID), b.Dismissible,
		unixOf(b.StartsAt), unixOf(b.EndsAt), nullID(by), now, now)
	if err != nil {
		return Banner{}, err
	}
	id, _ := res.LastInsertId()
	return s.BannerByID(ctx, id)
}

// UpdateBanner replaces a banner's settings; a changed text starts a new
// revision, so the banner shows again to those who closed it.
func (s *Store) UpdateBanner(ctx context.Context, b Banner) (Banner, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE banners SET
		revision = revision + (message <> ?1),
		message = ?1, kind = ?2, on_login = ?3, in_app = ?4, audience = ?5, group_id = ?6, dismissible = ?7,
		starts_at = ?8, ends_at = ?9, updated_at = ?10
		WHERE id = ?11`,
		b.Message, b.Kind, b.OnLogin, b.InApp, b.Audience, nullID(b.GroupID), b.Dismissible,
		unixOf(b.StartsAt), unixOf(b.EndsAt), s.unixNow(), b.ID)
	if err != nil {
		return Banner{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Banner{}, ErrNotFound
	}
	return s.BannerByID(ctx, b.ID)
}

// DeleteBanner removes a banner and its closings.
func (s *Store) DeleteBanner(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM banners WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// activeAt limits banners to their time window.
const activeAt = ` (b.starts_at = 0 OR b.starts_at <= ?1) AND (b.ends_at = 0 OR b.ends_at > ?1)`

// LoginBanners are the active banners for everyone on the sign-in page.
func (s *Store) LoginBanners(ctx context.Context, now time.Time) ([]Banner, error) {
	return s.queryBanners(ctx, `WHERE b.on_login = 1 AND b.audience = 'everyone' AND`+activeAt+bannerOrder, now.Unix())
}

// ActiveBanners are the banners an account sees in the app now: in its
// audience and not closed at their current revision.
func (s *Store) ActiveBanners(ctx context.Context, u User, now time.Time) ([]Banner, error) {
	return s.queryBanners(ctx, `WHERE b.in_app = 1 AND`+activeAt+`
		AND (b.audience = 'everyone'
			OR (b.audience = 'admins' AND ?3 = 'admin')
			OR (b.audience = 'group' AND (g.everyone = 1
				OR EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = b.group_id AND m.user_id = ?2))))
		AND NOT EXISTS (SELECT 1 FROM banner_dismissals d
			WHERE d.banner_id = b.id AND d.user_id = ?2 AND d.revision = b.revision)`+bannerOrder,
		now.Unix(), u.ID, string(u.Role))
}

// DismissBanner closes a banner for an account at its current revision.
func (s *Store) DismissBanner(ctx context.Context, bannerID, userID int64) error {
	b, err := s.BannerByID(ctx, bannerID)
	if err != nil {
		return err
	}
	if !b.Dismissible {
		return ErrNotDismissible
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO banner_dismissals (banner_id, user_id, revision, dismissed_at)
		VALUES (?1, ?2, ?3, ?4) ON CONFLICT (banner_id, user_id) DO UPDATE SET revision = ?3, dismissed_at = ?4`,
		bannerID, userID, b.Revision, s.unixNow())
	return err
}

// CleanBannerText trims a banner text and checks it: 1 to MaxBannerRunes
// characters, no control characters but line breaks and tabs.
func CleanBannerText(s string) (string, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	n := 0
	for _, r := range s {
		n++
		if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return "", false
		}
	}
	return s, n > 0 && n <= MaxBannerRunes
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
