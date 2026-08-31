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

	ID          int64  `bun:"id,pk,autoincrement"`
	Identity    string `bun:"identity,notnull"`
	DisplayName string `bun:"display_name"`
	IsAdmin     bool   `bun:"is_admin,notnull"`
	// IsBootstrap is set from configuration at every startup, and is what
	// keeps a re-derivation from group membership out of the way back in.
	IsBootstrap bool `bun:"is_bootstrap,notnull"`
	// AdminDerived says a group granted this rather than a person. Only what
	// a group gave is taken back when groups stop deciding, so somebody
	// promoted inside the application survives a change of mode.
	AdminDerived bool       `bun:"admin_derived,notnull"`
	CreatedAt    time.Time  `bun:"created_at,notnull"`
	LastSeenAt   *time.Time `bun:"last_seen_at"`
}

// Grant is one role held against one product.
type Grant struct {
	bun.BaseModel `bun:"table:role_grant,alias:rg"`

	ID        int64 `bun:"id,pk,autoincrement"`
	PersonID  int64 `bun:"person_id,notnull"`
	ProductID int64 `bun:"product_id,notnull"`
	Role      Role  `bun:"role,notnull"`
	// Source says whether an administrator assigned this or a group derived
	// it, and Active whether it grants anything at all right now. A grant made
	// inactive by a change of mode is kept so the change can be undone, and it
	// is never counted as access while it sits there.
	Source    Source    `bun:"source,notnull"`
	Active    bool      `bun:"active,notnull"`
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

// handle returns the connection this store was built over, or reports that it
// is already inside a transaction.
//
// A store built over a transaction cannot start another, and a retry that
// re-ran the inner half alone would repeat part of a transaction whose other
// part had been rolled back. Saying so is better than silently doing it.
func (s *Store) handle() (*bun.DB, error) {
	db, ok := s.db.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("this store is already inside a transaction")
	}
	return db, nil
}

// seenResolution is how coarse a last-used record is.
//
// It answers "is this still in use", which is a question about days. Recording
// it to the second would make every read a write on the row a person touches
// most, for a precision nothing asks for.
const seenResolution = time.Hour

// staleEnough reports whether a last-used stamp is old enough to rewrite.
func staleEnough(recorded *time.Time, now time.Time) bool {
	return recorded == nil || now.Sub(*recorded) >= seenResolution
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
	// Inactive grants are read past entirely. A row that grants nothing must
	// never be counted as access — not here, and not in any report or review
	// that asks what somebody holds (ACC-37).
	if err := s.db.NewSelect().Model(&held).
		Where("person_id = ?", person.ID).Where("active = ?", true).Scan(ctx); err != nil {
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

	// Written only when it has gone stale, not on every request. Every
	// authenticated request passes through here, so writing each time makes a
	// person's own row the hottest in the database and makes every read a
	// write — which a replica cannot serve from a follower and which, on a
	// cluster, turns two concurrent requests from one person into a
	// certification conflict at commit. The date is what anybody reads it for,
	// so an hour's resolution loses nothing.
	if seen := s.now().Truncate(time.Microsecond); staleEnough(person.LastSeenAt, seen) {
		if _, err := s.db.NewUpdate().Model((*Account)(nil)).
			Set("last_seen_at = ?", seen).Where("id = ?", person.ID).Exec(ctx); err != nil {
			return Subject{}, fmt.Errorf("record that %q was seen: %w", identity, err)
		}
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
		Source: Assigned, Active: true,
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
		Where("role = ?", role).Where("active = ?", true).Count(ctx)
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
	secret := KeyPrefix + base64.RawURLEncoding.EncodeToString(raw)

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
// People lists everybody and what they hold, inactive grants included and
// marked as such.
//
// The rows are returned whole rather than filtered, because this is the view
// an access review reads: a grant that has been set aside has to be visible as
// set aside, not hidden and not counted. What must never happen is an inactive
// row reading like a live one, which is what the caller renders.
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

// HoldsAnythingIn reports whether somebody still has any role on a product.
//
// Asked after a role is withdrawn, because the last one going is what turns
// their assigned work into work nobody can reach: assigned, so not in the
// shared queue, and assigned to somebody who can no longer open it.
func (s *Store) HoldsAnythingIn(ctx context.Context, personID, productID int64) (bool, error) {
	n, err := s.db.NewSelect().Model((*Grant)(nil)).
		Where("person_id = ?", personID).
		Where("product_id = ?", productID).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("read what they still hold: %w", err)
	}
	return n > 0, nil
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

// Names resolves people to what to call them, for showing who did something.
//
// A display name where one is known, and the identity they signed in with
// otherwise. Batched because the alternative is a query per row, and the
// places this is needed — a review queue, a list of what was dismissed — are
// exactly the ones that are long.
func (s *Store) Names(ctx context.Context, ids []int64) (map[int64]string, error) {
	names := map[int64]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var people []Account
	if err := s.db.NewSelect().Model(&people).
		Column("id", "identity", "display_name").
		Where("id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read who these people are: %w", err)
	}
	for _, person := range people {
		if person.DisplayName != "" {
			names[person.ID] = person.DisplayName
			continue
		}
		names[person.ID] = person.Identity
	}
	return names, nil
}
