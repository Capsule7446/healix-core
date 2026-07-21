package automation

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Capsule7446/healix-core/domain/automation"
)

type environmentRepositoryFake struct {
	current   domain.Environment
	created   domain.Environment
	updated   domain.Environment
	expected  domain.Revision
	loadErr   error
	createErr error
	updateErr error
}

func (f *environmentRepositoryFake) Load(context.Context, string) (domain.Environment, error) {
	return f.current, f.loadErr
}
func (f *environmentRepositoryFake) Create(_ context.Context, value domain.Environment) (domain.Environment, error) {
	f.created = value
	return value, f.createErr
}
func (f *environmentRepositoryFake) Update(_ context.Context, expected domain.Revision, value domain.Environment) (domain.Environment, error) {
	f.expected, f.updated = expected, value
	return value, f.updateErr
}

func TestEnvironmentServiceLifecycleUsesRevisionCAS(t *testing.T) {
	repository := &environmentRepositoryFake{}
	service := NewEnvironmentService(repository)
	created, err := service.Create(context.Background(), domain.Environment{ID: "environment", DisplayName: "Local", BaseURL: "https://example.com", Variables: domain.Properties{}, Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1})
	if err != nil || created.Revision != 1 || repository.created.Revision != 1 {
		t.Fatalf("created/error = %#v/%v", created, err)
	}
	repository.current = created
	updated, err := service.Update(context.Background(), created.ID, "CI", "https://ci.example.com", domain.Properties{"region": "eu"}, domain.Properties{}, 1, 2)
	if err != nil || updated.Revision != 2 || repository.expected != 1 || repository.updated.DisplayName != "CI" {
		t.Fatalf("updated/error/expected = %#v/%v/%d", updated, err, repository.expected)
	}
	repository.current = updated
	deleted, err := service.Delete(context.Background(), updated.ID, 2, 3)
	if err != nil || deleted.DeletedAt != 3 || deleted.Revision != 3 {
		t.Fatalf("deleted/error = %#v/%v", deleted, err)
	}
	repository.current = deleted
	restored, err := service.Restore(context.Background(), deleted.ID, 3, 4)
	if err != nil || restored.DeletedAt != 0 || restored.Revision != 4 {
		t.Fatalf("restored/error = %#v/%v", restored, err)
	}
}

func TestEnvironmentServiceRejectsConflictsAndPropagatesErrors(t *testing.T) {
	failure := errors.New("failure")
	valid := domain.Environment{ID: "environment", DisplayName: "Local", BaseURL: "https://example.com", Variables: domain.Properties{}, Properties: domain.Properties{}, CreatedAt: 1, UpdatedAt: 1, Revision: 1}
	tests := []struct {
		name       string
		repository *environmentRepositoryFake
		create     bool
		want       error
	}{
		{name: "invalid create", repository: &environmentRepositoryFake{}, create: true},
		{name: "create persistence", repository: &environmentRepositoryFake{createErr: failure}, create: true, want: failure},
		{name: "load", repository: &environmentRepositoryFake{loadErr: failure}, want: failure},
		{name: "revision conflict", repository: &environmentRepositoryFake{current: valid}, want: ErrRevisionConflict},
		{name: "update persistence", repository: &environmentRepositoryFake{current: valid, updateErr: failure}, want: failure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewEnvironmentService(test.repository)
			var err error
			if test.create {
				value := valid
				value.Revision = 0
				if test.name == "invalid create" {
					value.DisplayName = ""
				}
				_, err = service.Create(context.Background(), value)
			} else {
				expected := domain.Revision(2)
				if test.name == "update persistence" {
					expected = 1
				}
				_, err = service.Update(context.Background(), valid.ID, "Updated", valid.BaseURL, domain.Properties{}, domain.Properties{}, expected, 2)
			}
			if err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
