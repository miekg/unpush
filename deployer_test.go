package main

import (
	"testing"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestInjectPassEnv(t *testing.T) {
	env := func(name string) (string, bool) {
		m := map[string]string{"FOO": "bar", "BAZ": "qux"}
		v, ok := m[name]
		return v, ok
	}

	t.Run("injects present vars", func(t *testing.T) {
		project := &composetypes.Project{
			Services: composetypes.Services{"web": {Name: "web"}},
		}
		missing := injectPassEnv(project, []string{"FOO"}, env)
		assert.Empty(t, missing)
		assert.Equal(t, "bar", *project.Services["web"].Environment["FOO"])
	})

	t.Run("overrides value already in compose file", func(t *testing.T) {
		old := "old"
		project := &composetypes.Project{
			Services: composetypes.Services{"web": {
				Name:        "web",
				Environment: composetypes.MappingWithEquals{"FOO": &old},
			}},
		}
		injectPassEnv(project, []string{"FOO"}, env)
		assert.Equal(t, "bar", *project.Services["web"].Environment["FOO"])
	})

	t.Run("reports missing vars", func(t *testing.T) {
		project := &composetypes.Project{
			Services: composetypes.Services{"web": {Name: "web"}},
		}
		missing := injectPassEnv(project, []string{"MISSING"}, env)
		assert.Equal(t, []string{"MISSING"}, missing)
		assert.Nil(t, project.Services["web"].Environment)
	})

	t.Run("injects into multiple services", func(t *testing.T) {
		project := &composetypes.Project{
			Services: composetypes.Services{
				"web":    {Name: "web"},
				"worker": {Name: "worker"},
			},
		}
		missing := injectPassEnv(project, []string{"FOO", "BAZ"}, env)
		assert.Empty(t, missing)
		for _, name := range []string{"web", "worker"} {
			assert.Equal(t, "bar", *project.Services[name].Environment["FOO"])
			assert.Equal(t, "qux", *project.Services[name].Environment["BAZ"])
		}
	})

	t.Run("no-op with empty names list", func(t *testing.T) {
		project := &composetypes.Project{
			Services: composetypes.Services{"web": {Name: "web"}},
		}
		missing := injectPassEnv(project, nil, env)
		assert.Empty(t, missing)
		assert.Nil(t, project.Services["web"].Environment)
	})
}
