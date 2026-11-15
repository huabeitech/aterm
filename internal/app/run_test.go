package app

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestApplicationMenuUsesStandardMacMenus(t *testing.T) {
	application.New(application.Options{Name: "aTerm"})
	menu := applicationMenu()
	for _, role := range []application.Role{application.AppMenu, application.EditMenu, application.WindowMenu} {
		if menu.FindByRole(role) == nil {
			t.Fatalf("applicationMenu() missing role %d", role)
		}
	}
}
