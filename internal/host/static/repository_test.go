package static

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	wrapping "github.com/hashicorp/go-kms-wrapping"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/watchtower/internal/db"
	"github.com/hashicorp/watchtower/internal/host/static/store"
	"github.com/hashicorp/watchtower/internal/iam"
)

func TestRepository_New(t *testing.T) {

	cleanup, conn, _ := db.TestSetup(t, "postgres")
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	}()

	rw := db.New(conn)
	wrapper := db.TestWrapper(t)

	type args struct {
		r       db.Reader
		w       db.Writer
		wrapper wrapping.Wrapper
	}

	var tests = []struct {
		name      string
		args      args
		want      *Repository
		wantIsErr error
	}{
		{
			name: "valid",
			args: args{
				r:       rw,
				w:       rw,
				wrapper: wrapper,
			},
			want: &Repository{
				reader:  rw,
				writer:  rw,
				wrapper: wrapper,
			},
		},
		{
			name: "nil-reader",
			args: args{
				r:       nil,
				w:       rw,
				wrapper: wrapper,
			},
			want:      nil,
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "nil-writer",
			args: args{
				r:       rw,
				w:       nil,
				wrapper: wrapper,
			},
			want:      nil,
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "nil-wrapper",
			args: args{
				r:       rw,
				w:       rw,
				wrapper: nil,
			},
			want:      nil,
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "all-nils",
			args: args{
				r:       nil,
				w:       nil,
				wrapper: nil,
			},
			want:      nil,
			wantIsErr: db.ErrNilParameter,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			got, err := NewRepository(tt.args.r, tt.args.w, tt.args.wrapper)
			if tt.wantIsErr != nil {
				assert.Truef(errors.Is(err, tt.wantIsErr), "want err: %q got: %q", tt.wantIsErr, err)
				assert.Nil(got)
				return
			}
			assert.NoError(err)
			require.NotNil(got)
			assert.Equal(tt.want, got)
		})
	}
}

func TestRepository_CreateCatalog(t *testing.T) {
	cleanup, conn, _ := db.TestSetup(t, "postgres")
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	rw := db.New(conn)
	wrapper := db.TestWrapper(t)

	_, prj := iam.TestScopes(t, conn)

	var tests = []struct {
		name      string
		in        *HostCatalog
		opts      []Option
		want      *HostCatalog
		wantIsErr error
	}{
		{
			name:      "nil-catalog",
			wantIsErr: db.ErrNilParameter,
		},
		{
			name:      "nil-embedded-catalog",
			in:        &HostCatalog{},
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "invalid-no-scope-id",
			in: &HostCatalog{
				HostCatalog: &store.HostCatalog{},
			},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "invalid-public-id-set",
			in: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId:  prj.PublicId,
					PublicId: "sthc_OOOOOOOOOO",
				},
			},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "valid-no-options",
			in: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId: prj.PublicId,
				},
			},
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId: prj.PublicId,
				},
			},
		},
		{
			name: "valid-with-name",
			in: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId: prj.PublicId,
					Name:    "test-name-repo",
				},
			},
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId: prj.PublicId,
					Name:    "test-name-repo",
				},
			},
		},
		{
			name: "valid-with-description",
			in: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId:     prj.PublicId,
					Description: ("test-description-repo"),
				},
			},
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					ScopeId:     prj.PublicId,
					Description: ("test-description-repo"),
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			repo, err := NewRepository(rw, rw, wrapper)
			assert.NoError(err)
			require.NotNil(repo)
			got, err := repo.CreateCatalog(context.Background(), tt.in, tt.opts...)
			if tt.wantIsErr != nil {
				assert.Truef(errors.Is(err, tt.wantIsErr), "want err: %q got: %q", tt.wantIsErr, err)
				assert.Nil(got)
				return
			}
			assert.NoError(err)
			assert.Empty(tt.in.PublicId)
			require.NotNil(got)
			assertPublicId(t, HostCatalogPrefix, got.PublicId)
			assert.NotSame(tt.in, got)
			assert.Equal(tt.want.Name, got.Name)
			assert.Equal(tt.want.Description, got.Description)
			assert.Equal(got.CreateTime, got.UpdateTime)
		})
	}

	t.Run("invalid-duplicate-names", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		_, prj := iam.TestScopes(t, conn)
		in := &HostCatalog{
			HostCatalog: &store.HostCatalog{
				ScopeId: prj.GetPublicId(),
				Name:    "test-name-repo",
			},
		}

		got, err := repo.CreateCatalog(context.Background(), in)
		assert.NoError(err)
		require.NotNil(got)
		assertPublicId(t, HostCatalogPrefix, got.PublicId)
		assert.NotSame(in, got)
		assert.Equal(in.Name, got.Name)
		assert.Equal(in.Description, got.Description)
		assert.Equal(got.CreateTime, got.UpdateTime)

		got2, err := repo.CreateCatalog(context.Background(), in)
		assert.Truef(errors.Is(err, db.ErrNotUnique), "want err: %v got: %v", db.ErrNotUnique, err)
		assert.Nil(got2)
	})

	t.Run("valid-duplicate-names-diff-scopes", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		org, prj := iam.TestScopes(t, conn)
		in := &HostCatalog{
			HostCatalog: &store.HostCatalog{
				Name: "test-name-repo",
			},
		}
		in2 := in.clone()

		in.ScopeId = prj.GetPublicId()
		got, err := repo.CreateCatalog(context.Background(), in)
		assert.NoError(err)
		require.NotNil(got)
		assertPublicId(t, HostCatalogPrefix, got.PublicId)
		assert.NotSame(in, got)
		assert.Equal(in.Name, got.Name)
		assert.Equal(in.Description, got.Description)
		assert.Equal(got.CreateTime, got.UpdateTime)

		in2.ScopeId = org.GetPublicId()
		got2, err := repo.CreateCatalog(context.Background(), in2)
		assert.NoError(err)
		require.NotNil(got2)
		assertPublicId(t, HostCatalogPrefix, got2.PublicId)
		assert.NotSame(in2, got2)
		assert.Equal(in2.Name, got2.Name)
		assert.Equal(in2.Description, got2.Description)
		assert.Equal(got2.CreateTime, got2.UpdateTime)
	})
}

func assertPublicId(t *testing.T, prefix, actual string) {
	t.Helper()
	assert.NotEmpty(t, actual)
	parts := strings.Split(actual, "_")
	assert.Equalf(t, 2, len(parts), "want one '_' in PublicId, got multiple in %q", actual)
	assert.Equalf(t, prefix, parts[0], "PublicId want prefix: %q, got: %q in %q", prefix, parts[0], actual)
}

func TestRepository_UpdateCatalog(t *testing.T) {
	cleanup, conn, _ := db.TestSetup(t, "postgres")
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	rw := db.New(conn)
	wrapper := db.TestWrapper(t)

	changeName := func(s string) func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			c.Name = s
			return c
		}
	}

	changeDescription := func(s string) func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			c.Description = s
			return c
		}
	}

	makeNil := func() func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			return nil
		}
	}

	makeEmbeddedNil := func() func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			return &HostCatalog{}
		}
	}

	deletePublicId := func() func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			c.PublicId = ""
			return c
		}
	}

	nonExistentPublicId := func() func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			c.PublicId = "sthc_OOOOOOOOOO"
			return c
		}
	}

	combine := func(fns ...func(c *HostCatalog) *HostCatalog) func(*HostCatalog) *HostCatalog {
		return func(c *HostCatalog) *HostCatalog {
			for _, fn := range fns {
				c = fn(c)
			}
			return c
		}
	}

	var tests = []struct {
		name      string
		orig      *HostCatalog
		chgFn     func(*HostCatalog) *HostCatalog
		masks     []string
		want      *HostCatalog
		wantCount int
		wantIsErr error
	}{
		{
			name: "nil-catalog",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{},
			},
			chgFn:     makeNil(),
			masks:     []string{"Name", "Description"},
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "nil-embedded-catalog",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{},
			},
			chgFn:     makeEmbeddedNil(),
			masks:     []string{"Name", "Description"},
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "no-public-id",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{},
			},
			chgFn:     deletePublicId(),
			masks:     []string{"Name", "Description"},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "updating-non-existent-catalog",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-name-repo",
				},
			},
			chgFn:     combine(nonExistentPublicId(), changeName("test-update-name-repo")),
			masks:     []string{"Name"},
			wantIsErr: db.ErrRecordNotFound,
		},
		{
			name: "empty-field-mask",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-name-repo",
				},
			},
			chgFn:     changeName("test-update-name-repo"),
			wantIsErr: db.ErrEmptyFieldMask,
		},
		{
			name: "read-only-fields-in-field-mask",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-name-repo",
				},
			},
			chgFn:     changeName("test-update-name-repo"),
			masks:     []string{"PublicId", "CreateTime", "UpdateTime", "ScopeId"},
			wantIsErr: db.ErrInvalidFieldMask,
		},
		{
			name: "unknown-field-in-field-mask",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-name-repo",
				},
			},
			chgFn:     changeName("test-update-name-repo"),
			masks:     []string{"Bilbo"},
			wantIsErr: db.ErrInvalidFieldMask,
		},
		{
			name: "change-name",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-name-repo",
				},
			},
			chgFn: changeName("test-update-name-repo"),
			masks: []string{"Name"},
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-update-name-repo",
				},
			},
			wantCount: 1,
		},
		{
			name: "change-description",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Description: "test-description-repo",
				},
			},
			chgFn: changeDescription("test-update-description-repo"),
			masks: []string{"Description"},
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Description: "test-update-description-repo",
				},
			},
			wantCount: 1,
		},
		{
			name: "change-name-and-description",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-name-repo",
					Description: "test-description-repo",
				},
			},
			chgFn: combine(changeDescription("test-update-description-repo"), changeName("test-update-name-repo")),
			masks: []string{"Name", "Description"},
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-update-name-repo",
					Description: "test-update-description-repo",
				},
			},
			wantCount: 1,
		},
		{
			name: "delete-name",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-name-repo",
					Description: "test-description-repo",
				},
			},
			masks: []string{"Name"},
			chgFn: combine(changeDescription("test-update-description-repo"), changeName("")),
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Description: "test-description-repo",
				},
			},
			wantCount: 1,
		},
		{
			name: "delete-description",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-name-repo",
					Description: "test-description-repo",
				},
			},
			masks: []string{"Description"},
			chgFn: combine(changeDescription(""), changeName("test-update-name-repo")),
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name: "test-name-repo",
				},
			},
			wantCount: 1,
		},
		{
			name: "do-not-delete-name",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-name-repo",
					Description: "test-description-repo",
				},
			},
			masks: []string{"Description"},
			chgFn: combine(changeDescription("test-update-description-repo"), changeName("")),
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-name-repo",
					Description: "test-update-description-repo",
				},
			},
			wantCount: 1,
		},
		{
			name: "do-not-delete-description",
			orig: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-name-repo",
					Description: "test-description-repo",
				},
			},
			masks: []string{"Name"},
			chgFn: combine(changeDescription(""), changeName("test-update-name-repo")),
			want: &HostCatalog{
				HostCatalog: &store.HostCatalog{
					Name:        "test-update-name-repo",
					Description: "test-description-repo",
				},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			repo, err := NewRepository(rw, rw, wrapper)
			assert.NoError(err)
			require.NotNil(repo)
			_, prj := iam.TestScopes(t, conn)
			tt.orig.ScopeId = prj.GetPublicId()
			orig, err := repo.CreateCatalog(context.Background(), tt.orig)
			assert.NoError(err)
			require.NotNil(orig)

			if tt.chgFn != nil {
				orig = tt.chgFn(orig)
			}
			got, gotCount, err := repo.UpdateCatalog(context.Background(), orig, tt.masks)
			if tt.wantIsErr != nil {
				assert.Truef(errors.Is(err, tt.wantIsErr), "want err: %q got: %q", tt.wantIsErr, err)
				assert.Equal(tt.wantCount, gotCount, "row count")
				assert.Nil(got)
				return
			}
			assert.NoError(err)
			assert.Empty(tt.orig.PublicId)
			require.NotNil(got)
			assertPublicId(t, HostCatalogPrefix, got.PublicId)
			assert.Equal(tt.wantCount, gotCount, "row count")
			assert.NotSame(tt.orig, got)
			assert.Equal(tt.orig.ScopeId, got.ScopeId)
			if tt.want.Name == "" {
				assertColumnIsNull(t, conn, got, "name")
				return
			}
			assert.Equal(tt.want.Name, got.Name)
			if tt.want.Description == "" {
				assertColumnIsNull(t, conn, got, "description")
				return
			}
			assert.Equal(tt.want.Description, got.Description)
		})
	}

	t.Run("invalid-duplicate-names", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		name := "test-dup-name"
		cats := testCatalogs(t, conn, 2)
		c1 := cats[0]
		c1.Name = name
		got1, gotCount1, err := repo.UpdateCatalog(context.Background(), c1, []string{"name"})
		assert.NoError(err)
		require.NotNil(got1)
		assert.Equal(name, got1.Name)
		assert.Equal(1, gotCount1, "row count")

		c2 := cats[1]
		c2.Name = name
		got2, gotCount2, err := repo.UpdateCatalog(context.Background(), c2, []string{"name"})
		assert.Truef(errors.Is(err, db.ErrNotUnique), "want err: %v got: %v", db.ErrNotUnique, err)
		assert.Nil(got2)
		assert.Equal(db.NoRowsAffected, gotCount2, "row count")
	})

	t.Run("valid-duplicate-names-diff-scopes", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		org, prj := iam.TestScopes(t, conn)
		in := &HostCatalog{
			HostCatalog: &store.HostCatalog{
				Name: "test-name-repo",
			},
		}
		in2 := in.clone()

		in.ScopeId = prj.GetPublicId()
		got, err := repo.CreateCatalog(context.Background(), in)
		assert.NoError(err)
		require.NotNil(got)
		assertPublicId(t, HostCatalogPrefix, got.PublicId)
		assert.NotSame(in, got)
		assert.Equal(in.Name, got.Name)
		assert.Equal(in.Description, got.Description)

		in2.ScopeId = org.GetPublicId()
		in2.Name = "first-name"
		got2, err := repo.CreateCatalog(context.Background(), in2)
		assert.NoError(err)
		require.NotNil(got2)
		got2.Name = got.Name
		got3, gotCount3, err := repo.UpdateCatalog(context.Background(), got2, []string{"name"})
		assert.NoError(err)
		require.NotNil(got3)
		assert.NotSame(got2, got3)
		assert.Equal(got.Name, got3.Name)
		assert.Equal(got2.Description, got3.Description)
		assert.Equal(1, gotCount3, "row count")
	})

	t.Run("change-scope-id", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		c1, c2 := testCatalog(t, conn), testCatalog(t, conn)
		assert.NotEqual(c1.ScopeId, c2.ScopeId)
		orig := c1.clone()

		c1.ScopeId = c2.ScopeId
		assert.Equal(c1.ScopeId, c2.ScopeId)

		got1, gotCount1, err := repo.UpdateCatalog(context.Background(), c1, []string{"name"})

		assert.NoError(err)
		require.NotNil(got1)
		assert.Equal(orig.ScopeId, got1.ScopeId)
		assert.Equal(1, gotCount1, "row count")
	})
}

// TODO(mgaffney,jimlambrt) 05/2020: delete after support for setting
// columns to nil is added to db.Update.
func TestNullAssert(t *testing.T) {
	/*
		cleanup, conn, _ := db.TestSetup(t, "postgres")
		defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
		}()

		isNotNull := &HostCatalog{HostCatalog: &store.HostCatalog{PublicId: "sthc_JSH52G07wI"}}
		m := isNotNull
		if err := conn.Model(m).Where("public_id = ?", m.GetPublicId()).UpdateColumn("name", gorm.Expr("NULL")).Error; err != nil {
			t.Fatalf("could not set to null: %v", err)
		}

		assertColumnIsNull(t, conn, isNotNull, "name")
	*/
}

type resource interface {
	GetPublicId() string
	TableName() string
}

func assertColumnIsNull(t *testing.T, db *gorm.DB, m resource, column string) {

	query := fmt.Sprintf("public_id = ? AND %s is null", column)
	var count int

	// TODO(mgaffney) 05/2020: There is a good chance this test method will be
	// needed in other packages. If and when that happens, this method
	// should be moved to the db package. If the method is not needed, then
	// the method should be refactored to eliminate the direct call on
	// gorm.
	if err := db.Model(m).Where(query, m.GetPublicId()).Count(&count).Error; err != nil {
		t.Fatalf("could not query: table: %s, column: %s, public_id: %s err: %v", m.TableName(), column, m.GetPublicId(), err)
	}
	assert.Equalf(t, 1, count, "want NULL, got NOT NULL - table: %s, column: %s, public_id: %s", m.TableName(), column, m.GetPublicId())
}

func TestRepository_LookupCatalog(t *testing.T) {
	cleanup, conn, _ := db.TestSetup(t, "postgres")
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	cat := testCatalog(t, conn)
	badId, err := newHostCatalogId()
	assert.NoError(t, err)
	require.NotNil(t, badId)

	rw := db.New(conn)
	wrapper := db.TestWrapper(t)

	var tests = []struct {
		name    string
		id      string
		want    *HostCatalog
		wantErr error
	}{
		{
			name: "found",
			id:   cat.GetPublicId(),
			want: cat,
		},
		{
			name: "not-found",
			id:   badId,
			want: nil,
		},
		{
			name:    "bad-public-id",
			id:      "",
			want:    nil,
			wantErr: db.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			repo, err := NewRepository(rw, rw, wrapper)
			assert.NoError(err)
			require.NotNil(repo)

			got, err := repo.LookupCatalog(context.Background(), tt.id)
			if tt.wantErr != nil {
				assert.Truef(errors.Is(err, tt.wantErr), "want err: %q got: %q", tt.wantErr, err)
				return
			}
			assert.NoError(err)

			switch {
			case tt.want == nil:
				assert.Nil(got)
			case tt.want != nil:
				require.NotNil(got)
				assert.Equal(got, tt.want)
			}
		})
	}
}

func TestRepository_DeleteCatalog(t *testing.T) {
	cleanup, conn, _ := db.TestSetup(t, "postgres")
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()

	cat := testCatalog(t, conn)
	badId, err := newHostCatalogId()
	assert.NoError(t, err)
	require.NotNil(t, badId)

	rw := db.New(conn)
	wrapper := db.TestWrapper(t)

	var tests = []struct {
		name    string
		id      string
		want    int
		wantErr error
	}{
		{
			name: "found",
			id:   cat.GetPublicId(),
			want: 1,
		},
		{
			name: "not-found",
			id:   badId,
			want: 0,
		},
		{
			name:    "bad-public-id",
			id:      "",
			want:    0,
			wantErr: db.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			repo, err := NewRepository(rw, rw, wrapper)
			assert.NoError(err)
			require.NotNil(repo)

			got, err := repo.DeleteCatalog(context.Background(), tt.id)
			if tt.wantErr != nil {
				assert.Truef(errors.Is(err, tt.wantErr), "want err: %q got: %q", tt.wantErr, err)
				return
			}
			assert.NoError(err)
			assert.Equal(tt.want, got, "row count")
		})
	}
}

func randomString(size int) string {
	r := rand.New(rand.NewSource(42))
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	sb := strings.Builder{}
	sb.Grow(size)
	for i := 0; i < size; i++ {
		sb.WriteByte(letters[r.Intn(len(letters))])
	}
	return sb.String()
}

func TestRepository_CreateHost(t *testing.T) {
	// TODO(mgaffney) 06/2020: refactor: extract code that is common with
	// the TestRepository_CreateCatalog function.

	cleanup, conn, _ := db.TestSetup(t, "postgres")
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	rw := db.New(conn)
	wrapper := db.TestWrapper(t)

	cat := testCatalog(t, conn)

	minAddress, maxAddress := randomString(7), randomString(255)

	var tests = []struct {
		name      string
		in        *Host
		opts      []Option
		want      *Host
		wantIsErr error
	}{
		{
			name:      "nil-host",
			wantIsErr: db.ErrNilParameter,
		},
		{
			name:      "nil-embedded-host",
			in:        &Host{},
			wantIsErr: db.ErrNilParameter,
		},
		{
			name: "invalid-no-catalog-id",
			in: &Host{
				Host: &store.Host{
					Address: "1.1.1.1",
				},
			},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "invalid-public-id-set",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					PublicId:            "sth_OOOOOOOOOO",
					Address:             "1.1.1.1",
				},
			},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "address-to-small",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             randomString(6),
				},
			},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "address-to-large",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             randomString(256),
				},
			},
			wantIsErr: db.ErrInvalidParameter,
		},
		{
			name: "valid-minimum-address",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             minAddress,
				},
			},
			want: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             minAddress,
				},
			},
		},
		{
			name: "valid-maximum-address",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             maxAddress,
				},
			},
			want: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             maxAddress,
				},
			},
		},
		{
			name: "valid-no-options",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             "1.1.1.1",
				},
			},
			want: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Address:             "1.1.1.1",
				},
			},
		},
		{
			name: "valid-with-name",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Name:                "test-name-repo",
					Address:             "1.1.1.1",
				},
			},
			want: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Name:                "test-name-repo",
					Address:             "1.1.1.1",
				},
			},
		},
		{
			name: "valid-with-description",
			in: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Description:         ("test-description-repo"),
					Address:             "1.1.1.1",
				},
			},
			want: &Host{
				Host: &store.Host{
					StaticHostCatalogId: cat.PublicId,
					Description:         ("test-description-repo"),
					Address:             "1.1.1.1",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert, require := assert.New(t), require.New(t)
			repo, err := NewRepository(rw, rw, wrapper)
			require.NoError(err)
			require.NotNil(repo)
			got, err := repo.CreateHost(context.Background(), tt.in, tt.opts...)
			if tt.wantIsErr != nil {
				assert.Truef(errors.Is(err, tt.wantIsErr), "want err: %q got: %q", tt.wantIsErr, err)
				assert.Nil(got)
				return
			}
			require.NoError(err)
			assert.Empty(tt.in.PublicId)
			require.NotNil(got)
			assertPublicId(t, HostPrefix, got.PublicId)
			assert.NotSame(tt.in, got)
			assert.Equal(tt.want.Name, got.Name)
			assert.Equal(tt.want.Description, got.Description)
			assert.Equal(tt.want.Address, got.Address)
			assert.Equal(got.CreateTime, got.UpdateTime)
		})
	}

	t.Run("invalid-duplicate-names", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		cat := testCatalog(t, conn)
		in := &Host{
			Host: &store.Host{
				StaticHostCatalogId: cat.GetPublicId(),
				Name:                "test-name-repo",
				Address:             minAddress,
			},
		}

		got, err := repo.CreateHost(context.Background(), in)
		assert.NoError(err)
		require.NotNil(got)
		assertPublicId(t, HostPrefix, got.PublicId)
		assert.NotSame(in, got)
		assert.Equal(in.Name, got.Name)
		assert.Equal(in.Description, got.Description)
		assert.Equal(got.CreateTime, got.UpdateTime)

		got2, err := repo.CreateHost(context.Background(), in)
		assert.Truef(errors.Is(err, db.ErrNotUnique), "want err: %v got: %v", db.ErrNotUnique, err)
		assert.Nil(got2)
	})

	t.Run("valid-duplicate-names-diff-catalogs", func(t *testing.T) {
		assert, require := assert.New(t), require.New(t)
		repo, err := NewRepository(rw, rw, wrapper)
		assert.NoError(err)
		require.NotNil(repo)

		cats := testCatalogs(t, conn, 2)
		cat1, cat2 := cats[0], cats[1]
		in := &Host{
			Host: &store.Host{
				Name:    "test-name-repo",
				Address: minAddress,
			},
		}
		in2 := in.clone()

		in.StaticHostCatalogId = cat2.GetPublicId()
		got, err := repo.CreateHost(context.Background(), in)
		assert.NoError(err)
		require.NotNil(got)
		assertPublicId(t, HostPrefix, got.PublicId)
		assert.NotSame(in, got)
		assert.Equal(in.Name, got.Name)
		assert.Equal(in.Description, got.Description)
		assert.Equal(got.CreateTime, got.UpdateTime)

		in2.StaticHostCatalogId = cat1.GetPublicId()
		got2, err := repo.CreateHost(context.Background(), in2)
		assert.NoError(err)
		require.NotNil(got2)
		assertPublicId(t, HostPrefix, got2.PublicId)
		assert.NotSame(in2, got2)
		assert.Equal(in2.Name, got2.Name)
		assert.Equal(in2.Description, got2.Description)
		assert.Equal(got2.CreateTime, got2.UpdateTime)
	})
}