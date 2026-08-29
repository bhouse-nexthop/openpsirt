package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

// Account is somebody who has been granted access.
type Account struct {
	bun.BaseModel `bun:"table:person,alias:pe"`

	ID          int64      `bun:"id,pk,autoincrement"`
	Identity    string     `bun:"identity,notnull"`
	DisplayName string     `bun:"display_name"`
	IsAdmin     bool       `bun:"is_admin,notnull"`
	CreatedAt   time.Time  `bun:"created_at,notnull"`
	LastSeenAt  *time.Time `bun:"last_seen_at"`
}

// Grant is one role held against one product.
type Grant struct {
	bun.BaseModel `bun:"table:role_grant,alias:rg"`

	ID        int64     `bun:"id,pk,autoincrement"`
	PersonID  int64     `bun:"person_id,notnull"`
	ProductID int64     `bun:"product_id,notnull"`
	Role      Role      `bun:"role,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

// Key is a pipeline's credential.
type Key struct {
	bun.BaseModel `bun:"table:api_key,alias:ak"`

	ID         int64      `bun:"id,pk,autoincrement"`
	Name       string     `bun:"name,notnull"`
	SecretHash string     `bun:"secret_hash,notnull"`
	ProductID  int64      `bun:"product_id,notnull"`
	StreamID   *int64     `bun:"stream_id"`
	VariantID  *int64     `bun:"variant_id"`
	CreatedAt  time.Time  `bun:"created_at,notnull"`
	LastUsedAt *time.Time `bun:"last_used_at"`
	RevokedAt  *time.Time `bun:"revoked_at"`
}

// Store reads and writes who may do what.
type Store struct {
	db  bun.IDB
	now func() time.Time
}

// NewStore returns a store over db.
func NewStore(db bun.IDB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Ensure records somebody who has been granted access, or confirms one already
// recorded.
//
// This is the only path that creates a person, and nothing on a sign-in path
// calls it. Access is granted in advance or not at all.
func (s *Store) Ensure(ctx context.Context, identity, displayName string, admin bool) (*Account, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil, fmt.Errorf("a person needs an identity to be granted anything")
	}

	existing, err := s.ByIdentity(ctx, identity)
	if err == nil {
		if existing.IsAdmin != admin {
			if _, err := s.db.NewUpdate().Model((*Account)(nil)).
				Set("is_admin = ?", admin).Where("id = ?", existing.ID).Exec(ctx); err != nil {
				return nil, fmt.Errorf("record that %q is an administrator: %w", identity, err)
			}
			existing.IsAdmin = admin
		}
		return existing, nil
	}

	person := &Account{
		Identity: identity, DisplayName: displayName, IsAdmin: admin,
		CreatedAt: s.now().Truncate(time.Microsecond),
	}
	if _, err := s.db.NewInsert().Model(person).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record %q: %w", identity, err)
	}
	return person, nil
}

// ByIdentity finds somebody by what a sign-in path calls them.
func (s *Store) ByIdentity(ctx context.Context, identity string) (*Account, error) {
	person := new(Account)
	err := s.db.NewSelect().Model(person).Where("identity = ?", identity).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("look up %q: %w", identity, err)
	}
	return person, nil
}

// Resolve turns an identity into the subject it stands for.
//
// Somebody unknown, and somebody known but granted nothing, are both refused —
// with the same answer, deliberately.
func (s *Store) Resolve(ctx context.Context, identity string) (Subject, error) {
	person, err := s.ByIdentity(ctx, identity)
	if err != nil {
		return Subject{}, ErrDenied
	}

	var held []Grant
	if err := s.db.NewSelect().Model(&held).Where("person_id = ?", person.ID).Scan(ctx); err != nil {
		return Subject{}, fmt.Errorf("read what %q may do: %w", identity, err)
	}
	grants := map[int64][]Role{}
	for _, grant := range held {
		// A row naming something that is not a role grants nothing. It can
		// only get there by hand or by a downgrade, and reading it as "some
		// role" would make it a grant of whatever the reader assumes.
		if !grant.Role.Valid() {
			continue
		}
		grants[grant.ProductID] = append(grants[grant.ProductID], grant.Role)
	}
	if !person.IsAdmin && len(grants) == 0 {
		return Subject{}, ErrDenied
	}

	seen := s.now().Truncate(time.Microsecond)
	if _, err := s.db.NewUpdate().Model((*Account)(nil)).
		Set("last_seen_at = ?", seen).Where("id = ?", person.ID).Exec(ctx); err != nil {
		return Subject{}, fmt.Errorf("record that %q was seen: %w", identity, err)
	}
	return NewPerson(person.ID, person.Identity, person.IsAdmin, grants), nil
}

// GrantRole gives somebody a role on a product.
func (s *Store) GrantRole(ctx context.Context, personID, productID int64, role Role) error {
	if !role.Valid() {
		return fmt.Errorf("%q is not a role", role)
	}
	grant := &Grant{
		PersonID: personID, ProductID: productID, Role: role,
		CreatedAt: s.now().Truncate(time.Microsecond),
	}
	if _, err := s.db.NewInsert().Model(grant).Exec(ctx); err != nil {
		// Granting what somebody already holds is not a failure.
		if held, err := s.holds(ctx, personID, productID, role); err == nil && held {
			return nil
		}
		return fmt.Errorf("grant %q: %w", role, err)
	}
	return nil
}

func (s *Store) holds(ctx context.Context, personID, productID int64, role Role) (bool, error) {
	n, err := s.db.NewSelect().Model((*Grant)(nil)).
		Where("person_id = ?", personID).Where("product_id = ?", productID).
		Where("role = ?", role).Count(ctx)
	return n > 0, err
}

// secretBytes is how much randomness a key carries.
//
// A key is not a password: it is generated here, never chosen, and never
// typed from memory. What it needs is enough entropy that guessing is not a
// strategy, which is also why the stored form is a plain digest rather than a
// slow hash — there is nothing to slow down when there is nothing to guess.
const secretBytes = 32

// NewKey creates a pipeline credential, returning the secret once.
//
// Once is the whole point. A credential store that can hand back what it holds
// is a credential store that hands over every pipeline's key along with a copy
// of the database.
func (s *Store) NewKey(ctx context.Context, name string, scope Scope) (*Key, string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", fmt.Errorf("generate a key: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)

	key := &Key{
		Name: name, SecretHash: hashSecret(secret),
		ProductID: scope.ProductID, StreamID: scope.StreamID, VariantID: scope.VariantID,
		CreatedAt: s.now().Truncate(time.Microsecond),
	}
	if _, err := s.db.NewInsert().Model(key).Exec(ctx); err != nil {
		return nil, "", fmt.Errorf("record a key: %w", err)
	}
	return key, secret, nil
}

// ResolveKey turns a presented secret into the subject it stands for.
func (s *Store) ResolveKey(ctx context.Context, secret string) (Subject, error) {
	if strings.TrimSpace(secret) == "" {
		return Subject{}, ErrDenied
	}

	key := new(Key)
	err := s.db.NewSelect().Model(key).Where("secret_hash = ?", hashSecret(secret)).Scan(ctx)
	if err != nil {
		return Subject{}, ErrDenied
	}
	// Compared again in constant time. The lookup above found a row by digest,
	// which is not by itself a statement that the secrets match.
	if subtle.ConstantTimeCompare([]byte(key.SecretHash), []byte(hashSecret(secret))) != 1 {
		return Subject{}, ErrDenied
	}
	if key.RevokedAt != nil {
		return Subject{}, ErrDenied
	}

	used := s.now().Truncate(time.Microsecond)
	if _, err := s.db.NewUpdate().Model((*Key)(nil)).
		Set("last_used_at = ?", used).Where("id = ?", key.ID).Exec(ctx); err != nil {
		return Subject{}, fmt.Errorf("record that a key was used: %w", err)
	}
	return NewPipeline(key.ID, key.Name, Scope{
		ProductID: key.ProductID, StreamID: key.StreamID, VariantID: key.VariantID,
	}), nil
}

// Revoke stops a key working, without removing what it did.
func (s *Store) Revoke(ctx context.Context, keyID int64) error {
	_, err := s.db.NewUpdate().Model((*Key)(nil)).
		Set("revoked_at = ?", s.now().Truncate(time.Microsecond)).
		Where("id = ?", keyID).Where("revoked_at IS NULL").Exec(ctx)
	if err != nil {
		return fmt.Errorf("revoke key %d: %w", keyID, err)
	}
	return nil
}

// hashSecret is what gets stored.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// People lists everybody who has been granted something, with what they hold.
func (s *Store) People(ctx context.Context) ([]Account, map[int64][]Grant, error) {
	var people []Account
	if err := s.db.NewSelect().Model(&people).Order("identity").Scan(ctx); err != nil {
		return nil, nil, fmt.Errorf("list people: %w", err)
	}
	var grants []Grant
	if err := s.db.NewSelect().Model(&grants).Order("person_id", "product_id").Scan(ctx); err != nil {
		return nil, nil, fmt.Errorf("list what people hold: %w", err)
	}
	held := map[int64][]Grant{}
	for _, grant := range grants {
		held[grant.PersonID] = append(held[grant.PersonID], grant)
	}
	return people, held, nil
}

// Withdraw takes a role away.
//
// The row is removed rather than marked. A grant is a statement about now, and
// what somebody used to hold is answered by the record of what they did, not
// by keeping a permission that no longer applies.
func (s *Store) Withdraw(ctx context.Context, personID, productID int64, role Role) error {
	_, err := s.db.NewDelete().Model((*Grant)(nil)).
		Where("person_id = ?", personID).
		Where("product_id = ?", productID).
		Where("role = ?", role).Exec(ctx)
	if err != nil {
		return fmt.Errorf("withdraw %q: %w", role, err)
	}
	return nil
}

// Keys lists the pipeline credentials, without their secrets.
//
// There is nothing to list them with: what is stored is a digest, and that is
// the point. What an operator needs is which keys exist, what each reaches,
// when it was last used, and whether it still works.
func (s *Store) Keys(ctx context.Context) ([]Key, error) {
	var keys []Key
	if err := s.db.NewSelect().Model(&keys).Order("name").Scan(ctx); err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	return keys, nil
}
