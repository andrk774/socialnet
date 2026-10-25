package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Роли в сообществе по возрастанию прав.
var roleRank = map[string]int{"": 0, "member": 1, "editor": 2, "admin": 3, "owner": 4}

// RoleAtLeast: есть ли у роли права не ниже need.
func RoleAtLeast(role, need string) bool { return roleRank[role] >= roleRank[need] }

type Community struct {
	ID           int64     `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	OwnerID      int64     `json:"owner_id"`
	MembersCount int       `json:"members_count"`
	MyRole       string    `json:"my_role"` // пусто, если не участник
	CreatedAt    time.Time `json:"created_at"`
	Hidden       bool      `json:"-"`
}

const communitySelect = `
	SELECT c.id, c.slug, c.name, c.description, c.owner_id,
	       (SELECT count(*) FROM community_members m WHERE m.community_id = c.id),
	       COALESCE((SELECT role FROM community_members m WHERE m.community_id = c.id AND m.user_id = $1), ''),
	       c.created_at, c.hidden_at IS NOT NULL
	FROM communities c`

func scanCommunities(rows pgx.Rows) ([]Community, error) {
	defer rows.Close()
	out := []Community{}
	for rows.Next() {
		var c Community
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.Description, &c.OwnerID,
			&c.MembersCount, &c.MyRole, &c.CreatedAt, &c.Hidden); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) communityOne(ctx context.Context, where string, args ...any) (*Community, error) {
	rows, err := s.pool.Query(ctx, communitySelect+" WHERE "+where, args...)
	if err != nil {
		return nil, err
	}
	list, err := scanCommunities(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}

func (s *Store) CommunityByID(ctx context.Context, viewer, id int64) (*Community, error) {
	return s.communityOne(ctx, "c.id = $2", viewer, id)
}

func (s *Store) CommunityBySlug(ctx context.Context, viewer int64, slug string) (*Community, error) {
	return s.communityOne(ctx, "c.slug = $2", viewer, slug)
}

func (s *Store) CreateCommunity(ctx context.Context, owner int64, slug, name, desc string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO communities (slug, name, description, owner_id) VALUES ($1, $2, $3, $4) RETURNING id`,
		slug, name, desc, owner).Scan(&id)
	if _, ok := isUnique(err); ok {
		return 0, ErrSlugTaken
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO community_members (community_id, user_id, role) VALUES ($1, $2, 'owner')`, id, owner); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) UpdateCommunity(ctx context.Context, id int64, name, desc *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE communities SET name = COALESCE($2, name), description = COALESCE($3, description)
		WHERE id = $1`, id, name, desc)
	return err
}

// CommunityRole: роль пользователя или "" для не участника.
func (s *Store) CommunityRole(ctx context.Context, communityID, userID int64) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM community_members WHERE community_id = $1 AND user_id = $2`,
		communityID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (s *Store) JoinCommunity(ctx context.Context, communityID, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO community_members (community_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		communityID, userID)
	return err
}

// RemoveMember: владельца удалить нельзя (у сообщества всегда есть владелец).
func (s *Store) RemoveMember(ctx context.Context, communityID, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM community_members WHERE community_id = $1 AND user_id = $2 AND role <> 'owner'`,
		communityID, userID)
	return err
}

func (s *Store) SetMemberRole(ctx context.Context, communityID, userID int64, role string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE community_members SET role = $3
		WHERE community_id = $1 AND user_id = $2 AND role <> 'owner'`,
		communityID, userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type Member struct {
	UserCard
	Role string `json:"role"`
}

func (s *Store) Members(ctx context.Context, communityID, after int64, limit int) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+cardCols+`, m.role
		FROM community_members m JOIN users u ON u.id = m.user_id
		WHERE m.community_id = $1 AND u.id > $2 AND u.banned_at IS NULL
		ORDER BY u.id LIMIT $3`, communityID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		var avatar *string
		if err := rows.Scan(&m.ID, &m.Username, &m.FirstName, &m.LastName, &avatar, &m.Role); err != nil {
			return nil, err
		}
		m.AvatarURL = MediaURL(avatar, "thumb")
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) MyCommunities(ctx context.Context, userID, after int64, limit int) ([]Community, error) {
	rows, err := s.pool.Query(ctx, communitySelect+`
		JOIN community_members me ON me.community_id = c.id AND me.user_id = $1
		WHERE c.id > $2 AND c.hidden_at IS NULL
		ORDER BY c.id LIMIT $3`, userID, after, limit)
	if err != nil {
		return nil, err
	}
	return scanCommunities(rows)
}
